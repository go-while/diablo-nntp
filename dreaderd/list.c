
/*
 * LIST.C
 *
 *	NNTP LIST commands.
 *
 * (c)Copyright 1998, Matthew Dillon, All Rights Reserved.  Refer to
 *    the COPYRIGHT file in the base directory of this distribution
 *    for specific rights granted.
 */

#include "defs.h"

Prototype void NNTPList(Connection *conn, char **pptr);
Prototype void NNTPListActive(Connection *conn, char **pptr);
Prototype void NNTPListActiveTimes(Connection *conn, char **pptr);
Prototype void NNTPListHeaders(Connection *conn, char **pptr);
Prototype void NNTPListNewsgroups(Connection *conn, char **pptr);
Prototype void NNTPListExtensions(Connection *conn, char **pptr);
Prototype void NNTPListDistributions(Connection *conn, char **pptr);
Prototype void NNTPListDistribPats(Connection *conn, char **pptr);
Prototype void NNTPListOverviewFmt(Connection *conn, char **pptr);
Prototype void NNTPListSubscriptions(Connection *conn, char **pptr);
Prototype void NNTPListModerators(Connection *conn, char **pptr);
Prototype void NNTPListGroup(Connection *conn, char **pptr);
Prototype void NNTPNewgroups(Connection *conn, char **pptr);
Prototype void NNTPXGTitle(Connection *conn, char **pptr);

void NNStartListActiveScan(Connection *conn, const char *wc, TimeRestrict *tr, int mode);
void NNListActiveScan(Connection *conn);
void NNListGroup(Connection *conn);

/*
 * Non-blocking calls which attempt to obtain a read or write lock on the
 * active file cache. Return 0 in case of success, -1 in case of failure
 * due to other locks not permitting the request.
 *
 * Unlock calls always succeed.
 */
Prototype int ActiveCacheReadLock(void);
Prototype void ActiveCacheReadUnlock(void);
Prototype int ActiveCacheWriteLock(void);
Prototype void ActiveCacheWriteUnlock(void);

/* Cache management functions */
Prototype void ActiveCacheFreeMain(void);

Prototype GroupList *ListActiveGroups(Connection *conn, char *pat);

int ActiveCacheValid(KPDB *kpdb);
void ActiveCacheMarkValid(KPDB *kpdb);
activeCacheEnt *ActiveCacheFind(activeCacheEnt *ac, int cts);
activeCacheEnt *ActiveCacheGetNext(activeCacheEnt *ac);
void ActiveCacheInsertHelper(activeCacheEnt **acp, activeCacheEnt *parent,
			     char *newGroup, int newCts);
void ActiveCacheInsert(activeCacheEnt **acp, char *newGroup, int newCts);

/*
 * Active file cache -- locks, last-updated, and actual cache
 *
 * The read lock is 0 iff no request is accessing the database at this
 * time. It is probably not necessary, but it's there as a safeguard
 * against bad code which doesn't copy cache contents into local space
 * before letting other processes run. When there are multiple requests
 * in progress accessing the cache, the read lock should be equal to
 * the number of such requests.
 *
 * The write lock is 0 iff it is safe to use the cache. The write lock
 * and the read lock should never be non-zero at the same time.
 *
 * activeCache_AppendSeq is the active file append sequence. The cache
 * is valid iff activeCache_AppendSeq is the same as the append sequence
 * in the active file KPDB and activeCache is not NULL.
 *
 * Note that CTS might change without an append happening. However,
 * this cache should ONLY be used for "list newgroups", and we ignore
 * groups which spuriously change their CTS field without really
 * being a new group.
 */
int activeCache_ReadLock = 0;
int activeCache_WriteLock = 0;
int activeCache_AppendSeq = 0;

struct activeCacheEnt *activeCache = NULL;
MemPool *activeCache_MemPool;

