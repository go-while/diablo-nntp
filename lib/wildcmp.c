
/*
 * WILDCMP.C
 *
 * (c)Copyright 1997, Matthew Dillon, All Rights Reserved.  Refer to
 *    the COPYRIGHT file in the base directory of this distribution 
 *    for specific rights granted.
 *
 */

#include "defs.h"

Prototype int WildCmp(const char *w, const char *s);
Prototype int WildCaseCmp(const char *w, const char *s);

/*
 * WildCmp() - compare wild string to sane string
 *
 */

int
WildCmp(const char *w, const char *s)
{
    const char *sw = NULL;	/* pattern after the last '*' */
    const char *ss = NULL;	/* where that '*' currently ends in s */

    /*
     * Only the most recent '*' needs to be retried (any earlier one could
     * be satisfied by it), so the work is bounded by strlen(w) * strlen(s).
     */

    for (;;) {
	switch(*w) {
	case '*':
	    if (w[1] == 0)	/* optimize wild* case */
		return(0);
	    sw = ++w;
	    ss = s;
	    continue;
	case '?':
	    if (*s != 0) {
		++w;
		++s;
		continue;
	    }
	    break;
	default:
	    if (*w == *s) {
		if (*w == 0)	/* terminator */
		    return(0);
		++w;
		++s;
		continue;
	    }
	    break;
	}

	/*
	 * mismatch, let the last '*' absorb one more character
	 */
	if (sw == NULL || *ss == 0)
	    return(-1);
	s = ++ss;
	w = sw;
    }
    /* not reached */
    return(-1);
}


/*
 * WildCaseCmp() - compare wild string to sane string, case insensitive
 */

int
WildCaseCmp(const char *w, const char *s)
{
    const char *sw = NULL;	/* pattern after the last '*' */
    const char *ss = NULL;	/* where that '*' currently ends in s */

    /*
     * Only the most recent '*' needs to be retried (any earlier one could
     * be satisfied by it), so the work is bounded by strlen(w) * strlen(s).
     */

    for (;;) {
	switch(*w) {
	case '*':
	    if (w[1] == 0)	/* optimize wild* case */
		return(0);
	    sw = ++w;
	    ss = s;
	    continue;
	case '?':
	    if (*s != 0) {
		++w;
		++s;
		continue;
	    }
	    break;
	default:
	    if (tolower((unsigned char)*w) == tolower((unsigned char)*s)) {
		if (*w == 0)	/* terminator */
		    return(0);
		++w;
		++s;
		continue;
	    }
	    break;
	}

	/*
	 * mismatch, let the last '*' absorb one more character
	 */
	if (sw == NULL || *ss == 0)
	    return(-1);
	s = ++ss;
	w = sw;
    }
    /* not reached */
    return(-1);
}
