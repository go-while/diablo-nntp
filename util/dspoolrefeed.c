/*
 * DSPOOLREFEED.C	Write dnewslink queue files for every article on the
 *			spool, to feed a whole spool to another server (F-03).
 *
 * dspoolrefeed [-C diablo.config] [-a days] [-g groups] [-n lines] [-o prefix]
 *		[-q] [-V] [historyfile]
 *
 * The history holds only a hash of each Message-ID, so the Message-ID (and,
 * for -g, the Newsgroups header) is read from the article itself on the
 * spool, compressed spool objects included.  Each line is what diablo writes
 * to a queue file:
 *
 *	D.xxxxxxxx/B.xxxx <message-id> offset,size[ C<stored size>]
 *
 * Expired entries, header-only entries and articles whose file is gone are
 * left out.  dnewslink -b sends the files; see dspoolrefeed(8).
 */

#include "defs.h"

#define HEADMAX	16384	/* the header part read from each article */

void Usage(void);
int ArticleHeader(const char *path, const History *h, char *buf, int bufSize, long *storeLen);
const char *HeaderValue(const char *head, const char *name, char *out, int outSize);
int GroupsMatch(const char *nglist);
FILE *NextOutput(void);

const char *GroupPats = NULL;	/* -g: comma list, '!' excludes, last match decides */
const char *OutPrefix = NULL;	/* -o */
long LinesPerFile = 0;		/* -n */
int QuietOpt = 0;
FILE *OutFo = NULL;
long OutLines = 0;
int OutPart = 0;

void
Usage(void)
{
    fprintf(stderr, "Write dnewslink queue files for every article on the spool.\n\n");
    fprintf(stderr, "Usage: dspoolrefeed [-C diablo.config] [-a days] [-g groups] [-n lines]\n");
    fprintf(stderr, "                    [-o prefix] [-q] [-V] [historyfile]\n");
    fprintf(stderr, "\t-a days\t\tonly articles received in the last days\n");
    fprintf(stderr, "\t-g groups\tonly articles posted to a matching group: a comma list\n");
    fprintf(stderr, "\t\t\tof wildmats, '!' in front excludes, the last match decides\n");
    fprintf(stderr, "\t-n lines\twith -o, start a new file every lines entries\n");
    fprintf(stderr, "\t-o prefix\twrite prefix (or prefix.0000, .0001, ... with -n),\n");
    fprintf(stderr, "\t\t\tnot standard output\n");
    fprintf(stderr, "\t-q\t\tno counts at the end\n");
    fprintf(stderr, "\thistoryfile\tdefault: path_history of diablo.config\n");
    exit(1);
}