void 
NNTPList(Connection *conn, char **pptr)
{
    /*
     * list active [groupwild]
     * list active.times
     * list newsgroups [groupwild]
     * list extensions
     * list distributions
     * list distrib.pats
     * list overview.fmt
     * list subscriptions
     * list moderators
     */
    char *ltype = parseword(pptr, " \t");

    if (ltype) {
	int i;
	for (i = 0; ltype[i]; ++i)
	    ltype[i] = tolower((int)(unsigned char)ltype[i]);
    }
    if (ltype == NULL) {
	NNTPListActive(conn, NULL);
    } else if (strcmp(ltype, "active") == 0) {
	NNTPListActive(conn, pptr);
    } else if (strcmp(ltype, "active.times") == 0) {
	NNTPListActiveTimes(conn, pptr);
    } else if (strcmp(ltype, "headers") == 0) {
	NNTPListHeaders(conn, pptr);
    } else if (strcmp(ltype, "newsgroups") == 0) {
	NNTPListNewsgroups(conn, pptr);
    } else if (strcmp(ltype, "extensions") == 0) {
	NNTPListExtensions(conn, pptr);
    } else if (strcmp(ltype, "distributions") == 0) {
	NNTPListDistributions(conn, pptr);
    } else if (strcmp(ltype, "distrib.pats") == 0) {
	NNTPListDistribPats(conn, pptr);
    } else if (strcmp(ltype, "overview.fmt") == 0) {
	NNTPListOverviewFmt(conn, pptr);
    } else if (strcmp(ltype, "subscriptions") == 0) {
	NNTPListSubscriptions(conn, pptr);
    } else if (strcmp(ltype, "moderators") == 0) {
	NNTPListModerators(conn, pptr);
    } else {
	NNBadCommandUse(conn);
    }
}

void
NNTPListExtensions(Connection *conn, char **pptr)
{
    MBLogPrintf(conn, &conn->co_TMBuf, "202 Extensions supported:\r\n");
    MBLogPrintf(conn, &conn->co_TMBuf, "HDR\r\n");
    MBLogPrintf(conn, &conn->co_TMBuf, "OVER\r\n");
    MBLogPrintf(conn, &conn->co_TMBuf, ".\r\n");
}

void 
NNTPListActive(Connection *conn, char **pptr)
{
    const char *wc = NULL;

    if (pptr)
	wc = parseword(pptr, " \t");	/* may also be NULL */
    else
	wc = "*";

    MBLogPrintf(conn, &conn->co_TMBuf, "215 Newsgroups in form \"group high low flags\".\r\n");

    NNStartListActiveScan(conn, wc, NULL, COM_ACTIVE);
}

void
NNStartListActiveScan(Connection *conn, const char *wc, TimeRestrict *tr, int mode)
{
    /*
     * Setup list pattern
     */

    zfreeStr(&conn->co_MemPool, &conn->co_ListPat);
    if (wc)
	conn->co_ListPat = zallocStr(&conn->co_MemPool, wc);
    if (tr)
	conn->co_TimeRestrict = *tr;

    /*
     * Optimize NEWGROUPS requests via active file cache
     */

    if ((mode == COM_NEWGROUPS) && tr) {
	if (ActiveCacheValid(KDBActive) && (ActiveCacheReadLock() == 0)) {
	    conn->co_ListCacheMode = ACMODE_READ;
	    conn->co_ListCachePtr = ActiveCacheFind(activeCache, tr->tr_Time);
	    conn->co_ListCacheGroups = conn->co_ListCachePtr ?
		conn->co_ListCachePtr->nglist : NULL;
	} else {
	    if (ActiveCacheWriteLock() == 0) {
		ActiveCacheFreeMain();
		conn->co_ListCacheMode = ACMODE_WRITE;
	    }
	}
    }

    /*
     * Initiate active file scan, but optimize if specific group requested
     */

    if (wc == NULL || strchr(wc, '*') || strchr(wc, '?')) {
	conn->co_ListRec = (conn->co_ListCacheMode == ACMODE_READ) ? 0 :
			   KPDBScanFirst(KDBActive, 0, &conn->co_ListRecLen);
	conn->co_Flags |= COF_PATISWILD;
    } else {
	conn->co_ListRec = (conn->co_ListCacheMode == ACMODE_READ) ? 0 :
		   KPDBReadRecordOff(KDBActive, wc, 0, &conn->co_ListRecLen);
	conn->co_Flags &= ~COF_PATISWILD;
    }
    conn->co_ArtMode = mode;
    NNListActiveScan(conn);
}

