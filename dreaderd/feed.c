
/*
 * FEED.C
 *
 *	Feed-related NNTP commands.
 *
 * (c)Copyright 1998, Matthew Dillon, All Rights Reserved.  Refer to
 *    the COPYRIGHT file in the base directory of this distribution
 *    for specific rights granted.
 */

#include "defs.h"

Prototype void NNTPMode(Connection *conn, char **pptr);
Prototype void NNTPIHave(Connection *conn, char **pptr);
Prototype void NNTPCheck(Connection *conn, char **pptr);
Prototype void NNTPTakeThis(Connection *conn, char **pptr);
Prototype void NNDiscardArticleStart(Connection *conn, const char *reply);
Prototype void NNTPPost(Connection *conn, char **pptr);
Prototype int ValidXRef(const char *buf, int len);

void NNAcceptArticleStart(Connection *conn, const char *msgid);
void NNAcceptArticle(Connection *conn);

void DebugData(const char *h, const void *buf, int n);

/*
 * mode stream
 * mode reader
 * mode headfeed
 */

void 
NNTPMode(Connection *conn, char **pptr)
{
    char *mode = parseword(pptr, " \t");
    int ok = 0;

    if (mode) {
	if (strcasecmp(mode, "stream") == 0) {
	    /*
	     * CHECK and TAKETHIS exist only in transit mode (COF_SERVER), so
	     * streaming is only offered there (RFC 4644 2.3: 501 otherwise,
	     * and CAPABILITIES lists STREAMING only there).
	     */
	    if (conn->co_Flags & COF_SERVER) {
		conn->co_Flags |= COF_STREAM;
		MBLogPrintf(conn, &conn->co_TMBuf, "203 StreamOK.\r\n");
	    } else {
		MBLogPrintf(conn, &conn->co_TMBuf, "501 MODE STREAM not available in reader mode\r\n");
	    }
	    ok = 1;
	}
	if (strcasecmp(mode, "headfeed") == 0) {
	    if (conn->co_Flags & COF_SERVER) {
		conn->co_Flags |= COF_HEADFEED;
		MBLogPrintf(conn, &conn->co_TMBuf, "250 Mode Command OK.\r\n");
	    } else {
		MBLogPrintf(conn, &conn->co_TMBuf, "500 Mode Command Failed, no server access.\r\n");
	    }
	    ok = 1;
	}
	if (strcasecmp(mode, "reader") == 0) {
	    if (conn->co_Auth.dr_Flags & DF_FEEDONLY) {
		MBLogPrintf(conn, &conn->co_TMBuf, "500 Mode Command Failed, feed-only connection.\r\n");
		ok = 1;
	    } else {
		if (conn->co_Flags & COF_SERVER) {
		    /*
		     * Once out of server mode, you cant get back in and
		     * the reader-mode flags apply.
		     */
		    conn->co_Flags &= ~COF_SERVER;
		}
		NNWriteHello(conn);
		return;
	    }
	}
    }
    if (ok == 0)
	MBLogPrintf(conn, &conn->co_TMBuf, "500 Syntax error or bad command\r\n");
    NNCommand(conn);
}

/*
 *
 */

void 
NNTPIHave(Connection *conn, char **pptr)
{
    if (conn->co_Flags & COF_SERVER) {
	const char *origmsgid = "<>";
	const char *msgid = MsgId(parseword(pptr, ""), &origmsgid);

	if (strcmp(msgid, "<>") == 0) {
	    MBLogPrintf(conn, &conn->co_TMBuf, "435 %s Bad Message-ID\r\n",
								origmsgid);
	    NNCommand(conn);
	    return;
	}

	conn->co_Flags &= ~(COF_IHAVE|COF_POST|COF_POSTTOOBIG|COF_POSTXREF);
	conn->co_Flags |= COF_IHAVE;
	MBLogPrintf(conn, &conn->co_TMBuf, "335\r\n");
	NNAcceptArticleStart(conn, msgid);
    } else {
	MBLogPrintf(conn, &conn->co_TMBuf, "480 Transfer permission denied\r\n");
	NNCommand(conn);
    }
}

void 
NNTPCheck(Connection *conn, char **pptr)
{
    const char *origmsgid = "<>";
    const char *msgid = MsgId(parseword(pptr, " \t"), &origmsgid);

    if (conn->co_Flags & COF_SERVER) {
	if (strcmp(msgid, "<>") == 0)
	    MBLogPrintf(conn, &conn->co_TMBuf, "438 %s\r\n", origmsgid);
	else
	    MBLogPrintf(conn, &conn->co_TMBuf, "238 %s\r\n", msgid);
    } else {
	MBLogPrintf(conn, &conn->co_TMBuf, "500 What?\r\n");
    }
    NNCommand(conn);
}

