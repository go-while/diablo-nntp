
/*
 * LIB/ZALLOC.C	- mostly self contained zero-overhead memory pool/allocation 
 *		  subsystem.
 *
 *	This subsystem implements memory pools and memory allocation 
 *	routines.  It uses pagealloc() for a low level allocator and
 *	malloc() for the MemPool structures.  The idea is to (a) prevent 
 *	fragmentation and (b) allow pools to be unmapped from the VM space
 *	when no longer needed.
 *
 *	Pools are managed via a linked list of 'free' areas.  Allocating
 *	memory creates holes in the freelist, freeing memory fills them.
 *	Since the freelist consists only of free memory areas, it is possible
 *	to allocate all the memory in a pool without incuring any structural
 *	overhead.
 *
 *	The system works best when allocating similarly-sized chunks of
 *	memory.
 *
 * (c)Copyright 1997, Matthew Dillon, All Rights Reserved.  Refer to
 *    the COPYRIGHT file in the base directory of this distribution
 *    for specific rights granted.
 */

#define DEBUG_ZALLOC_OFF
#include "defs.h"

#define POOLSIZE	65536

#define MEMNODE_SIZE_MASK	((sizeof(char *) == 4) ? 7 : 15)

Prototype void *nzalloc(MemPool **mpool, int bytes);
Prototype void *zalloc(MemPool **mpool, int bytes);
Prototype char *zallocStr(MemPool **pmp, const char *s);
Prototype char *zappendStr(MemPool **pmp, char **pbuf, const char *s1, const char *s2);
Prototype char *zallocStrTrim(MemPool **pmp, const char *s, int l);
Prototype char *zallocStrTrim2(MemPool **pmp, int trimwsafter, const char *s, int l);
Prototype void zfree(MemPool **mpool, void *ptr, int bytes);
Prototype void zfreeStr(MemPool **pmp, char **ps);
Prototype void allocPool(MemPool **mpool, int bytes);
Prototype void freePool(MemPool **mpool);

#ifdef DEBUG_ZALLOC
Prototype void dbg_install_sighandler(void);
static void dbg_zalloc_add(MemPool *mp, char *dataptr, int bytes);
static void dbg_zalloc_free(MemPool **pmp, char *dataptr, int bytes);
static void dbg_zalloc_add_pool(MemPool **pmp, MemPool *mp);
static void dbg_zalloc_free_pool(MemPool **pmp);

char *__dbg_file;
char *__dbg_func;
int __dbg_line;
#endif

MemPool *initPool(int bytes);

void *
nzalloc(MemPool **pmp, int bytes)
{
    MemPool *mp;

    /* 8 or 16-byte alignment required, depending on the pointer size */
    bytes = (bytes + MEMNODE_SIZE_MASK) & ~MEMNODE_SIZE_MASK;

    for(mp = *pmp; mp != NULL; mp = mp->mp_Next) {
	if (bytes <= mp->mp_Size - mp->mp_Used) {
            MemNode **pmn;
            MemNode *mn;

            for (pmn = &mp->mp_First; (mn = *pmn) != NULL; pmn = &mn->mr_Next) {
                if (bytes <= mn->mr_Bytes) {
                    /*
                     *  Cut a chunk of memory out of the beginning of this
                     *  block and fixup the link appropriately.
                     */
		    char *ptr = (char *)mn;
                    if (mn->mr_Bytes == bytes) {
                        *pmn = mn->mr_Next;
                    } else {
                        mn = (MemNode *)((char *)mn + bytes);
                        mn->mr_Next  = ((MemNode *)ptr)->mr_Next;
                        mn->mr_Bytes = ((MemNode *)ptr)->mr_Bytes - bytes;
                        *pmn = mn;
                    }
                    mp->mp_Used += bytes;
#ifdef DEBUG_ZALLOC
		    dbg_zalloc_add(mp, ptr, bytes);
#endif
                    return(ptr);
                }
            }
        }
    }
    /*
     * Failed to locate sufficient memory, allocate another
     * pool.
     */
    allocPool(pmp, ((bytes < POOLSIZE) ? POOLSIZE : bytes));
    return(zalloc(pmp, bytes));
}