void
NNListActiveScan(Connection *conn)
{
    struct GroupList *groups = conn->co_Auth.dr_ListGroupDef->gr_Groups;

    conn->co_Func = NNListActiveScan;
    conn->co_State = "listac";

    while ((conn->co_TMBuf.mh_Bytes < MBUF_HIWAT) &&
	   (((conn->co_ListCacheMode != ACMODE_READ) &&
	     conn->co_ListRec) || 
	    ((conn->co_ListCacheMode == ACMODE_READ) &&
	     conn->co_ListCacheGroups))) {
	int glen;
	const char *group;
	char grpbuf[MAXGNAME];
	const char *rec;

	if (conn->co_ListCacheMode == ACMODE_READ) {
	    char *grpname = conn->co_ListCacheGroups->group;
	    rec = KPDBReadRecord(KDBActive, grpname, 0, &conn->co_ListRecLen);
	} else {
	    rec = KPDBReadRecordAt(KDBActive, conn->co_ListRec, 0, NULL);
	}

	group = rec ?
		KPDBGetField(rec, conn->co_ListRecLen, NULL, &glen, NULL) :
		"";

	if (*group && (glen < MAXGNAME)) {
	    bcopy(group, grpbuf, glen);
	    grpbuf[glen] = 0;

	    if ((conn->co_ListPat == NULL || 
		WildCmp(conn->co_ListPat, grpbuf) == 0) &&
		(groups == NULL || GroupFindWild(grpbuf, groups))
	    ) {
		/*
		 * note: because we are locking a previously retrieved but unlocked
		 * record, the record may be marked as deleted.  We cannot do 
		 * KPDBWrite()'s with KP_LOCK_CONTINUE in this case because it
		 * may not properly locate the record to continue the lock at.
		 */
		KPDBLock(KDBActive, rec);

		if (conn->co_ArtMode == COM_GROUPDESC) {
		    int glen;
		    const char *desc = KPDBGetField(rec, conn->co_ListRecLen, "GD", &glen, "?");

		    MBPrintf(&conn->co_TMBuf, "%s\t", grpbuf);
		    MBWriteDecode(&conn->co_TMBuf, desc, glen);
		    MBWrite(&conn->co_TMBuf, "\r\n", 2);
		} else if (conn->co_ArtMode == COM_ACTIVE) {
		    int flen;

		    const char *flags = KPDBGetField(rec, conn->co_ListRecLen, "S", &flen, "n");
		    artno_t ne = strtoll(KPDBGetField(rec, conn->co_ListRecLen, "NE", NULL, "0"), NULL, 10);
		    artno_t nb = strtoll(KPDBGetField(rec, conn->co_ListRecLen, "NB", NULL, "0"), NULL, 10);

		    /*
		     * NOTE: we cannot use *.*s because it's broken on most
		     * platforms... it will strlen() the string, and the string
		     * in this case is the entire size of the active file!
		     */

		    MBPrintf(&conn->co_TMBuf, "%s %010lld %010lld ",
			grpbuf,
			artno_ne(nb, ne, conn->co_Numbering),
			artno_nb(nb, ne, conn->co_Numbering)
		    );
		    MBWrite(&conn->co_TMBuf, flags, flen);
		    MBWrite(&conn->co_TMBuf, "\r\n", 2);
		} else if (conn->co_ArtMode == COM_ACTIVETIMES) {
		    const char *cts = KPDBGetField(rec, conn->co_ListRecLen, "CTS", NULL, NULL);

		    if (cts) {
			MBPrintf(&conn->co_TMBuf, "%s %lu unknown\r\n", grpbuf,
			    (unsigned long)strtoul(cts, NULL, 16));
		    }
		} else if (conn->co_ArtMode == COM_NEWGROUPS) {
		    const char *cts = KPDBGetField(rec, conn->co_ListRecLen, "CTS", NULL, NULL);
		    if (cts) {
			int dt, cts_int = (int)strtoul(cts, NULL, 16);

			if (conn->co_ListCacheMode == ACMODE_WRITE)
			    ActiveCacheInsert(&activeCache, grpbuf, cts_int);

			dt = cts_int - (int)conn->co_TimeRestrict.tr_Time;
			if (dt > 0) {
			    int flen;
			    const char *flags = KPDBGetField(rec, conn->co_ListRecLen, "S", &flen, "n");
			    artno_t ne = strtoll(KPDBGetField(rec, conn->co_ListRecLen, "NE", NULL, "0"), NULL, 10);
			    artno_t nb = strtoll(KPDBGetField(rec, conn->co_ListRecLen, "NB", NULL, "0"), NULL, 10);
			    MBPrintf(&conn->co_TMBuf, "%s %lld %lld ",
				grpbuf,
				artno_ne(nb, ne, conn->co_Numbering),
				artno_nb(nb, ne, conn->co_Numbering)
			    );
			    MBWrite(&conn->co_TMBuf, flags, flen);
			    MBWrite(&conn->co_TMBuf, "\r\n", 2);
			}
		    }
		}
		KPDBUnlock(KDBActive, rec);
	    }
	}

	/*
	 * If we are looking for a wildcard, continue the scan.  Otherwise
	 * we are done.
	 */

	if (conn->co_Flags & COF_PATISWILD) {
	    if (conn->co_ListCacheMode == ACMODE_READ) {
		conn->co_ListCacheGroups = conn->co_ListCacheGroups->next;
		if (conn->co_ListCacheGroups == NULL) {
		    conn->co_ListCachePtr =
			ActiveCacheGetNext(conn->co_ListCachePtr);
		    conn->co_ListCacheGroups = conn->co_ListCachePtr ?
			conn->co_ListCachePtr->nglist : NULL;
		}
	    } else {
		conn->co_ListRec = KPDBScanNext(
		    KDBActive, 
		    conn->co_ListRec, 
		    0, 
		    &conn->co_ListRecLen
		);
	    }
	} else {
	    conn->co_ListRec = 0;
	    conn->co_ListCachePtr = NULL;
	}
    }

    if ((conn->co_ListCacheMode == ACMODE_NONE) && (conn->co_ListRec == 0)) {
	MBPrintf(&conn->co_TMBuf, ".\r\n");
	NNCommand(conn);
	zfreeStr(&conn->co_MemPool, &conn->co_ListPat);
	if (DebugOpt)
	    printf("done\n");
    }
    if ((conn->co_ListCacheMode == ACMODE_READ) &&
	(conn->co_ListCacheGroups == NULL)) {
	ActiveCacheReadUnlock();
	conn->co_ListCacheMode = ACMODE_NONE;
	MBPrintf(&conn->co_TMBuf, ".\r\n");
	NNCommand(conn);
	zfreeStr(&conn->co_MemPool, &conn->co_ListPat);
	if (DebugOpt)
	    printf("done\n");
    }
    if ((conn->co_ListCacheMode == ACMODE_WRITE) && (conn->co_ListRec == 0)) {
	ActiveCacheMarkValid(KDBActive);
	ActiveCacheWriteUnlock();
	conn->co_ListCacheMode = ACMODE_NONE;
	MBPrintf(&conn->co_TMBuf, ".\r\n");
	NNCommand(conn);
	zfreeStr(&conn->co_MemPool, &conn->co_ListPat);
	if (DebugOpt)
	    printf("done\n");
    }
}