void 
NNTPTakeThis(Connection *conn, char **pptr)
{
    const char *origmsgid = "<>";
    const char *msgid = MsgId(parseword(pptr, ""), &origmsgid);

    conn->co_Flags &= ~(COF_IHAVE|COF_POST|COF_POSTTOOBIG|COF_POSTXREF);

    /*
     * The article follows the command without waiting for an answer (RFC
     * 4644 2.5.2): a refused TAKETHIS must read and drop it too, or its
     * lines would be taken for commands.  The command table lets TAKETHIS
     * through on a server (transit) connection only.
     */
    if (conn->co_Flags & COF_SERVER) {
	if (strcmp(msgid, "<>") == 0) {
	    /*
	     * 439 with the message-id as sent (its first word, bounded), if
	     * every byte of it is printable US-ASCII; else, or with none, "<>".
	     */
	    char reply[MAXMSGIDLEN + 8];
	    size_t n = strcspn(origmsgid, " \t\r\n");
	    size_t i;

	    if (n > MAXMSGIDLEN)
		n = MAXMSGIDLEN;
	    for (i = 0; i < n; ++i) {
		unsigned char c = (unsigned char)origmsgid[i];

		if (c < 0x21 || c > 0x7e)
		    break;
	    }
	    if (n == 0 || i < n) {
		origmsgid = "<>";
		n = 2;
	    }
	    snprintf(reply, sizeof(reply), "439 %.*s", (int)n, origmsgid);
	    NNDiscardArticleStart(conn, reply);
	} else
	    NNAcceptArticleStart(conn, msgid);
    } else {
	MBLogPrintf(conn, &conn->co_TMBuf, "500 What?\r\n");
	NNCommand(conn);
    }
}

void
NNTPPost(Connection *conn, char **pptr)
{
    char *rcode;
    char *rdesc;

    if (conn->co_Auth.dr_Flags & DF_POST) {
	conn->co_Flags &= ~(COF_IHAVE|COF_POST|COF_POSTTOOBIG|COF_POSTXREF);
	conn->co_Flags |= COF_POST;
	GenerateMessageID(conn);
#ifdef POST_FORCEMSGID
	rcode = "345";
	rdesc = "(forced)";
#else
	rcode = "340";
	rdesc = "(desired)";
#endif
	MBLogPrintf(conn, &conn->co_TMBuf, "340 %s %s %s\r\n",
					rcode, conn->co_IHaveMsgId, rdesc);
	NNAcceptArticleStart(conn, NULL);
    } else {
	PostLogRefused(conn, NULL, "440 Posting Not Allowed.");
	MBLogPrintf(conn, &conn->co_TMBuf, "440 Posting Not Allowed.\r\n");
	NNCommand(conn);
    }
}

void
NNAcceptArticleStart(Connection *conn, const char *msgid)
{
    if (msgid) {
	if (conn->co_IHaveMsgId)
	    zfreeStr(&conn->co_MemPool, &conn->co_IHaveMsgId);
	conn->co_IHaveMsgId = zallocStr(&conn->co_MemPool, msgid);
    }
    conn->co_Flags |= COF_INHEADER;
    conn->co_Flags &= ~COF_WASCONTROL;
    conn->co_Flags |= COF_MAYNOTCLOSE;
    conn->co_ByteCounter = 0.0;
    conn->co_BytesHeader = 0;
    conn->co_LinesHeader = -1;
    conn->co_BodyLines = 0;
    conn->co_PrevCrlf = 1;	/* the command line ended the line before */
    MBFree(&conn->co_ArtBuf);
    NNAcceptArticle(conn);
}