void *
zalloc(MemPool **pmp, int bytes)
{
    void *ptr = nzalloc(pmp, bytes);
    bzero(ptr, bytes);
    return(ptr);
}

char *
zallocStr(MemPool **pmp, const char *s)
{
    char *r = zalloc(pmp, strlen(s) + 1);
    strcpy(r, s);
    return(r);
}

char *
zappendStr(MemPool **pmp, char **pbuf, const char *s1, const char *s2)
{
    int l = ((s1) ? strlen(s1) : 0) + ((s2) ? strlen(s2) : 0);
    int o = 0;
    char *ptr = NULL;

    if (*pbuf) {
	o = strlen(*pbuf);
	ptr = zalloc(pmp, l + o + 1);
	strcpy(ptr, *pbuf);
	zfree(pmp, *pbuf, o + 1);
	*pbuf = NULL;
    } else {
	ptr = zalloc(pmp, l + 1);
    }
    ptr[o] = 0;
    if (s1)
	strcat(ptr + o, s1);
    if (s2)
	strcat(ptr + o, s2);
    *pbuf = ptr;
    return(ptr);
}

char *
zallocStrTrim(MemPool **pmp, const char *s, int l)
{
    char *r;

    while (l && (*s == ' ' || *s == '\t')) {
	++s;
	--l;
    }
    --l;
    while (l >= 0 &&
	    (s[l] == '\r' || s[l] == '\n' || s[l] == ' ' || s[l] == '\t')
    ) {
	--l;
    }
    ++l;

    r = zalloc(pmp, l + 1);
    bcopy(s, r, l);
    r[l] = 0;
    return(r);
}

/*
 * Same as zallocStrTrim, but also removes embedded \r and \n,
 * and removes whitespace after the 'trimwsafter' character.
 * For example with trimwsafter set to ',':
 * foo.bar1, foo.bar2,\r\n\tfoo.bar3 => foo.bar1,foo.bar2,foo.bar3
 */
char *
zallocStrTrim2(MemPool **pmp, int trimwsafter, const char *s, int l)
{
    char *r = NULL;
    int i, n, doit;
    int trimws = 0;

    while (l && (*s == ' ' || *s == '\t')) {
	++s;
	--l;
    }
    --l;
    while (l >= 0 &&
	    (s[l] == '\r' || s[l] == '\n' || s[l] == ' ' || s[l] == '\t')
    ) {
	--l;
    }
    ++l;

    n = 0;
    for (doit = 0; doit < 2; doit++) {
	if (doit) {
	    r = zalloc(pmp, n + 1);
	    n = 0;
	}
	for (i = 0; i < l; i++) {
	    /* skip cr and lf */
	    if (s[i] == '\r' || s[i] == '\n')
		continue;
	    /* skip whitespace after the 'trimwsafter' character */
	    if (trimws && (s[i] == ' ' || s[i] == '\t'))
		continue;
	    if (doit)
		r[n] = s[i];
	    n++;
	    trimws = (s[i] == trimwsafter);
	}
    }
    r[n] = 0;
    return(r);
}