int
main(int ac, char **av)
{
    const char *fileName = NULL;
    long maxAgeMins = -1;
    int fd;
    int i;
    HistHead hh;
    char *hbuf;
    int rsize;
    int n;
    uint32 now;
    long scanned = 0, written = 0, empty = 0, expired = 0, headOnly = 0;
    long tooOld = 0, missing = 0, noMsgId = 0, mismatch = 0, notGroup = 0;
    char *head;

    LoadDiabloConfig(ac, av);

    for (i = 1; i < ac; ++i) {
	char *ptr = av[i];

	if (*ptr != '-') {
	    if (fileName != NULL)
		Usage();
	    fileName = ptr;
	    continue;
	}
	ptr += 2;
	switch(ptr[-1]) {
	case 'C':
	    if (*ptr == 0)
		++i;
	    break;
	case 'a':
	    if (*ptr == 0 && i + 1 >= ac)
		Usage();
	    maxAgeMins = strtol(*ptr ? ptr : av[++i], NULL, 10) * 24 * 60;
	    if (maxAgeMins <= 0)
		Usage();
	    break;
	case 'g':
	    if (*ptr == 0 && i + 1 >= ac)
		Usage();
	    GroupPats = *ptr ? ptr : av[++i];
	    if (strlen(GroupPats) >= 4096) {	/* GroupsMatch()'s copy */
		fprintf(stderr, "dspoolrefeed: -g: the list is longer than 4095 characters\n");
		exit(1);
	    }
	    break;
	case 'n':
	    if (*ptr == 0 && i + 1 >= ac)
		Usage();
	    LinesPerFile = strtol(*ptr ? ptr : av[++i], NULL, 10);
	    if (LinesPerFile <= 0)
		Usage();
	    break;
	case 'o':
	    if (*ptr == 0 && i + 1 >= ac)
		Usage();
	    OutPrefix = *ptr ? ptr : av[++i];
	    break;
	case 'q':
	    QuietOpt = 1;
	    break;
	case 'V':
	    PrintVersion();
	    break;
	default:
	    Usage();
	}
    }
    if (LinesPerFile > 0 && OutPrefix == NULL) {
	fprintf(stderr, "dspoolrefeed: -n needs -o\n");
	exit(1);
    }
    if (fileName == NULL)	/* a copy: the Pat*Expand() buffer is reused */
	fileName = strdup(PatDbExpand(DHistoryPat));

    LoadSpoolCtl(0, 1);

    if ((fd = open(fileName, O_RDONLY)) < 0) {
	fprintf(stderr, "dspoolrefeed: %s: %s\n", fileName, strerror(errno));
	exit(1);
    }
    if (read(fd, &hh, sizeof(hh)) != sizeof(hh) || hh.hmagic != HMAGIC) {
	fprintf(stderr, "dspoolrefeed: %s: not a history file\n", fileName);
	exit(1);
    }
    if (hh.henSize < sizeof(History) || hh.henSize > 4096) {
	fprintf(stderr, "dspoolrefeed: %s: record size %d (expected %d)\n",
	    fileName, (int)hh.henSize, (int)sizeof(History));
	exit(1);
    }
    rsize = hh.henSize;
    if (lseek(fd, (off_t)hh.headSize + (off_t)hh.hashSize * sizeof(HistIndex) + rsize, 0) < 0) {
	fprintf(stderr, "dspoolrefeed: %s: %s\n", fileName, strerror(errno));
	exit(1);
    }
    if ((hbuf = malloc(rsize * 4096)) == NULL || (head = malloc(HEADMAX + 1)) == NULL) {
	fprintf(stderr, "dspoolrefeed: out of memory\n");
	exit(1);
    }
    if ((OutFo = NextOutput()) == NULL)
	exit(1);

    now = time(NULL) / 60;
    while ((n = read(fd, hbuf, rsize * 4096)) > 0) {
	n /= rsize;
	for (i = 0; i < n; ++i) {
	    History h;
	    char path[PATH_MAX];
	    long storeLen = 0;
	    char rel[PATH_MAX];
	    char msgid[MAXMSGIDLEN + 1];
	    char ngbuf[8192];
	    hash_t hv;

	    bcopy(hbuf + i * rsize, &h, sizeof(h));
	    ++scanned;
	    if (h.gmt == 0) {
		++empty;
		continue;
	    }
	    if (H_EXPIRED(h.exp)) {
		++expired;
		continue;
	    }
	    if (h.exp & EXPF_HEADONLY) {
		++headOnly;
		continue;
	    }
	    if (maxAgeMins > 0 && (long)now - (long)h.gmt > maxAgeMins) {
		++tooOld;
		continue;
	    }
	    ArticleFileName(path, sizeof(path), &h, ARTFILE_FILE);
	    if (ArticleHeader(path, &h, head, HEADMAX, &storeLen) < 0) {
		++missing;
		continue;
	    }
	    /*
	     * The <...> part, as diablo hashed it for the history (MsgId()):
	     * a comment or other text after it ("<a@b> (x)") is not part
	     * of the Message-ID.
	     */
	    if (HeaderValue(head, "Message-ID", msgid, sizeof(msgid)) == NULL ||
		strcmp(MsgId(msgid, NULL), "<>") == 0) {
		++noMsgId;
		continue;
	    }
	    snprintf(msgid, sizeof(msgid), "%s", MsgId(msgid, NULL));
	    /* the offset must still hold this history entry's article */
	    hv = hhash(msgid);
	    if (hv.h1 != h.hv.h1 || hv.h2 != h.hv.h2) {
		++mismatch;
		continue;
	    }
	    if (GroupPats != NULL &&
		(HeaderValue(head, "Newsgroups", ngbuf, sizeof(ngbuf)) == NULL ||
		 !GroupsMatch(ngbuf))) {
		++notGroup;
		continue;
	    }
	    ArticleFileName(rel, sizeof(rel), &h, ARTFILE_FILE_REL);
	    if (LinesPerFile > 0 && OutLines >= LinesPerFile) {
		if (fclose(OutFo) != 0 || (OutFo = NextOutput()) == NULL)
		    exit(1);
		OutLines = 0;
	    }
	    if (SpoolCompressed(H_SPOOL(h.exp)))
		fprintf(OutFo, "%s %s %lld,%ld C%ld\n", rel, msgid,
		    (long long)h.boffset, (long)h.bsize, storeLen);
	    else
		fprintf(OutFo, "%s %s %lld,%ld\n", rel, msgid,
		    (long long)h.boffset, (long)h.bsize);
	    ++OutLines;
	    ++written;
	}
    }
    if (n < 0) {
	fprintf(stderr, "dspoolrefeed: %s: %s\n", fileName, strerror(errno));
	exit(1);
    }
    close(fd);
    if (fflush(OutFo) != 0 || ferror(OutFo) || (OutFo != stdout && fclose(OutFo) != 0)) {
	fprintf(stderr, "dspoolrefeed: write error: %s\n", strerror(errno));
	exit(1);
    }
    if (!QuietOpt) {
	fprintf(stderr, "dspoolrefeed: %ld history entries, %ld written", scanned, written);
	if (OutPrefix != NULL && LinesPerFile > 0)
	    fprintf(stderr, " in %d files", OutPart);
	fprintf(stderr, "\n  left out: %ld empty, %ld expired, %ld header-only, %ld older than -a,\n"
	    "            %ld file or article gone, %ld without Message-ID, %ld hash mismatch,\n"
	    "            %ld not in -g groups\n",
	    empty, expired, headOnly, tooOld, missing, noMsgId, mismatch, notGroup);
    }
    return(0);
}

