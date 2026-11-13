
/*
 * DILOAD.C	
 *
 *	Add records to the history database given information on stdin
 *	in the 'didump' format.  Generally used to recover or transfer
 *	a history file between two machines.
 *
 *	To recover a blown up history file:
 *
 *		rm -f dhistory.new
 *		didump dhistory | diload -f dhistory.new
 *		if ( $status == 0) then
 *		    mv -f dhistory.new dhistory
 *		endif
 *
 *	To transfer between machines:
 *
 *		rm -f dhistory.new
 *		rsh remote -n "/news/dbin/didump /news/dhistory" | \
 *		    diload -f dhistory.new
 *		if ( $status == 0) then
 *		    mv -f dhistory.new dhistory
 *		endif
 *
 * (c)Copyright 1997, Matthew Dillon, All Rights Reserved.  Refer to
 *    the COPYRIGHT file in the base directory of this distribution 
 *    for specific rights granted.
 */

#include "defs.h"

int HistoryEntryValid(History *h);
static int dumpRest(const char *p, History *h, char *f1);

void
Usage(void)
{
    printf("Loads entries from stdin into dhistory - usually the output from didump\n\n");
    printf("Usage: diload [-f] [-h hsize] [-u] [-v] [-C diablo.config] [-d[n]] [-V] historyfile\n");
    printf("  where:\n");
    printf("\t-f\t\tfast mode - don't do any locking of history\n");
    printf("\t-h hsize\tspecify the number of entries in the hash table\n");
    printf("\t-u\t\tunconditional - override check for an existing history\n");
    printf("\t-v\t\tbe slightly more verbose and show progress\n");
    printf("\t-C file\tspecify diablo.config to use\n");
    printf("\t-d[n]\tset debug [with optional level]\n");
    printf("\t-V\tprint version and exit\n");
    exit(1);
}

int
main(int ac, char **av)
{
    int r = 1;
    int count = 0;
    int failed = 0;
    int inputError = 0;
    char buf[1024];
    int flags = 0;
    int uflag = 0;
    int verbose = 0;
    char *fileName = NULL;
    int totalentries = 0;

    LoadDiabloConfig(ac, av);

    {
	int i;

	for (i = 1; i < ac; ++i) {
	    char *ptr = av[i];

	    if (*ptr != '-') {
		if (fileName != NULL) {
		    fprintf(stderr, "Unexpected argument\n");
		    Usage();
		}
		fileName = ptr;
		continue;
	    }
	    ptr += 2;
	    switch(ptr[-1]) {
	    case 'f':
		flags |= HGF_FAST | HGF_NOSEARCH;
		break;
	    case 'h':
		NewHSize = strtol(((*ptr) ? ptr : av[++i]), &ptr, 0);
		if (*ptr == 'k' || *ptr == 'K')
		    NewHSize *= 1024;
		if (*ptr == 'm' || *ptr == 'M')
		    NewHSize *= 1024 * 1024;
		if ((NewHSize ^ (NewHSize - 1)) != (NewHSize << 1) - 1) {
		    fprintf(stderr, "specified history size is not a power of 2\n");
		    exit(1);
		}
		break;
	    case 'u':
		uflag = 1;
		break;
	    case 'v':
		verbose = 1;
		break;
	    /* Common options */
	    case 'C':		/* parsed by LoadDiabloConfig */
		if (*ptr == 0)
		    ++i;
		break;
	    case 'd':
		DebugOpt = 1;
		if (*ptr)
		    DebugOpt = strtol(ptr, NULL, 0);
		break;
	    case 'V':
		PrintVersion();
		break;
	    default:
		Usage();
		break;
	    }
	}
    }

    if (fileName == NULL)
	Usage();

    if (flags & HGF_FAST) {
	struct stat st;

	if (stat(fileName, &st) == 0 && uflag == 0) {
	    fprintf(stderr, "-f history files may not previously exist unless you also specify -u\n");
	    fprintf(stderr, "WARNING! -f -u is NOT suggested!\n");
	    Usage();
	}
	if (uflag)
	    flags &= ~HGF_NOSEARCH;
    }

    HistoryOpen(fileName, flags);

    while (fgets(buf, sizeof(buf), stdin) != NULL) {
	History h = { 0 };
	int n;
	int iter = 0;
	int exp = 0;
	int pos = 0;
	char f1 = 0;

	if (buf[0] == '.') {
	    r = 0;
	    break;
	}

	n = sscanf(buf, "DUMP %x.%x.%x gm=%d ex=%d%n",
	    &h.hv.h1,
	    &h.hv.h2,
	    &iter,
	    &h.gmt,
	    &exp,
	    &pos
	);
	if (n == 5 && dumpRest(buf + pos, &h, &f1) == 0) {
	    int r = 0;

	    h.iter = iter;
	    h.exp = exp;
	    if (f1 == 'H')
		h.exp |= EXPF_HEADONLY;
	    if (HistoryEntryValid(&h) < 0)
		++failed;
	    else if ((r = HistoryAdd(NULL, &h)) == 0)
		++count;
	    else if (r != RCTRYAGAIN)
		++failed;
	    else {
		fprintf(stderr, "HistoryAdd: write failed!\n");
		exit(1);
	    }
	} else if (sscanf(buf, "ENTRIES %d", &totalentries) == 1) {
	    ;
	} else {
	    fprintf(stderr, "Format error: %s", buf);
	    inputError = 1;
	    break;
	}
	if (verbose && ((count + failed) & 1023) == 0) {
	    if (totalentries > 0)
		printf("%d/%d/%d\r", count, count + failed, totalentries);
	    else
		printf("%d/%d\r", count, count + failed);
	    fflush(stdout);
	}
    }
    if (ferror(stdin)) {
	fprintf(stderr, "diload: read error on input\n");
	inputError = 1;
    }
    printf("diload: %d/%d entries loaded\n", count, count + failed);
    /*
     * A line we could not read, or a read error, must show in the exit
     * status whatever HistoryClose() says: the caller is about to move
     * this file over the live history.  Input that simply ends without
     * the "." line stays valid, as before.
     */
    r = HistoryClose();
    if (r == RCOK && inputError == 0)
	return(0);
    else
	return(1);
}