void
zfree(MemPool **pmp, void *ptr, int bytes)
{
    MemPool *mp;

    /* 8 or 16-byte alignment required, depending on the pointer size */
    bytes = (bytes + MEMNODE_SIZE_MASK) & ~MEMNODE_SIZE_MASK;

#ifdef DEBUG_ZALLOC
    dbg_zalloc_free(pmp, ptr, bytes);
#endif

    while ((mp = *pmp) != NULL) {
	if ((char *)ptr >= (char *)mp->mp_Base && (char *)ptr < (char *)mp->mp_Base + mp->mp_Size) {
	    MemNode **pmn;
	    MemNode *mn;

	    mp->mp_Used -= bytes;

	    for (pmn = &mp->mp_First; (mn = *pmn) != NULL; pmn = &mn->mr_Next) {
		/*
		 * If area between last node and current node
		 *  - check range
		 *  - check merge with next area
		 *  - check merge with previous area
		 */
		if ((char *)ptr <= (char *)mn) {
		    /*
		     * range check
		     */
		    if ((char *)ptr + bytes > (char *)mn) {
			logit(LOG_CRIT, "zfree(%08lx,%d) failed1, corrupt memlist", (long)ptr, bytes);
			exit(1);
		    }

		    /*
		     * merge against next area or create independant area
		     */

		    if ((char *)ptr + bytes == (char *)mn) {
			((MemNode *)ptr)->mr_Next = mn->mr_Next;
			((MemNode *)ptr)->mr_Bytes= bytes + mn->mr_Bytes;
		    } else {
			((MemNode *)ptr)->mr_Next = mn;
			((MemNode *)ptr)->mr_Bytes= bytes;
		    }
		    *pmn = mn = (MemNode *)ptr;

		    /*
		     * merge against previous area (if there is a previous
		     * area).
		     */

		    if (pmn != &mp->mp_First) {
			if ((char *)pmn + ((MemNode *)pmn)->mr_Bytes == (char *)ptr) {
			    ((MemNode *)pmn)->mr_Next = mn->mr_Next;
			    ((MemNode *)pmn)->mr_Bytes += mn->mr_Bytes;
			}
		    }
		    return;
		}
		if ((char *)ptr < (char *)mn + mn->mr_Bytes) {
		    logit(LOG_CRIT, "zfree(%08lx,%d) failed2, corrupt memlist", (long)ptr, bytes);
		    exit(1);
		}
	    }
	    /*
	     * We are beyond the last MemNode, append new MemNode.  Merge against
	     * previous area if possible.
	     */
	    ((MemNode *)ptr)->mr_Next = NULL;
	    ((MemNode *)ptr)->mr_Bytes = bytes;
	    *pmn = mn = (MemNode *)ptr;
	    return;
	}
	pmp = &mp->mp_Next;
    }
    logit(LOG_CRIT, "zfree(%08lx,%d) failed3, corrupt memlist", (long)ptr, bytes);
    exit(1);
}

void
zfreeStr(MemPool **pmp, char **ps)
{
    if (*ps) {
	zfree(pmp, *ps, strlen(*ps) + 1);
	*ps = NULL;
    }
}

void
allocPool(MemPool **pmp, int bytes)
{
    MemPool *mp = calloc(1, sizeof(MemPool));

    mp->mp_Next = *pmp;
    mp->mp_Base = pagealloc(&mp->mp_Size, bytes);
    mp->mp_First = mp->mp_Base;
    mp->mp_First->mr_Next = NULL;
    mp->mp_First->mr_Bytes = mp->mp_Size;
#ifdef DEBUG_ZALLOC
    dbg_zalloc_add_pool(pmp, mp);
#endif
    *pmp = mp;
}

void
freePool(MemPool **pmp)
{
    MemPool *mp;

#ifdef DEBUG_ZALLOC
    dbg_zalloc_free_pool(pmp);
#endif

    while ((mp = *pmp) != NULL) {
	*pmp = mp->mp_Next;
	pagefree(mp->mp_Base, mp->mp_Size);
	free(mp);
    }
}

#ifdef DEBUG_ZALLOC

#define NODE_BUCKETS	512

struct dbg_zalloc_node {
    char *file;
    int line;
    char *func;
    char *dataptr;
    int bytes;
    struct dbg_zalloc_node **pprevnext;
    struct dbg_zalloc_node *next;
};