/*
 * NextOutput() - standard output, or the next file of -o: prefix itself, or
 *		  prefix.NNNN with -n.  An existing file is not overwritten.
 */
FILE *
NextOutput(void)
{
    char name[PATH_MAX];
    int fd;
    FILE *fo;

    if (OutPrefix == NULL)
	return(stdout);
    if (LinesPerFile > 0)
	snprintf(name, sizeof(name), "%s.%04d", OutPrefix, OutPart);
    else
	snprintf(name, sizeof(name), "%s", OutPrefix);
    ++OutPart;
    if ((fd = open(name, O_WRONLY|O_CREAT|O_EXCL, 0644)) < 0 ||
	(fo = fdopen(fd, "w")) == NULL) {
	fprintf(stderr, "dspoolrefeed: %s: %s\n", name, strerror(errno));
	if (fd >= 0)
	    close(fd);
	return(NULL);
    }
    return(fo);
}

/*
 * ArticleHeader() - the article's header (up to bufSize bytes, NUL
 *		     terminated, CRs removed) from its spool file: plain or
 *		     compressed, with or without the spool article header.
 *		     *storeLen is the size of the article on disk from the
 *		     spool article header (the C<size> of a compressed
 *		     article's queue line), or h->bsize without one.
 *		     Returns 0, or -1 when the file or the article is gone.
 */