/*
 * list active.times [wildmat] (RFC 3977 7.6.4): "group time creator" for
 * every group with a creation time (CTS).  Diablo doesn't record who
 * created a group, so the creator is "unknown".
 */
void 
NNTPListActiveTimes(Connection *conn, char **pptr)
{
    const char *wc = NULL;

    if (pptr)
	wc = parseword(pptr, " \t");	/* may also be NULL */

    MBLogPrintf(conn, &conn->co_TMBuf, "215 Newsgroup creation times follow\r\n");
    NNStartListActiveScan(conn, wc, NULL, COM_ACTIVETIMES);
}

/*
 * list headers [msgid|range] (RFC 3977 8.6): the fields HDR can return.
 * HDR with a range works for any header in the stored article headers
 * (":"); HDR by message-id isn't implemented, so that list is empty.
 */
void
NNTPListHeaders(Connection *conn, char **pptr)
{
    const char *form = pptr ? parseword(pptr, " \t") : NULL;

    if (form != NULL && strcasecmp(form, "msgid") != 0 && strcasecmp(form, "range") != 0) {
	MBLogPrintf(conn, &conn->co_TMBuf, "501 list headers [msgid|range]\r\n");
	NNCommand(conn);
	return;
    }
    MBLogPrintf(conn, &conn->co_TMBuf, "215 Field list follows\r\n");
    if (form == NULL || strcasecmp(form, "range") == 0)
	MBPrintf(&conn->co_TMBuf, ":\r\n");
    MBPrintf(&conn->co_TMBuf, ".\r\n");
    NNCommand(conn);
}

