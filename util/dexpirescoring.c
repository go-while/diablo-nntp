/*
 * DEXPIRESCORING.C    - dreaderd cache scoring expire.
 *
 * (c)Copyright 2002, Francois Petillon, All Rights Reserved.  Refer to
 *    the COPYRIGHT file in the base directory of this distribution 
 *    for specific rights granted.
 *
 */

#include "defs.h"
#include <math.h>

int CacheHitsFD=-1;
char *CacheHits;
uint32 CacheHitsEnd;

void Usage(char *progname)
{
    fprintf(stderr, "Expire the cache scoring\n");
    fprintf(stderr, "dexpirescoring [-H halflife]\n");
    exit(1);
}

char *
allocTmpCopy(const char *buf, int bufLen)
{
    static char *SaveAry[8];
    static int SaveCnt;
    char **pptr;

    SaveCnt = (SaveCnt + 1) % arysize(SaveAry);
    pptr = &SaveAry[SaveCnt];
    if (*pptr)
	free(*pptr);
    *pptr = malloc(bufLen + 1);
    memcpy(*pptr, buf, bufLen);
    (*pptr)[bufLen] = 0;
    return(*pptr);
}

/*
 * We hold the cache.hits lock, so no entry is added meanwhile; dreaderd's
 * readers still update the counters (atomically) while we run.  main()
 * checked the header: the hash table fits and one entry fits after the
 * header.  A damaged chain ends the walk instead of leaving the map or
 * looping.
 */
struct CacheHitEntry*
FindCacheHitEntry(struct CacheHash_t *ch) {
    uint32 *i=NULL;
    struct CacheHitHead *chh=(struct CacheHitHead *)CacheHits;
    uint32 first=sizeof(struct CacheHitHead)+chh->chh_hashSize*sizeof(uint32);
    uint32 max=CacheHitsEnd-sizeof(struct CacheHitEntry);
    uint32 hops=(CacheHitsEnd-first)/sizeof(struct CacheHitEntry);
    struct CacheHitEntry *che=NULL;
    i = (uint32*) (CacheHits+sizeof(struct CacheHitHead)+((ch->h1^ch->h2)%chh->chh_hashSize)*sizeof(uint32));

    /* the last entry may end exactly at CacheHitsEnd; a chain longer
     * than the entries that fit is a loop */
    while (*i != 0 && *i >= first && *i <= max && hops-- > 0) {
	che = (struct CacheHitEntry*) (CacheHits + (*i));
	if (CacheHashEqual(ch, CHE_HASH(che))) {
	    return che;
	}
	i = &(che->che_Next);
    }
    return NULL;
}

/* Divide a counter by expire without losing a reader's concurrent add. */
static void
DecayReadArt(int *p, double expire)
{
    int o, n;

    do {
	o = CH_LOAD(p);
	n = (int)((float)o/expire);
    } while (!CH_CAS(p, o, n));
}

static void
DecayHits(uint32 *p, double expire)
{
    uint32 o, n;

    do {
	o = CH_LOAD(p);
	n = (int)((float)o/expire);
    } while (!CH_CAS(p, o, n));
}

