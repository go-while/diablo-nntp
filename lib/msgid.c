
/*
 * LIB/MSGID.C
 *
 * (c)Copyright 1997, Matthew Dillon, All Rights Reserved.  Refer to
 *    the COPYRIGHT file in the base directory of this distribution 
 *    for specific rights granted.
 */

#include "defs.h"

Prototype const char *MsgId(const char *s, const char **badmsgid);
Prototype int MsgIdStrict(const char *s, int len);

/*
 * atext of RFC 5322 3.2.3: letters, digits and these
 */
static int
isAtext(int c)
{
    return((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
	(c >= '0' && c <= '9') || (c != 0 && strchr("!#$%&'*+-/=?^_`{|}~", c) != NULL));
}

/*
 * dotAtomText() - skip a dot-atom-text (RFC 5322 3.2.3: 1*atext *("."
 *		   1*atext)) at *pp, before e.  Returns 0 if there is none
 *		   or a "." is not followed by atext ("a..b", "a.").
 */
static int
dotAtomText(const char **pp, const char *e)
{
    const char *p = *pp;

    for (;;) {
	const char *q = p;

	while (p < e && isAtext((unsigned char)*p))
	    ++p;
	if (p == q)
	    return(0);
	if (p < e && *p == '.') {
	    ++p;
	    continue;
	}
	break;
    }
    *pp = p;
    return(1);
}

/*
 * MsgIdStrict() - is the value of a Message-ID header (s, len bytes, from
 *		   after the colon to the end of the line) a msg-id as RFC
 *		   5536 3.1.3 defines it?  That is "<" id-left "@" id-right
 *		   ">", id-left a dot-atom-text, id-right a dot-atom-text or a
 *		   "[" *mdtext "]" literal, at most 250 octets, only white
 *		   space around it: no comment, no folding, no second "@",
 *		   no ".." or a "." at either end of id-left.  MsgId() is the
 *		   lax check dreaderd and diablo use otherwise.
 */
int
MsgIdStrict(const char *s, int len)
{
    const char *e = s + len;
    const char *p;

    if (e - s >= 2 && e[-2] == '\r' && e[-1] == '\n')
	e -= 2;
    else if (e > s && e[-1] == '\n')
	--e;
    while (s < e && (*s == ' ' || *s == '\t'))
	++s;
    while (e > s && (e[-1] == ' ' || e[-1] == '\t'))
	--e;
    if (e - s < 5 || e - s > MAXMSGIDLEN || *s != '<' || e[-1] != '>')
	return(0);
    p = s + 1;
    --e;
    if (!dotAtomText(&p, e) || p >= e || *p != '@')
	return(0);
    ++p;
    if (p < e && *p == '[') {
	/* mdtext: %d33-61 / %d63-90 / %d94-126 */
	for (++p; p < e && *p >= 33 && *p <= 126 &&
		*p != '>' && *p != '[' && *p != ']' && *p != '\\'; ++p)
	    ;
	if (p >= e || *p != ']')
	    return(0);
	++p;
    } else if (!dotAtomText(&p, e)) {
	return(0);
    }
    return(p == e);
}

/*
 * MsgId() - The message id must begin with a '<', end with a '>', 
 *	     and not contain any embedded '<', space, or TAB.
 *
 *	     Additionally, we require either nul or whitespace terminator
 *	     after the logical end of the message-id.
 *
 *	     WARNING!  Returned storage only good until next MsgId() call!
 *
 *	     If badmsgid is non-NULL, then an attempt is made to write
 *	     the bad message-id into that buffer
 */

const char *
MsgId(const char *s, const char **badmsgid)
{
    int i;
    static char LMsgId[MAXMSGIDLEN + 1];
    const char *badId = "<>";

    if (s == NULL) {
	if (badmsgid)
	    *badmsgid = badId;
	return(badId);
    }

    /*
     * Locate beginning of of message-id
     */
    while (*s && (*s == ' ' || *s == '\t' || *s == '\r' || *s == '\n'))
	++s;
    if (*s != '<') {
	if (badmsgid)
	    *badmsgid = s;
	return(badId);
    }

    /*
     * Locate end of message-id
     */

    for (i = 1; s[i] && s[i] != '>'; ++i) {
	if (s[i] == '<' || s[i] == '\t' || s[i] == ' ' || s[i] == '\r' || s[i] == '\n') {
	    if (badmsgid)
		*badmsgid = s;
	    return(badId);
	}
    }

    if (s[i] != '>') {
	if (badmsgid)
	    *badmsgid = s;
	return(badId);
    }

    /*
     * Last character cannot be a '.'
     */
    if (i > 2 && s[i - 1] == '.') {
	if (badmsgid)
	    *badmsgid = s;
	return(badId);
    }

    /*
     * check length, check for garbage (note: whitespace after msgid 
     * is ok and may indicate a tagged-on comment).
     */

    ++i;
    if (i >= MAXMSGIDLEN || (s[i] && s[i] != ' ' && s[i] != '\t' &&
					s[i] != '\r' && s[i] != '\n')) {
	if (badmsgid)
	    *badmsgid = s;
	return(badId);
    }

    bcopy(s, LMsgId, i);
    LMsgId[i] = 0;
    if (badmsgid)
	*badmsgid = LMsgId;
    return(LMsgId);
}