/*
 * list newsgroups grouppat
 */

void 
NNTPListNewsgroups(Connection *conn, char **pptr)
{
    const char *wc = NULL;

    if (pptr)
	wc = parseword(pptr, " \t");	/* may also be NULL */

    MBLogPrintf(conn, &conn->co_TMBuf, "215 descriptions in form \"group description\"\r\n");

    NNStartListActiveScan(conn, wc, NULL, COM_GROUPDESC);
}

void
NNTPXGTitle(Connection *conn, char **pptr)
{
    const char *wc = NULL;

    if (pptr)
	wc = parseword(pptr, " \t");	/* may also be NULL */

    MBLogPrintf(conn, &conn->co_TMBuf, "282 list follows\r\n");

    NNStartListActiveScan(conn, wc, NULL, COM_GROUPDESC);
}

void 
NNTPListDistributions(Connection *conn, char **pptr)
{
    FILE *fi;
    MBLogPrintf(conn, &conn->co_TMBuf, "215 Distributions in form \"area description\".\r\n");

    if ((fi = fopen(PatLibExpand(DistributionsPat), "r")) != NULL) {
	char buf[256];

	while (fgets(buf, sizeof(buf), fi) != NULL) {
	    int l = strlen(buf);
	    if (l > 0 && buf[l-1] == '\n') {
		buf[--l] = 0;
		if (l > 0 && buf[l-1] == '\r')
		    buf[--l] = 0;
	    }
	    if (buf[0] == '.')
		MBWrite(&conn->co_TMBuf, ".", 1);
	    MBWrite(&conn->co_TMBuf, buf, l);
	    MBWrite(&conn->co_TMBuf, "\r\n", 2);
	}
	fclose(fi);
    }
    MBPrintf(&conn->co_TMBuf, ".\r\n");
    NNCommand(conn);
}

void 
NNTPListDistribPats(Connection *conn, char **pptr)
{
    FILE *fi;

    MBLogPrintf(conn, &conn->co_TMBuf, "215 Default distributions in form \"weight:pattern:value\".\r\n");

    if ((fi = fopen(PatLibExpand(DistribDotPatsPat), "r")) != NULL) {
	char buf[256];

	while (fgets(buf, sizeof(buf), fi) != NULL) {
	    int l = strlen(buf);
	    if (l > 0 && buf[l-1] == '\n') {
		buf[--l] = 0;
		if (l > 0 && buf[l-1] == '\r')
		    buf[--l] = 0;
	    }
	    if (buf[0] == '.')
		MBWrite(&conn->co_TMBuf, ".", 1);
	    MBWrite(&conn->co_TMBuf, buf, l);
	    MBWrite(&conn->co_TMBuf, "\r\n", 2);
	}
	fclose(fi);
    }
    MBPrintf(&conn->co_TMBuf, ".\r\n");
    NNCommand(conn);
}

void 
NNTPListOverviewFmt(Connection *conn, char **pptr)
{
    MBLogPrintf(conn, &conn->co_TMBuf, "215 Order of fields in overview database.\r\n");
    MBPrintf(&conn->co_TMBuf, "%s.\r\n", OverViewFmt);
    NNCommand(conn);
}

void 
NNTPListSubscriptions(Connection *conn, char **pptr)
{
    MBLogPrintf(conn, &conn->co_TMBuf, "215 Subscriptions in form \"group\"\r\n");
    MBPrintf(&conn->co_TMBuf, ".\r\n");
    NNCommand(conn);
}

void
NNTPListModerators(Connection *conn, char **pptr)
{
    FILE *fi;

    MBLogPrintf(conn, &conn->co_TMBuf, "215 Newsgroup moderators in form \"group-pattern:mail-address-pattern\".\r\n");
    if ((fi = fopen(PatLibExpand(ModeratorsPat), "r")) != NULL) {
	char buf[256];
	while (fgets(buf, sizeof(buf), fi) != NULL) {
	    int l = strlen(buf);
	    if (l == 0 || buf[0] == '#' || buf[0] == '\n')
		continue;
	    if (buf[l-1] == '\n')
		buf[--l] = 0;
	    if (buf[0] == '.')
		MBWrite(&conn->co_TMBuf, ".", 1);
	    MBWrite(&conn->co_TMBuf, buf, l);
	    MBWrite(&conn->co_TMBuf, "\r\n", 2);
	}
	fclose(fi);
    }
    MBPrintf(&conn->co_TMBuf, ".\r\n");
    NNCommand(conn);
}