struct dbg_zalloc_pool {
    char *file;
    int line;
    unsigned long id;
    MemPool *mp;
    struct dbg_zalloc_node *node[NODE_BUCKETS];
    struct dbg_zalloc_pool **pprevnext;
    struct dbg_zalloc_pool *next;
};

static struct dbg_zalloc_pool *dzpools;
static int dbg_busy;

static char *
dbg_size(unsigned long sz, char *tmp)
{
    char *suffix = "";
    char *fmt = "%lu (%.1f%s)";
    double val;

    if (sz > (1024*1024*1024)) {
	val = (double)sz / (1024*1024*1024);
	suffix = "G";
    } else if (sz > (1024*1024)) {
	val = (double)sz / (1024*1024);
	suffix = "M";
    } else if (sz > 1024) {
	val = (double)sz / 1024;
	suffix = "K";
    } else {
	val = sz;
	fmt = "%lu (%.0f%s)";
    }
    if (val >= 4)
	fmt = "%lu (%.0f%s)";
    snprintf(tmp, 64, fmt, sz, val, suffix);
    return tmp;
}

static void
dbg_dump(int sig)
{
    struct dbg_zalloc_node *dzn;
    struct dbg_zalloc_pool *dzp;
    MemPool *mp;
    unsigned long mp_Size, mp_Used;
    char fn[128];
    char tmp[128];
    char tmp1[64], tmp2[64];
    int i, fd;

    snprintf(fn, sizeof(fn), "/news/run/dump.%ld.%ld",
			(long)getpid(), (long)time(NULL));
    fd = open(fn, O_CREAT|O_WRONLY|O_EXCL, 0600);
    if (fd < 0)
	return;

    __sync_synchronize();
    if (dbg_busy) {
	snprintf(tmp, sizeof(tmp), "cannot dump - datastructures are busy\n");
	WriteBestEffort(fd, tmp, strlen(tmp));
	return;
    }

    for (dzp = dzpools; dzp; dzp = dzp->next) {
	mp_Size = mp_Used = 0;
	for (mp = dzp->mp; mp; mp = mp->mp_Next) {
		mp_Size += mp->mp_Size;
		mp_Used += mp->mp_Used;
	}
	snprintf(tmp, sizeof(tmp),
		"pool %lu (from %s:%d) size: %s used: %s\n",
		dzp->id, dzp->file, dzp->line,
		dbg_size(mp_Size, tmp1), dbg_size(mp_Used, tmp2));
	WriteBestEffort(fd, tmp, strlen(tmp));
	mp_Used = 0;
	for (i = 0; i < NODE_BUCKETS; i++) {
	    for (dzn = dzp->node[i]; dzn; dzn = dzn->next) {
		snprintf(tmp, sizeof(tmp),
			"  %s:%d %s allocated %d bytes @ %p\n",
			dzn->file, dzn->line, dzn->func,
			dzn->bytes, dzn->dataptr);
		WriteBestEffort(fd, tmp, strlen(tmp));
		mp_Used += dzn->bytes;
	    }
	}
	snprintf(tmp, sizeof(tmp), "all memnodes: %s\n\n",
		dbg_size(mp_Used, tmp1));
	WriteBestEffort(fd, tmp, strlen(tmp));
    }
    WriteBestEffort(fd, "\n", 1);
    close(fd);
}

void
dbg_install_sighandler(void)
{
    struct sigaction sa;

    memset(&sa, 0, sizeof(sa));
    sa.sa_handler = dbg_dump;
    sa.sa_flags = SA_RESTART;
    sigaction(SIGUSR2, &sa, NULL);
}

static struct dbg_zalloc_node *
dbg_zalloc_find_node( struct dbg_zalloc_pool *dzp, char *dataptr)
{
    struct dbg_zalloc_node *dzn;
    unsigned int h;

    h = (unsigned long)dataptr % NODE_BUCKETS;
    for (dzn = dzp->node[h]; dzn; dzn = dzn->next) {
	if (dzn->dataptr == dataptr)
	    break;
    }
    return dzn;
}