int
ArticleHeader(const char *path, const History *h, char *buf, int bufSize, long *storeLen)
{
    /*
     * The history is in arrival order, so the next entry is usually in
     * the same spool file: keep the last one open.
     */
    static char lastPath[PATH_MAX];
    static int lastFd = -1;
    int fd;
    int n = 0;
    SpoolArtHdr ah;
    char *s, *d;

    if (lastFd < 0 || strcmp(path, lastPath) != 0) {
	if (lastFd >= 0)
	    close(lastFd);
	lastPath[0] = 0;
	if ((lastFd = open(path, O_RDONLY)) < 0)
	    return(-1);
	snprintf(lastPath, sizeof(lastPath), "%s", path);
    }
    fd = lastFd;
    if ((n = pread(fd, &ah, sizeof(ah), h->boffset)) <= 0)
	return(-1);
    if (n != sizeof(ah) ||	/* a short old-format article at the end */
	(uint8)ah.Magic1 != STORE_MAGIC1 || (uint8)ah.Magic2 != STORE_MAGIC2) {
	ah.HeadLen = 0;		/* no spool article header */
	ah.StoreLen = h->bsize;
    } else if (ah.HeadLen > sizeof(ah)) {
	return(-1);
    }
    *storeLen = (long)ah.StoreLen;
    if (SpoolCompressed(H_SPOOL(h->exp))) {
#ifdef USE_ZLIB
	gzFile gzf;
	int dfd;

	/* gzclose() closes its descriptor: give it a copy of ours */
	if (lseek(fd, h->boffset + ah.HeadLen, 0) < 0 || (dfd = dup(fd)) < 0)
	    return(-1);
	if ((gzf = gzdopen(dfd, "r")) == NULL) {
	    close(dfd);
	    return(-1);
	}
	n = gzread(gzf, buf, bufSize);
	gzclose(gzf);
#else
	return(-1);
#endif
    } else {
	int want = h->bsize - ah.HeadLen;

	if (want > bufSize)
	    want = bufSize;
	n = (want > 0) ? pread(fd, buf, want, h->boffset + ah.HeadLen) : 0;
    }
    if (n <= 0)
	return(-1);
    buf[n] = 0;
    /* drop CRs (wire format); stop at the end of the header */
    for (s = d = buf; *s; ++s) {
	if (*s == '\r')
	    continue;
	if (*s == '\n' && d > buf && d[-1] == '\n')
	    break;
	*d++ = *s;
    }
    *d = 0;
    return(0);
}

/*
 * HeaderValue() - the value of header name (folded lines joined, white space
 *		   trimmed) in head, copied to out, or NULL.
 */
const char *
HeaderValue(const char *head, const char *name, char *out, int outSize)
{
    size_t l = strlen(name);
    const char *p = head;

    while (*p) {
	if (strncasecmp(p, name, l) == 0 && p[l] == ':') {
	    int o = 0;

	    p += l + 1;
	    for (;;) {
		while (*p == ' ' || *p == '\t')
		    ++p;
		while (*p && *p != '\n') {
		    if (o < outSize - 1)
			out[o++] = *p;
		    ++p;
		}
		if (*p == '\n' && (p[1] == ' ' || p[1] == '\t')) {
		    if (o < outSize - 1)
			out[o++] = ' ';
		    ++p;
		    continue;
		}
		break;
	    }
	    while (o > 0 && (out[o-1] == ' ' || out[o-1] == '\t'))
		--o;
	    out[o] = 0;
	    return(out);
	}
	if ((p = strchr(p, '\n')) == NULL)
	    break;
	++p;
    }
    return(NULL);
}

/*
 * GroupsMatch() - is one of the groups of nglist selected by -g?  Each group
 * goes through the whole pattern list; the last pattern that matches decides
 * (a '!' pattern excludes).
 */
int
GroupsMatch(const char *nglist)
{
    char groups[8192];
    char pats[4096];
    char *g, *gsave = NULL;

    snprintf(groups, sizeof(groups), "%s", nglist);
    for (g = strtok_r(groups, ", \t", &gsave); g; g = strtok_r(NULL, ", \t", &gsave)) {
	char *p, *psave = NULL;
	int sel = 0;

	snprintf(pats, sizeof(pats), "%s", GroupPats);
	for (p = strtok_r(pats, ",", &psave); p; p = strtok_r(NULL, ",", &psave)) {
	    int neg = (*p == '!');

	    if (WildCmp(p + neg, g) == 0)
		sel = !neg;
	}
	if (sel)
	    return(1);
    }
    return(0);
}