/*
 * LISTGROUP [group [range]] (RFC 3977 6.1.2): select the group as GROUP
 * does (or the current one), answer "211 count low high group" and list
 * the article numbers in range (default: the whole group).
 */
void 
NNTPListGroup(Connection *conn, char **pptr)
{
    char *group = parseword(pptr, " \t");
    char *range = (group != NULL) ? parseword(pptr, " \t") : NULL;
    artno_t beg;
    artno_t end;

    if (group != NULL) {
	if (strlen(group) >= MAXGNAME || parseword(pptr, " \t") != NULL) {
	    MBLogPrintf(conn, &conn->co_TMBuf, "501 listgroup [newsgroup [range]]\r\n");
	    NNCommand(conn);
	    return;
	}
	if (SelectGroup(conn, group) < 0) {
	    MBLogPrintf(conn, &conn->co_TMBuf, "411 No such group %s\r\n", group);
	    NNCommand(conn);
	    return;
	}
    } else if (conn->co_GroupName == NULL) {
	MBLogPrintf(conn, &conn->co_TMBuf, "412 No newsgroup selected\r\n");
	NNCommand(conn);
	return;
    } else if (SelectGroup(conn, conn->co_GroupName) < 0) {
	MBLogPrintf(conn, &conn->co_TMBuf, "411 No such group %s\r\n", conn->co_GroupName);
	NNCommand(conn);
	return;
    }

    /* the range, in the client's numbering: n, n- or n-m */
    beg = conn->co_ArtBeg;
    end = conn->co_ArtEnd;
    if (range != NULL) {
	char *p;
	artno_t lo = strtoll(range, &p, 10);
	artno_t hi = lo;

	if (p == range || lo < 0) {
	    MBLogPrintf(conn, &conn->co_TMBuf, "501 Bad range %s\r\n", range);
	    NNCommand(conn);
	    return;
	}
	if (*p == '-') {
	    char *q = p + 1;

	    if (*q == 0) {
		hi = -1;		/* n-: to the end */
		p = q;
	    } else {
		hi = strtoll(q, &p, 10);
		if (p == q || hi < 0)
		    p = q - 1;		/* reject below */
	    }
	}
	if (*p != 0) {
	    MBLogPrintf(conn, &conn->co_TMBuf, "501 Bad range %s\r\n", range);
	    NNCommand(conn);
	    return;
	}
	lo = artno_input(conn->co_ArtBeg, conn->co_ArtEnd, lo, conn->co_Numbering);
	if (lo > beg)
	    beg = lo;
	if (hi >= 0) {
	    hi = artno_input(conn->co_ArtBeg, conn->co_ArtEnd, hi, conn->co_Numbering);
	    if (hi < end)
		end = hi;
	}
    }

    {
	artno_t nb = artno_nb(conn->co_ArtBeg, conn->co_ArtEnd, conn->co_Numbering);
	artno_t ne = artno_ne(conn->co_ArtBeg, conn->co_ArtEnd, conn->co_Numbering);

	MBLogPrintf(conn, &conn->co_TMBuf, "211 %lld %lld %lld %s list follows\r\n",
	    ne - nb + 1, nb, ne, conn->co_GroupName);
    }
    conn->co_ListBegNo = beg;
    conn->co_ListEndNo = end + 1;
    NNListGroup(conn);
}

void
NNListGroup(Connection *conn)
{
    OverInfo *ov = (conn->co_GroupName) ? GetOverInfo(conn->co_GroupName) : NULL;

    conn->co_Func = NNListGroup;
    conn->co_State = "listgr";

    if (ov == NULL)			/* no overview yet: nothing to list */
	conn->co_ListBegNo = conn->co_ListEndNo;
    while (
	conn->co_TMBuf.mh_Bytes < MBUF_HIWAT && 
	conn->co_ListBegNo < conn->co_ListEndNo
    ) {
	int resLen;
        int artSize;

        if (GetOverRecord(ov, conn->co_ListBegNo, &resLen, &artSize, NULL, NULL) != NULL) {
	    MBPrintf(&conn->co_TMBuf, "%lld\r\n",
		artno_art(conn->co_ArtBeg, conn->co_ArtEnd, conn->co_ListBegNo, conn->co_Numbering));
	}
	++conn->co_ListBegNo;
    }
    if (ov != NULL)
	PutOverInfo(ov);
    if (conn->co_ListBegNo < conn->co_ListEndNo) {
	;
    } else {
	MBPrintf(&conn->co_TMBuf, ".\r\n");
	NNCommand(conn);
    }
}