static void
dbg_zalloc_add(MemPool *mp, char *dataptr, int bytes)
{
    struct dbg_zalloc_node *dzn;
    struct dbg_zalloc_pool *dzp;
    unsigned long h;

    dzp = (struct dbg_zalloc_pool *)(mp->mp_DzpPtr);

    dzn = malloc(sizeof(*dzn));
    if (dzn == NULL)
	kill(getpid(), SIGABRT);
    dzn->file = __dbg_file;
    dzn->line = __dbg_line;
    dzn->func = __dbg_func;
    dzn->dataptr = dataptr;
    dzn->bytes = bytes;

    dbg_busy++;
    __sync_synchronize();
    h = (unsigned long)dataptr % NODE_BUCKETS;
    dzn->pprevnext = &dzp->node[h];
    dzn->next = dzp->node[h];
    if (dzp->node[h])
	dzp->node[h]->pprevnext = &dzn->next;
    dzp->node[h] = dzn;
    __sync_synchronize();
    dbg_busy--;
}

static void
dbg_zalloc_free(MemPool **pmp, char *dataptr, int bytes)
{
    struct dbg_zalloc_node *dzn;
    struct dbg_zalloc_pool *dzp;

    if (*pmp == NULL)
	return;

    dzp = (struct dbg_zalloc_pool *)((*pmp)->mp_DzpPtr);
    dzn = dbg_zalloc_find_node(dzp, dataptr);
    if (dzn == NULL) {
	logit(LOG_ERR, "dbg_zalloc_free: "
			"cannot find dataptr %p", dataptr);
	return;
    }
    dbg_busy++;
    __sync_synchronize();
    *(dzn->pprevnext) = dzn->next;
    if (dzn->next)
	dzn->next->pprevnext = dzn->pprevnext;
    __sync_synchronize();
    dbg_busy--;
    free(dzn);
}

static void
dbg_zalloc_add_pool(MemPool **pmp, MemPool *mp)
{
    struct dbg_zalloc_pool *dzp;
    static unsigned long mempool_id;

    if (*pmp) {
	mp->mp_DzpPtr = (*pmp)->mp_DzpPtr;
	mp->mp_DzpId = (*pmp)->mp_DzpId;
	dzp = (*pmp)->mp_DzpPtr;
	dzp->mp = mp;
	return;
    }

    dzp = calloc(1, sizeof(*dzp));
    if (dzp == NULL)
	kill(getpid(), SIGABRT);
    dzp->file = __dbg_file;
    dzp->line = __dbg_line;
    dzp->id = ++mempool_id;
    dzp->mp = mp;

    mp->mp_DzpPtr = dzp;
    mp->mp_DzpId = dzp->id;
    dzp->mp = mp;

    dbg_busy++;
    __sync_synchronize();
    dzp->pprevnext = &dzpools;
    dzp->next = dzpools;
    if (dzpools)
	dzpools->pprevnext = &dzp->next;
    dzpools = dzp;
    __sync_synchronize();
    dbg_busy--;
}

static void
dbg_zalloc_free_pool(MemPool **pmp)
{
    struct dbg_zalloc_pool *dzp;
    struct dbg_zalloc_node *dzn;
    int i;

    if (*pmp == NULL)
	return;
    dzp = (*pmp)->mp_DzpPtr;

    dbg_busy++;
    __sync_synchronize();
    for (i = 0; i < NODE_BUCKETS; i++) {
	struct dbg_zalloc_node *next;
	for (dzn = dzp->node[i]; dzn; dzn = next) {
	    next = dzn->next;
	    free(dzn);
	}
    }

    *(dzp->pprevnext) = dzp->next;
    if (dzp->next)
	dzp->next->pprevnext = dzp->pprevnext;
    __sync_synchronize();
    dbg_busy--;
    free(dzp);
}

#endif /* DEBUG_ZALLOC */

