/*
 * DREADERD/DNSCACHE.C - name lookup cache of the main process
 *
 *	Main keeps the reverse name and forward-confirmation outcome of
 *	recent client addresses, and passes a hit to the DNS worker in the
 *	DnsReq so the worker skips both lookups.  Access rules, auth and
 *	accounting still run on every login; only the lookups are cached.
 *
 *	The table is fixed-size and 4-way set associative.  A full set
 *	drops the entry closest to expiry.  Main is single-threaded, so
 *	there is no locking.  Entries with a name use readerdnscachettl,
 *	entries without one or with a forward mismatch readerdnscachenegttl.
 *	An entry keeps the (monotonic) time it was stored and is checked
 *	against the current TTL, so a lowered TTL applies at once.  Lookups
 *	that failed, or may have failed, temporarily are not cached.
 */

#include "defs.h"

Prototype int DnsCacheGet(const struct sockaddr *sa, int *lookup, char *host, size_t hostlen);
Prototype void DnsCacheStore(const struct sockaddr *sa, int lookup, const char *host, unsigned int gen);
Prototype unsigned int DnsCacheGen(void);
Prototype int DnsCacheSameAddr(const struct sockaddr *a, const struct sockaddr *b);
Prototype void DnsCacheFlush(void);
Prototype void DnsCacheStats(FILE *fo);

#define DC_WAYS	4

typedef struct DcKey {
    unsigned char k_Family;		/* 4 or 6, 0 = not cacheable	*/
    unsigned char k_Addr[16];
    unsigned int k_Scope;			/* IPv6 scope id		*/
} DcKey;

typedef struct DcEnt {
    DcKey dc_Key;
    time_t dc_Stored;			/* dcNow() when stored		*/
    int dc_Lookup;			/* DL_*, DL_NONE = empty slot	*/
    char dc_Host[DNSHOSTLEN];
} DcEnt;

static DcEnt *DcTab;
static int DcSets;
static int DcAllocFailed;
static unsigned int DcSeed;
static unsigned int DcGen;		/* bumped by a flush		*/
static unsigned long DcHits, DcMisses, DcStores, DcEvicts, DcUsed;

/*
 * The key is the address without the port.  An IPv4-mapped IPv6 address
 * gets the same key as the IPv4 address.
 */
static int
dcKey(const struct sockaddr *sa, DcKey *k)
{
    memset(k, 0, sizeof(*k));
    if (sa == NULL)
	return(0);
    if (sa->sa_family == AF_INET) {
	const struct sockaddr_in *sin = (const void *)sa;

	k->k_Family = 4;
	memcpy(k->k_Addr, &sin->sin_addr, 4);
	return(1);
    }
#ifdef INET6
    if (sa->sa_family == AF_INET6) {
	const struct sockaddr_in6 *sin6 = (const void *)sa;

	if (IN6_IS_ADDR_V4MAPPED(&sin6->sin6_addr)) {
	    k->k_Family = 4;
	    memcpy(k->k_Addr, (const char *)&sin6->sin6_addr + 12, 4);
	} else {
	    k->k_Family = 6;
	    memcpy(k->k_Addr, &sin6->sin6_addr, 16);
	    k->k_Scope = sin6->sin6_scope_id;
	}
	return(1);
    }
#endif
    return(0);
}

static int
dcKeyEq(const DcKey *a, const DcKey *b)
{
    return(a->k_Family == b->k_Family && a->k_Scope == b->k_Scope &&
	memcmp(a->k_Addr, b->k_Addr, sizeof(a->k_Addr)) == 0);
}

/* FNV-1a over the key fields */
static unsigned int
dcHash(const DcKey *k)
{
    unsigned int h = 2166136261U ^ DcSeed;
    int i;

    h = (h ^ k->k_Family) * 16777619U;
    for (i = 0; i < (int)sizeof(k->k_Addr); ++i)
	h = (h ^ k->k_Addr[i]) * 16777619U;
    for (i = 0; i < 4; ++i)
	h = (h ^ ((k->k_Scope >> (i * 8)) & 0xff)) * 16777619U;
    return(h);
}

static int
dcTtl(int lookup)
{
    return((lookup == DL_HOST) ? DOpts.ReaderDnsCacheTTL :
				DOpts.ReaderDnsCacheNegTTL);
}

/* seconds from a clock that a wall-clock step does not move */
static time_t
dcNow(void)
{
#ifdef CLOCK_MONOTONIC
    struct timespec ts;

    if (clock_gettime(CLOCK_MONOTONIC, &ts) == 0)
	return(ts.tv_sec);
#endif
    return(time(NULL));
}

/* seconds an entry has left under the current TTLs, <= 0 = expired */
static time_t
dcLeft(const DcEnt *e, time_t now)
{
    int ttl = dcTtl(e->dc_Lookup);

    if (ttl <= 0 || now < e->dc_Stored)
	return(0);
    return(e->dc_Stored + ttl - now);
}