void
NNTPNewgroups(Connection *conn, char **pptr)
{
    const char *yymmdd = NULL;
    const char *hhmmss = NULL;
    const char *gmt = NULL;
    TimeRestrict tr;

    if (pptr) {
	yymmdd = parseword(pptr, " \t");
	hhmmss = parseword(pptr, " \t");
	gmt = parseword(pptr, " \t");
    }
    if (SetTimeRestrict(&tr, yymmdd, hhmmss, gmt) < 0) {
	MBLogPrintf(conn, &conn->co_TMBuf, "501 yymmdd hhmmss [\"GMT\"] [<distributions>]\r\n");
	NNCommand(conn);
    } else {
	MBLogPrintf(conn, &conn->co_TMBuf, "231 New newsgroups follow.\r\n");

	NNStartListActiveScan(conn, "*", &tr, COM_NEWGROUPS);
    }
}   

int
ActiveCacheValid(KPDB *kpdb)
{
    int cur_aval;

    cur_aval = KPDBAppendCount(kpdb);
    return ((cur_aval == activeCache_AppendSeq) &&
            (activeCache != NULL));
}

void
ActiveCacheMarkValid(KPDB *kpdb)
{
    activeCache_AppendSeq = KPDBAppendCount(kpdb);
}

int
ActiveCacheReadLock(void)
{
    if (activeCache_WriteLock != 0)
	return -1;

    activeCache_ReadLock++;
    return 0;
}

void
ActiveCacheReadUnlock(void)
{
    activeCache_ReadLock--;
}

int
ActiveCacheWriteLock(void)
{
    if (activeCache_ReadLock != 0)
	return -1;
    if (activeCache_WriteLock != 0)
	return -1;

    activeCache_WriteLock = 1;
    return 0;
}

void
ActiveCacheWriteUnlock(void)
{
    activeCache_WriteLock = 0;
}

void
ActiveCacheFreeMain(void)
{
    freePool(&activeCache_MemPool);
    activeCache_MemPool = NULL;
    activeCache = NULL;
}

activeCacheEnt *
ActiveCacheFind(activeCacheEnt *ac, int cts)
{
    activeCacheEnt *tryLeft;

    if (ac == NULL)
	return NULL;
    if (cts > ac->cts)
	return ActiveCacheFind(ac->right, cts);

    tryLeft = ActiveCacheFind(ac->left, cts);
    if (tryLeft)
	return tryLeft;
    else
	return ac;
}

activeCacheEnt *
ActiveCacheGetNext(activeCacheEnt *ac)
{
    return ac->next;
}

void
ActiveCacheInsertHelper(activeCacheEnt **acp, activeCacheEnt *parent,
			char *newGroup, int newCts)
{
    if (*acp == NULL) {
	struct activeCacheEnt *newEnt =
	    (struct activeCacheEnt *)zalloc(&activeCache_MemPool,
		sizeof(struct activeCacheEnt));
	*acp = newEnt;
	newEnt->nglist =
	    (struct GroupList *)zalloc(&activeCache_MemPool,
		sizeof(struct GroupList));
	newEnt->nglist->group = zallocStr(&activeCache_MemPool, newGroup);
	newEnt->nglist->next = NULL;
	newEnt->cts = newCts;
	newEnt->left = NULL;
	newEnt->right = NULL;
	newEnt->parent = parent;
	if (newEnt->parent->left == newEnt) {
	    activeCacheEnt *temp;

	    temp = newEnt->parent->prev;
	    newEnt->parent->prev = newEnt;
	    newEnt->next = newEnt->parent;
	    newEnt->prev = temp;
	    if (newEnt->prev)
		newEnt->prev->next = newEnt;
	} else {
	    /* Assume newEnt->parent->right == newEnt */
	    activeCacheEnt *temp;

	    temp = newEnt->parent->next;
	    newEnt->parent->next = newEnt;
	    newEnt->prev = newEnt->parent;
	    newEnt->next = temp;
	    if (newEnt->next)
		newEnt->next->prev = newEnt;
	}
    } else {
	if (newCts == (*acp)->cts) {
	    struct GroupList *newList =
		(struct GroupList *)zalloc(&activeCache_MemPool,
		    sizeof(struct GroupList));
	    newList->next = (*acp)->nglist;
	    newList->group = zallocStr(&activeCache_MemPool, newGroup);
	    (*acp)->nglist = newList;
	} else {
	    if (newCts > (*acp)->cts)
		ActiveCacheInsertHelper(&((*acp)->right), *acp,
					newGroup, newCts);
	    else
		ActiveCacheInsertHelper(&((*acp)->left), *acp,
					newGroup, newCts);
	}
    }
}