/*
 * dumpRest() - check what follows "ex=N" in a DUMP line and take its
 * fields.  A line is old style (didump -o: nothing more) or new style:
 * boff= and bsize=, then any of file="..." (didump -v), flags= (hand made
 * lists leave it out) and next=, as didump writes them.  Anything else,
 * a line that stops inside a field included, is a format error (-1):
 * sscanf() alone took a line cut after boff= as an old style one, and
 * anything at all after bsize=.  A line may end in CR LF.
 *
 * A complete line is taken as it is, also as the last one without its
 * newline: a dump cut off right after a field (ex=, bsize=, flags=, a
 * quoted file=) or inside a number cannot be told from a complete
 * shorter line.
 */
static int
dumpRest(const char *p, History *h, char *f1)
{
    int boff, bsize, pos = 0;

    p += strspn(p, " \t\r\n");
    if (*p == 0)
	return(0);
    if (sscanf(p, "boff=%d bsize=%d%n", &boff, &bsize, &pos) != 2 || pos == 0)
	return(-1);
    p += pos;
    while (*p == ' ' || *p == '\t') {
	p += strspn(p, " \t");
	if (strncmp(p, "file=\"", 6) == 0) {
	    const char *q = strchr(p + 6, '"');

	    if (q == NULL)
		return(-1);
	    p = q + 1;
	} else if (strncmp(p, "flags=", 6) == 0) {
	    p += 6;
	    if (*p == 'H') {
		*f1 = 'H';
		++p;
	    }
	} else if (strncmp(p, "next=", 5) == 0) {
	    p += 5;
	    if (*p == '-')
		++p;
	    if (strspn(p, "0123456789") == 0)
		return(-1);
	    p += strspn(p, "0123456789");
	}
    }
    p += strspn(p, "\r\n");
    if (*p != 0)
	return(-1);
    h->boffset = boff;
    h->bsize = bsize;
    return(0);
}

int
HistoryEntryValid(History *h)
{
    if (h->gmt == 0)
	return(-1);
    if (h->hv.h1 == 0 && h->hv.h2 == 0)
	return(-1);
    return(0);
}