int
main(int ac, char **av) {
    struct CacheHitHead chh;
    double expire=0;
    int i,r,halflife=0;
    int verbose=0;

    LoadDiabloConfig(ac, av);

    CacheHitsFD = open(PatDbExpand(CacheHitsPat), O_RDWR, 0644);
    if (CacheHitsFD<0) {
	fprintf(stderr, "Can not open cache hits file (%s)\n", PatDbExpand(CacheHitsPat));
	exit(1);
    }
    if (hflock(CacheHitsFD, 0, XLOCK_EX) < 0) {
	int error = errno;

	fprintf(stderr, "Can not lock cache hits file %s (%s)\n",
	    PatDbExpand(CacheHitsPat), strerror(error));
	exit(1);
    }
    r = read(CacheHitsFD, &chh, sizeof(struct CacheHitHead));
    /* the same checks as dreaderd, and room for one entry */
    if (!CacheHitsUsable(CacheHitsFD, &chh, r) ||
	chh.chh_end < sizeof(struct CacheHitHead) + sizeof(struct CacheHitEntry)) {
	fprintf(stderr, "Wrong version or damaged header, can not expire scoring\n");
	hflock(CacheHitsFD, 0, XLOCK_UN);
	exit(1);
    }
    CacheHits = mmap(NULL, chh.chh_end, PROT_READ|PROT_WRITE, MAP_SHARED, CacheHitsFD, 0);
    CacheHitsEnd = chh.chh_end;
    if (CacheHits == MAP_FAILED) {
	fprintf(stderr, "Error on cache hits mmap (%s)", strerror(errno));
	hflock(CacheHitsFD, 0, XLOCK_UN);
	close(CacheHitsFD);
	exit(1);
    }

    for (i = 1; i < ac; ++i) {
	char *ptr = av[i];
	if (*ptr != '-') {
	    fprintf(stderr, "Unexpected argument: %s\n", ptr);
	    Usage(av[0]);
	}
	ptr += 2;
	switch(ptr[-1]) {
	case 'C':		/* parsed by LoadDiabloConfig */
	    if (*ptr == 0)
		++i;
	    break;
	case 'H':
	    halflife = strtol((*ptr) ? ptr : av[++i], NULL, 0);
	    break;
	case 'v':
	    verbose++;
	    break;
	default:
	    fprintf(stderr, "Unknown option: %s\n", ptr - 2);
	    Usage(av[0]);
	}
    }

    if (halflife>0) {
	time_t delay, now;
	struct CacheHitHead *chh = (struct CacheHitHead *) CacheHits;

	now = time(NULL);
	delay = now - chh->chh_lastExpired;
	chh->chh_lastExpired = now;
	expire = pow(2, ((double)delay/halflife));

	if (verbose) {
	    printf("delay : %i (%f)\n", (int)delay, expire);
	}
    }

    {
	KPDB  *KDBActive;
	int recOff, recLen;
	KDBActive = KPDBOpen(PatDbExpand(ReaderDActivePat), O_RDWR);
	if (KDBActive == NULL) {
	    fprintf(stderr, "Unable to open dactive.kp\n");
	    exit(1);
	}
	for (recOff = KPDBScanFirst(KDBActive, 0, &recLen);
		recOff;
		recOff = KPDBScanNext(KDBActive, recOff, 0, &recLen)
	) {
	    int groupLen;
	    struct CacheHash_t ch;
	    struct CacheHitEntry *che;
	    const char *rec = KPDBReadRecordAt(KDBActive, recOff, 0, NULL);
	    const char *group = KPDBGetField(rec, recLen, NULL, &groupLen, NULL);
	    artno_t endNo = strtoll(KPDBGetField(rec, recLen, "NE", NULL, "-1"), NULL, 10);
	    int iter = (int)strtoul(KPDBGetField(rec, recLen, "ITER", NULL, "0"), NULL, 10);	/* written as %06d */
	    
	    if (group)
		group = allocTmpCopy(group, groupLen);

	    SetCacheHash(&ch, group, iter, &DOpts.ReaderGroupHashMethod);
	    che = FindCacheHitEntry(&ch);
	    if (che != NULL) {
		int new;
		new = che->che_NewArt+endNo-che->che_LastHi;
		che->che_LastHi = endNo;
		if (expire>1) {
		    DecayReadArt(&che->che_ReadArt, expire);
		    DecayHits(&che->che_Hits, expire);
		    che->che_NewArt = (int)((float)new/expire);
		    new = che->che_NewArt+endNo-che->che_LastHi;
		} else {
		    che->che_NewArt = new;
		}
		if (verbose) {
		    if (new<1) new=1;
		    if (che->che_Hits) {
			printf("read:%7i (hits:%6.2f%%) new:%6i (r/n=%6.2f) %s\n", che->che_ReadArt+che->che_Hits, 100*(float)che->che_Hits/(che->che_ReadArt+che->che_Hits), new, ((float)(che->che_ReadArt+che->che_Hits)/new), group);
		    } else {
			printf("read:%7i                new:%6i (r/n=%6.2f) %s\n", che->che_ReadArt+che->che_Hits, new, ((float)che->che_ReadArt/new), group);
		    }
		}
	    }
	}
    }

    
    hflock(CacheHitsFD, 0, XLOCK_UN);
    close(CacheHitsFD);
    exit(0);
}