void
ActiveCacheInsert(activeCacheEnt **acp, char *newGroup, int newCts)
{
    if (*acp == NULL) {
	*acp = (struct activeCacheEnt *)zalloc(&activeCache_MemPool,
	    sizeof(activeCacheEnt));
	(*acp)->nglist = (struct GroupList *)zalloc(&activeCache_MemPool,
	    sizeof(struct GroupList));
	(*acp)->nglist->group = zallocStr(&activeCache_MemPool, newGroup);
	(*acp)->nglist->next = NULL;
	(*acp)->cts = newCts;
	(*acp)->left = NULL;
	(*acp)->right = NULL;
	(*acp)->next = NULL;
	(*acp)->prev = NULL;
	(*acp)->parent = NULL;
    } else {
	ActiveCacheInsertHelper(acp, NULL, newGroup, newCts);
    }
}

/*
 * Return a list of all newsgroups matching a pattern
 *
 * NOTE that the returned list is not in the same order as the
 * comma-separated pattern
 */
GroupList *
ListActiveGroups(Connection *conn, char *pat)
{
    int recLen;
    int recOff;
    GroupList *patlist = NULL;
    GroupList *gl = NULL;
    GroupList *allowedgroups = conn->co_Auth.dr_GroupDef->gr_Groups;
    char *ng;
 
    /*
     * Create a list of the patterns
     */
    for (ng = strtok(pat, ","); ng; ng = strtok(NULL, ",")) {
	GroupList *tgl;
	int wild = (strchr(ng, '*') || strchr(ng, '?'));

	if (allowedgroups != NULL && !wild &&
					GroupFindWild(ng, allowedgroups) == 0)
	    continue;

	tgl = zalloc(&conn->co_MemPool, sizeof(GroupList));
	tgl->group = zallocStr(&conn->co_MemPool, ng);
	tgl->next = NULL;
	if (wild) {
	    tgl->next = patlist;
	    patlist = tgl;
	} else {
	    tgl->next = conn->co_ListCacheGroups;
	    conn->co_ListCacheGroups = tgl;
	}
    }
    for (recOff = KPDBScanFirst(KDBActive, 0, &recLen);
	recOff;
	recOff = KPDBScanNext(KDBActive, recOff, 0, &recLen)
    ) {
	int groupLen; 
	char grpbuf[MAXGNAME];
	const char *rec = KPDBReadRecordAt(KDBActive, recOff, 0, NULL);
	const char *group = KPDBGetField(rec, recLen, NULL, &groupLen, NULL);
	if (group && groupLen < MAXGNAME) {
	    bcopy(group, grpbuf, groupLen);
	    grpbuf[groupLen] = 0;
	    for (gl = patlist; gl; gl = gl->next) {
		if (WildCmp(gl->group, grpbuf) == 0) {
		    GroupList *tgl;

		    if (allowedgroups != NULL &&
				GroupFindWild(grpbuf, allowedgroups) == 0) {
			continue;
		    }

		    tgl = zalloc(&conn->co_MemPool, sizeof(GroupList));
		    tgl->group = zallocStr(&conn->co_MemPool, grpbuf);
		    tgl->next = NULL;
		    tgl->next = conn->co_ListCacheGroups;
		    conn->co_ListCacheGroups = tgl;
		}
	    }
	}
    }
    while (patlist) {
	gl = patlist;
	patlist = patlist->next;
	zfreeStr(&conn->co_MemPool, &gl->group);
	zfree(&conn->co_MemPool, gl, sizeof(GroupList));
    }
    return(conn->co_ListCacheGroups);
}

