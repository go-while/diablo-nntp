/*
 * LIB/CANLOCK.C	- Cancel-Lock and Cancel-Key (RFC 8315)
 *
 *	A Cancel-Lock header lists "scheme:c-lock" elements, a Cancel-Key
 *	header "scheme:c-key" elements.  A key matches a lock of the same
 *	scheme when base64(H(c-key)) == c-lock, where c-key is the base64
 *	string itself (RFC 8315 section 3).  Schemes: sha256 (mandatory),
 *	sha512, and sha1 (deprecated, still accepted when verifying).
 *
 *	Verification needs no secret; generating locks and keys (for local
 *	posts) is not done here yet.
 */

#include "defs.h"
#include <openssl/evp.h>
#include <openssl/crypto.h>

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
 * schemeDigest() - the digest for a scheme name (case-insensitive), or NULL
 * for an unknown scheme.
 */
static const EVP_MD *
schemeDigest(const char *scheme, int len)
{
    if (len == 6 && strncasecmp(scheme, "sha256", 6) == 0)
	return(EVP_sha256());
    if (len == 6 && strncasecmp(scheme, "sha512", 6) == 0)
	return(EVP_sha512());
    if (len == 4 && strncasecmp(scheme, "sha1", 4) == 0)
	return(EVP_sha1());
    return(NULL);
}

/*
 * CanLockVerify() - 1 when one element of the Cancel-Key value keys
 * matches an element of the same scheme in the Cancel-Lock value locks,
 * else 0.  Unknown schemes and malformed elements never match.
 */
int
CanLockVerify(const char *keys, const char *locks)
{
    const char *kp = keys;
    const char *k;
    int klen;

    while ((k = nextElement(&kp, &klen)) != NULL) {
	const char *colon = memchr(k, ':', klen);
	const EVP_MD *md;
	unsigned char hash[EVP_MAX_MD_SIZE];
	unsigned int hashLen = 0;
	char b64[2 * EVP_MAX_MD_SIZE + 4];
	int b64Len;
	const char *lp = locks;
	const char *l;
	int llen;
	int slen;

	if (colon == NULL || colon == k || colon == k + klen - 1)
	    continue;
	slen = colon - k;
	if ((md = schemeDigest(k, slen)) == NULL)
	    continue;
	/* the lock is the hash of the key's base64 string itself */
	if (EVP_Digest(colon + 1, k + klen - colon - 1, hash, &hashLen, md, NULL) != 1)
	    continue;
	b64Len = EVP_EncodeBlock((unsigned char *)b64, hash, hashLen);

	while ((l = nextElement(&lp, &llen)) != NULL) {
	    const char *lcolon = memchr(l, ':', llen);
	    int vlen;

	    if (lcolon == NULL || lcolon - l != slen ||
		strncasecmp(l, k, slen) != 0)
		continue;
	    vlen = l + llen - lcolon - 1;
	    if (vlen == b64Len && CRYPTO_memcmp(lcolon + 1, b64, b64Len) == 0)
		return(1);
	}
    }
    return(0);
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