void
NNAcceptArticle(Connection *conn)
{
    char *buf;
    int len = 0;
    int maxl = 1000;
    int bareLF;
    int thisCrlf;
    int prevCrlf;

    conn->co_Func = NNAcceptArticle;
    conn->co_State = "accart";

    /*
     * a line too long is dropped, not buffered, and the article refused
     * once it is complete, the connection stays open.
     */
    conn->co_RMBuf.mh_SkipLong = 1;

    while (--maxl > 0 && (len = MBReadLine(&conn->co_RMBuf, &buf)) > 0) {
	if (conn->co_RMBuf.mh_LongLine) {
	    conn->co_RMBuf.mh_LongLine = 0;
	    conn->co_Flags |= COF_POSTTOOBIG;
	    /* its ending is known once the line is dropped: mh_LongCR */
	    conn->co_PrevCrlf = -1;
	    continue;
	}
	thisCrlf = (len >= 2 && buf[len-2] == '\r');
	prevCrlf = (conn->co_PrevCrlf < 0) ?
		    conn->co_RMBuf.mh_LongCR : conn->co_PrevCrlf;
	conn->co_PrevCrlf = thisCrlf;
	/*
	 * expected EOF, ".\r\n".  A POST also takes ".\n" because some
	 * idiotic newsreaders (like old versions of netscape) don't do the
	 * right thing.  A fed article (IHAVE, TAKETHIS) ends only on CRLF
	 * "." CRLF (RFC 3977 3.1.1): a "." the peer ended with a bare LF,
	 * or one after a bare LF, is data.
	 */
	if ((conn->co_Flags & COF_POST) ?
	    ((len == 3 && strcmp(buf, ".\r") == 0) ||
	     (len == 2 && strcmp(buf, ".") == 0)) :
	    (prevCrlf && len == 3 && strcmp(buf, ".\r") == 0)
	) {
	    if (conn->co_Flags & COF_HEADFEED)	/* can't use bytecounter */
		conn->co_ByteCounter = 0.0;

	    if (conn->co_Flags & COF_POST) {
		NNPostBuffer(conn);
	    } else {
		NNFeedOverview(conn);
	    }
	    conn->co_Flags &= ~COF_MAYNOTCLOSE;
	    conn->co_RMBuf.mh_SkipLong = 0;
	    return;
	}

	/*
	 * We normally unescape dots, but if this article reception is for
	 * a POST command we leave them intact so we do not have to 
	 * re-escape them when the article buffer is passed on.
	 */

	if ((conn->co_Flags & COF_POST) == 0) {
	    if (prevCrlf && len > 0 && buf[0] == '.') {
		++buf;
		--len;
	    }
	}

	/*
	 * end of headers ?
	 * control message ?
	 */

	bareLF = ((conn->co_Flags & (COF_POST|COF_INHEADER)) == (COF_POST|COF_INHEADER) &&
		    (len == 1 || buf[len-2] != '\r'));

	if ((conn->co_Flags & COF_INHEADER) == 0)
	    ++conn->co_BodyLines;

	if (conn->co_Flags & COF_INHEADER) {
	    char ch = tolower((unsigned char)*buf);
	    if (strcmp(buf, "\r") == 0 || len == 1) {	/* CRLF or LF only */
		conn->co_Flags &= ~COF_INHEADER;
	    } else if (ch == 'b' && strncasecmp(buf, "Bytes:", 6) == 0) {
		/*
		 * save received Bytes: header (for header-only feed), but
		 * do not write the header out.  dnewslink -H sends
		 * "Bytes: <size> <lines>": the body line count for the
		 * overview's :lines; an older one sends the size alone.
		 */
		char *e;
		long l;

		conn->co_BytesHeader = strtol(buf + 6, &e, 10);
		while (*e == ' ' || *e == '\t')
		    ++e;
		if (*e >= '0' && *e <= '9' &&
		    (l = strtol(e, NULL, 10)) >= 0 && l < 0x7FFFFFFF)
		    conn->co_LinesHeader = (int)l;
		len = 0;
	    } else if (ch == 'c' && strncasecmp(buf, "Control:", 8) == 0) {
		conn->co_Flags |= COF_WASCONTROL;
	    } else if (ch == 'x' && strncasecmp(buf, "Xref:", 5) == 0) {
		/*
		 * strip XRef: unless it's one we want.  A post must not
		 * have one at all (RFC 5537 3.5): NNPostBuffer refuses it.
		 */
		if (conn->co_Flags & COF_POST) {
		    conn->co_Flags |= COF_POSTXREF;
		    len = 0;
		} else if (ValidXRef(buf, len) < 0)
		    len = 0;
	    }
	}

	/*
	 * buffer the article, but don't bother buffering the article body
	 * for a fed article.  BUT, if it's a Control: message, we need the
	 * whole body so we can pgp-verify it
	 */

	conn->co_Auth.dr_PostBytes += len;
	conn->co_ByteCounter += len;

	if (DOpts.ReaderMaxArtSize > 0 && conn->co_ArtBuf.mh_Bytes > DOpts.ReaderMaxArtSize)
	    conn->co_Flags |= COF_POSTTOOBIG;

	else if (conn->co_Flags & COF_POSTTOOBIG)
	    ;

	else if ((conn->co_Flags & COF_INHEADER) || (conn->co_Flags & (COF_POST|COF_WASCONTROL))) {
	    if (len > 0 && bareLF) {
		/*
		 * a header line ended by LF alone is stored with CR LF, so
		 * the checks on the posted article see each line on its own
		 */
		MBWrite(&conn->co_ArtBuf, buf, len - 1);
		buf = "\r\n";
		len = 2;
	    } else if (len > 0) {
		buf[len-1] = '\n';
	    }
	    MBWrite(&conn->co_ArtBuf, buf, len);
	}
    }
    if (len <= 0) {
	if (len < 0)
	    NNTerminate(conn);
    } else {
	/*
	 * more lines were pending, wake us up immediately, after we've
	 * processed some other people.
	 */
	FD_SET(conn->co_Desc->d_Fd, &WFds);
    }
}

