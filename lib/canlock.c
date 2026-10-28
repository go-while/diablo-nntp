/*
 * LIB/CANLOCK.C	- Cancel-Lock and Cancel-Key (RFC 8315)
 *
 *	A Cancel-Lock header lists "scheme:c-lock" elements, a Cancel-Key
 *	header "scheme:c-key" elements.  The computations (a key matches a
 *	lock of the same scheme when base64(H(c-key)) == c-lock, RFC 8315
 *	section 3.5) are libcanlock's, the library INN uses; this file only
 *	finds the headers in an article and hands libcanlock the elements.
 *	Without CANLOCK_ENABLED (lib/vendor.h) nothing can be verified, so
 *	CanLockVerify() never matches: a locked article is never cancelled
 *	(the safe side) unless readercanlock is 'all'.
 *
 *	libcanlock-hp, which could find and split the header fields, runs a
 *	helper program for every call; that is too much for the reader's event
 *	loop, so the small text handling here stays ours.
 */

#include "defs.h"
#ifdef	CANLOCK_ENABLED
#include <libcanlock-3/canlock.h>
#if CL_API_MAJOR < 3 || (CL_API_MAJOR == 3 && CL_API_MINOR < 3)
#error "libcanlock 3.3.0 or later is needed (cl_verify_multi); or #undef CANLOCK_ENABLED in lib/vendor.h"
#endif
#endif

Prototype int CanLockFindHeader(const char *hdrs, int hlen, const char *name, char *buf, int bufLen);
Prototype int CanLockVerify(const char *keys, const char *locks);
Prototype const char *CanLockModeName(int mode);

/*
 * CanLockFindHeader() - copy the value of the first header field called
 * name (case-insensitive) from the header block hdrs (hlen bytes, which may
 * continue into the body: the scan stops at the first empty line).  Folded
 * continuation lines are joined with a space, CRs dropped.  Returns 1 when
 * found, 0 when not, -1 when the value doesn't fit in buf.
 */
int
CanLockFindHeader(const char *hdrs, int hlen, const char *name, char *buf, int bufLen)
{
    int nlen = strlen(name);
    int i = 0;

    while (i < hlen) {
	int e;

	/* end of this physical line */
	for (e = i; e < hlen && hdrs[e] != '\n'; ++e)
	    ;
	if (e == i || (e == i + 1 && hdrs[i] == '\r'))
	    return(0);				/* empty line: end of headers */
	if (e - i > nlen && hdrs[i + nlen] == ':' &&
	    strncasecmp(hdrs + i, name, nlen) == 0) {
	    int n = 0;
	    int p = i + nlen + 1;

	    for (;;) {
		for (; p < e; ++p) {
		    if (hdrs[p] == '\r')
			continue;
		    if (n + 1 >= bufLen)
			return(-1);
		    buf[n++] = hdrs[p];
		}
		/* continuation line? */
		if (e + 1 < hlen && (hdrs[e + 1] == ' ' || hdrs[e + 1] == '\t')) {
		    if (n + 1 >= bufLen)
			return(-1);
		    buf[n++] = ' ';
		    p = e + 1;
		    for (e = p; e < hlen && hdrs[e] != '\n'; ++e)
			;
		    continue;
		}
		break;
	    }
	    buf[n] = 0;
	    return(1);
	}
	i = e + 1;
    }
    return(0);
}

#ifdef	CANLOCK_ENABLED
/*
 * nextElement() - the next "scheme:value" element of a Cancel-Lock or
 * Cancel-Key value, skipping white space and (nested) comments.  Returns a
 * pointer to the element and sets *len, or NULL at the end.
 */
static const char *
nextElement(const char **pp, int *len)
{
    const char *p = *pp;
    const char *s;

    for (;;) {
	while (*p == ' ' || *p == '\t' || *p == '\r' || *p == '\n')
	    ++p;
	if (*p == '(') {
	    int depth = 0;

	    for (; *p; ++p) {
		if (*p == '\\' && p[1]) {
		    ++p;
		} else if (*p == '(') {
		    ++depth;
		} else if (*p == ')' && --depth == 0) {
		    ++p;
		    break;
		}
	    }
	    continue;
	}
	break;
    }
    if (*p == 0) {
	*pp = p;
	return(NULL);
    }
    s = p;
    while (*p && *p != ' ' && *p != '\t' && *p != '\r' && *p != '\n' && *p != '(')
	++p;
    *len = p - s;
    *pp = p;
    return(s);
}

/*
 * normalize() - the elements of a Cancel-Lock or Cancel-Key value in the form
 * libcanlock wants: separated by single spaces, white space and comments
 * gone.  Returns a malloc'ed string (never longer than value), or NULL.
 */
static char *
normalize(const char *value)
{
    char *out = malloc(strlen(value) + 1);
    const char *p = value;
    const char *e;
    int len;
    int n = 0;

    if (out == NULL)
	return(NULL);
    while ((e = nextElement(&p, &len)) != NULL) {
	if (n)
	    out[n++] = ' ';
	memcpy(out + n, e, len);
	n += len;
    }
    out[n] = 0;
    return(out);
}

#endif

/*
 * CanLockVerify() - 1 when one element of the Cancel-Key value keys
 * matches an element of the same scheme in the Cancel-Lock value locks
 * (cl_verify_multi(), RFC 8315 section 3.5), else 0.  Unknown schemes and
 * malformed elements never match.  Always 0 without CANLOCK_ENABLED.
 */
int
CanLockVerify(const char *keys, const char *locks)
{
    int r = 0;
#ifdef	CANLOCK_ENABLED
    char *k = normalize(keys);
    char *l = normalize(locks);

    if (k && l)
	r = (cl_verify_multi(NULL, k, l) == 0);
    free(k);
    free(l);
#endif
    return(r);
}

/*
 * CanLockModeName() - the diablo.config name of a readercanlock mode.
 */
const char *
CanLockModeName(int mode)
{
    switch (mode) {
    case CANLOCK_ALL:
	return("all");
    case CANLOCK_REQUIRE_AUTH:
	return("require-auth");
    case CANLOCK_AUTH:
	return("auth");
    case CANLOCK_NONE:
	return("none");
    }
    return("?");
}