/*
 * The size is read on first use: readerdnscachesize entries, rounded down
 * to a multiple of DC_WAYS (at least one set).  A failed allocation is
 * logged once and not retried until a flush.
 */
static int
dcAlloc(void)
{
    if (DcTab != NULL)
	return(1);
    if (DOpts.ReaderDnsCacheSize <= 0 || DcAllocFailed)
	return(0);
    DcSets = DOpts.ReaderDnsCacheSize / DC_WAYS;
    if (DcSets < 1)
	DcSets = 1;
    if ((DcTab = calloc((size_t)DcSets * DC_WAYS, sizeof(DcEnt))) == NULL) {
	logit(LOG_ERR, "DNS cache: no memory for %d entries, cache off",
	    DcSets * DC_WAYS);
	DcSets = 0;
	DcAllocFailed = 1;
	return(0);
    }
    DcSeed = (unsigned int)time(NULL) * 2654435761U ^ (unsigned int)getpid();
    DcUsed = 0;
    return(1);
}

/*
 * A hit fills *lookup and host and returns 1.  Otherwise returns 0 and the
 * worker does the lookups.
 */
int
DnsCacheGet(const struct sockaddr *sa, int *lookup, char *host, size_t hostlen)
{
    DcKey k;
    DcEnt *e;
    time_t now;
    int i;

    if (DcTab == NULL || !dcKey(sa, &k))
	return(0);
    now = dcNow();
    e = &DcTab[(size_t)(dcHash(&k) % (unsigned int)DcSets) * DC_WAYS];
    for (i = 0; i < DC_WAYS; ++i, ++e) {
	if (e->dc_Lookup == DL_NONE || !dcKeyEq(&e->dc_Key, &k))
	    continue;
	if (dcLeft(e, now) <= 0 || strlen(e->dc_Host) >= hostlen)
	    break;
	*lookup = e->dc_Lookup;
	memcpy(host, e->dc_Host, strlen(e->dc_Host) + 1);
	++DcHits;
	return(1);
    }
    ++DcMisses;
    return(0);
}

/* the flush generation; a result looked up before a flush is not stored */
unsigned int
DnsCacheGen(void)
{
    return(DcGen);
}

void
DnsCacheStore(const struct sockaddr *sa, int lookup, const char *host, unsigned int gen)
{
    DcKey k;
    DcEnt *set;
    DcEnt *slot = NULL;
    time_t now;
    size_t len = strlen(host);
    int i;

    if (gen != DcGen || dcTtl(lookup) <= 0 ||
	    lookup < DL_HOST || lookup > DL_NOHOST ||
	    len >= DNSHOSTLEN || (lookup != DL_NOHOST && len == 0))
	return;
    if (!dcKey(sa, &k) || !dcAlloc())
	return;
    now = dcNow();
    set = &DcTab[(size_t)(dcHash(&k) % (unsigned int)DcSets) * DC_WAYS];

    /* same key, else an empty or expired slot, else closest to expiry */
    for (i = 0; i < DC_WAYS; ++i) {
	if (set[i].dc_Lookup != DL_NONE && dcKeyEq(&set[i].dc_Key, &k)) {
	    slot = &set[i];
	    break;
	}
    }
    for (i = 0; slot == NULL && i < DC_WAYS; ++i) {
	if (set[i].dc_Lookup == DL_NONE) {
	    slot = &set[i];
	    ++DcUsed;
	}
    }
    for (i = 0; slot == NULL && i < DC_WAYS; ++i) {
	if (dcLeft(&set[i], now) <= 0)
	    slot = &set[i];
    }
    if (slot == NULL) {
	slot = &set[0];
	for (i = 1; i < DC_WAYS; ++i) {
	    if (dcLeft(&set[i], now) < dcLeft(slot, now))
		slot = &set[i];
	}
	++DcEvicts;
    }
    slot->dc_Key = k;
    slot->dc_Stored = now;
    slot->dc_Lookup = lookup;
    memcpy(slot->dc_Host, host, len + 1);
    ++DcStores;
}

/* 1 if both addresses (without the port) are the same client */
int
DnsCacheSameAddr(const struct sockaddr *a, const struct sockaddr *b)
{
    DcKey ka, kb;

    return(dcKey(a, &ka) && dcKey(b, &kb) && dcKeyEq(&ka, &kb));
}

/*
 * Free the table (it is allocated again on the next store) and bump the
 * generation, so lookups already in flight are not stored.
 */
void
DnsCacheFlush(void)
{
    free(DcTab);
    DcTab = NULL;
    DcSets = 0;
    DcUsed = 0;
    DcAllocFailed = 0;
    ++DcGen;
}

void
DnsCacheStats(FILE *fo)
{
    fprintf(fo, "dnscache size=%d used=%lu hits=%lu misses=%lu stores=%lu evictions=%lu ttl=%d negttl=%d\n",
	(DcTab != NULL) ? DcSets * DC_WAYS : 0, DcUsed, DcHits, DcMisses,
	DcStores, DcEvicts, DOpts.ReaderDnsCacheTTL,
	DOpts.ReaderDnsCacheNegTTL);
}