/*
 * NNDiscardArticleStart() and its state NNDiscardArticle() - read the article
 * that follows a refused TAKETHIS up to its terminating dot and throw it
 * away, then answer reply (500 if none is kept).  The peer
 * sends the article without waiting for an answer (RFC 4644 2.5.2): if it
 * were not read here, its lines would be taken for commands.  The answer is
 * kept with the connection (a reader process serves many at once).  The
 * article ends as an accepted one does (NNAcceptArticle()): only on CRLF
 * "." CRLF, a "." after a bare LF or ended by one is data.
 */

static void
NNDiscardArticle(Connection *conn)
{
    char *buf;
    int len = 0;
    int maxl = 1000;
    int prevCrlf;

    conn->co_Func = NNDiscardArticle;
    conn->co_State = "discard";
    conn->co_RMBuf.mh_SkipLong = 1;

    while (--maxl > 0 && (len = MBReadLine(&conn->co_RMBuf, &buf)) > 0) {
	if (conn->co_RMBuf.mh_LongLine) {
	    conn->co_RMBuf.mh_LongLine = 0;
	    conn->co_PrevCrlf = -1;	/* as in NNAcceptArticle() */
	    continue;
	}
	prevCrlf = (conn->co_PrevCrlf < 0) ?
		    conn->co_RMBuf.mh_LongCR : conn->co_PrevCrlf;
	conn->co_PrevCrlf = (len >= 2 && buf[len-2] == '\r');
	if (prevCrlf && len == 3 && strcmp(buf, ".\r") == 0) {
	    conn->co_Flags &= ~COF_MAYNOTCLOSE;
	    conn->co_RMBuf.mh_SkipLong = 0;
	    MBLogPrintf(conn, &conn->co_TMBuf, "%s\r\n",
		conn->co_DiscardReply ? conn->co_DiscardReply : "500 What?");
	    zfreeStr(&conn->co_MemPool, &conn->co_DiscardReply);
	    NNCommand(conn);
	    return;
	}
    }
    if (len <= 0) {
	if (len < 0)
	    NNTerminate(conn);
    } else {
	FD_SET(conn->co_Desc->d_Fd, &WFds);
    }
}

void
NNDiscardArticleStart(Connection *conn, const char *reply)
{
    char note[] = "dropping the article of a refused TAKETHIS";

    zfreeStr(&conn->co_MemPool, &conn->co_DiscardReply);
    conn->co_DiscardReply = zallocStr(&conn->co_MemPool, reply);
    conn->co_Flags |= COF_MAYNOTCLOSE;
    conn->co_PrevCrlf = 1;	/* the command line ended the line before */
    /* with the command log (readerdetaillog, logcmd): tests wait for this */
    LogCmd(conn, '-', note);
    NNDiscardArticle(conn);
}

/*
 * Return 0 on success, -1 if XRefSlaveHost is disabled, -2
 * If the XRef is invalid.
 */

int
ValidXRef(const char *buf, int len)
{
    /*
     * strip XRef: unless it's one we want
     */
    int l;
    int lp;
    const char *p;

    if (DOpts.ReaderXRefSlaveHost == NULL)
	return(-1);

    l = strlen(DOpts.ReaderXRefSlaveHost);
    p = buf + 5;
    lp = len - 5;

    while (lp > 0 && *p == ' ') {
	--lp;
	++p;
    }
    if (lp <= l || 
	strncasecmp(DOpts.ReaderXRefSlaveHost, p, l) != 0 ||
	p[l] != ' '
    ) {
	return(-2);
    }
    return(0);
}

