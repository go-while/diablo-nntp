#include "defs.h"

#if NEED_TERMIOS
#include <sys/termios.h>
#endif

typedef struct Feed {
    struct Feed *fe_Next;
    char        *fe_Label;
    int         fe_Fd;
    char        *fe_Buf;
    int         fe_BufIdx;
    int         fe_BufMax;
    int         fe_Failed;
    int         fe_OpenFailed;	/* the queue file could not be opened: logged */
    int         fe_Lost;	/* lines that were not queued: exit 1 */
} Feed;

Feed    *FeBase;
MemPool *ParProcMemPool;
char    *MySpoolHome;

int fwCallBack(const char *hlabel, const char *msgid, const char *path, const char *offsize, int plfo, int headOnly, const char *artType, int lines, const char *cSize);
void writeFeedOut(const char *label, const char *file, const char *msgid, const char *offSize, int rt, int headOnly, int lines, const char *cSize);
int flushFeeds(int justClose);
void flushFeedOut(Feed *fe);
static void openFailed(Feed *fe);

void
Usage(void)
{
    fprintf(stderr, "Requeue articles for a feed or all feeds from DUMP lines (usually written\n");
    fprintf(stderr, "from diloadfromspool) on stdin.\n\n");
    fprintf(stderr, "Usage: drequeue [-a] [-C file] [-d[n]] [-f file] [-s] [-V] [label]\n");
    fprintf(stderr, "  where:\n");
    fprintf(stderr, "\t-a\t- requeue for all labels in dnewsfeeds\n");
    fprintf(stderr, "\t-C file\t- specify diablo.config to use\n");
    fprintf(stderr, "\t-d[n]\t- set debug [with optional level]\n");
    fprintf(stderr, "\t-f file\t- specify the dnewsfeeds filename\n");
    fprintf(stderr, "\t-s\t- print each line to stdout and to labels\n");
    fprintf(stderr, "\t-V\t- print version and exit\n");
    fprintf(stderr, "\tlabel\t- only requeue entries for specified dnewsfeeds label\n");
    exit(1);
}

int
main(int ac, char **av)
{
    char buf[8192];
    char *s1;
    char *hlabel = NULL;
    int i;
    int sendStdout = 0;
    int doAll = 0;

    LoadDiabloConfig(ac, av);

     /*
     * Extract the spool home pattern into a real path and place
     * in MySpoolHome
     */

    {
        int l = strlen(PatExpand(SpoolHomePat)) + 1;
        MySpoolHome = malloc(l);
        snprintf(MySpoolHome, l, "%s", PatExpand(SpoolHomePat));
    }

    for (i = 1; i < ac; ++i) {
	char *ptr = av[i];
	
	if (*ptr != '-') {
	    hlabel = ptr;
	} else {
	    ptr++;
	    switch(*ptr++) {
		case 'a':
		    doAll = 1;
		    break;
		case 'C':
		    if (*ptr == 0)
			++i;
		    break;
		case 'd':
		    DebugOpt = *ptr ? atoi(ptr) : 1;
		    break;
		case 'f':
		    DNewsfeedsPat = *ptr ? ptr : av[++i];
		    break;
		case 's':
		    sendStdout = 1;
		    break;
		case 'V':
		    PrintVersion();
		    break;
		default:
		    Usage();
	    }
	}
    }

    if (hlabel == NULL && doAll == 0)
	Usage();

    if (hlabel != NULL)
	printf("Requeueing for label: %s\n", hlabel);

    LoadNewsFeed(0, 1, hlabel);

    while (fgets(buf, sizeof(buf), stdin) != NULL) {
        s1 = strtok(buf, " \t\n");
        if (s1 == NULL)
            continue;

        if (strncmp(s1, "SOUT", 4) == 0) {
            char *path = strtok(NULL, "\t\n");
            char *offsize = strtok(NULL, "\t\n");
            const char *msgid = MsgId(strtok(NULL, "\t\n"), NULL);
            char *nglist = strtok(NULL, "\t\n");
            char *dist = strtok(NULL, "\t\n");
            char *npath = strtok(NULL, "\t\n");
            char *headOnly = strtok(NULL, "\t\n");
            const char *artType = strtok(NULL, "\t\n");
            const char *cSize = strtok(NULL, "\t\n");
            int lines = -1;

            /* L<lines> before cSize (D-174); older SOUT lines have none */
            if (cSize != NULL && cSize[0] == 'L') {
                lines = (int)strtol(cSize + 1, NULL, 10);
                cSize = strtok(NULL, "\t\n");
            }
            /* T<size>:<type>:<generation> of a binary in a text article (FeedAdd()): not used here */
            if (cSize != NULL && cSize[0] == 'T')
                cSize = strtok(NULL, "\t\n");
            if (path && offsize && msgid && nglist) {
                int spamArt = 0;
		if (npath == NULL) npath = "1";
		if (headOnly == NULL) headOnly = "0";
		if (artType == NULL) artType = "000000";
		if (cSize != NULL && strcmp(cSize, "0") == 0) cSize = NULL;
		if (sendStdout)
		    printf(
			"SOUTLINE: %s %s %s %s %s HO=%s AT=%s %s\n",
			path, 
			offsize,
			msgid,
			nglist,
			npath, 
			headOnly,
			artType,
			cSize != NULL ? cSize : ""
		    );

    		FeedWrite(0, fwCallBack, msgid, path, offsize, nglist, npath, dist, headOnly, artType, spamArt, lines, -1, -1, cSize);
	    }
	}
    }
    /*
     * Lines that could not be queued (the queue file could not be opened,
     * or a write failed) must show in the exit status.
     */
    if (flushFeeds(0) > 0)
	exit(1);
    exit(0);
}

int 
fwCallBack(const char *hlabel, const char *msgid, const char *path, const char *offsize, int plfo, int headOnly, const char *artType, int lines, const char *cSize)
{
    writeFeedOut(hlabel, path, msgid, offsize, ((plfo > 0) ? 1 : 0), headOnly, lines, cSize);
    return(0);
}

void
writeFeedOut(const char *label, const char *file, const char *msgid, const char *offSize, int rt, int headOnly, int lines, const char *cSize)
{
    Feed *fe;

    /*
     * locate feed
     */

    for (fe = FeBase; fe; fe = fe->fe_Next) {
	if (strcmp(label, fe->fe_Label) == 0)
	    break;
    }

    /*
     * allocate feed if not found.  The buffer is allocated also when the
     * queue file cannot be opened now: the lines wait in it and the next
     * flush tries to open the file again (flushFeedOut()).
     */

    if (fe == NULL) {
	int bsize;

	fe = zalloc(&ParProcMemPool, sizeof(Feed) + strlen(label) + 1);
	fe->fe_Label = (char *)(fe + 1);
	strcpy(fe->fe_Label, label);
	fe->fe_Fd = xopen(O_APPEND|O_RDWR|O_CREAT, 0644, "%s/%s", PatExpand(DQueueHomePat), label);
	if (fe->fe_Fd < 0)
	    openFailed(fe);
	fe->fe_Buf = pagealloc(&bsize, 1);
	fe->fe_BufMax = bsize;
	fe->fe_BufIdx = 0;
	fe->fe_Failed = 0;
	fe->fe_Next = FeBase;
	FeBase = fe;
    }

    /*
     * write to buffered feed, flushing buffers if there is not enough
     * room.  If the line is too long, something has gone wrong and we
     * throw it away.
     *
     * note that we cannot fill the buffer to 100%, because the trailing
     * nul (which we do not write) will overrun it.  I temporarily add 4 to
     * l instead of 3 to include the trailing nul in the calculations, but
     * subtract it off after the actual copy operation.
     */

    if (fe->fe_Buf != NULL) {
	char lineField[16];
	/*
	 * " H", " L" + lines, " C" + cSize, the spaces, newline and nul.
	 * cSize was not
	 * counted: it comes from the SOUT line on stdin, and a line whose
	 * fields are shifted can put a long value there (as in diablo's
	 * writeFeedOut()).
	 */
	int l = strlen(file) + strlen(msgid) + strlen(offSize) +
		(cSize != NULL ? strlen(cSize) : 0) + (3 + 32 + 12 + 1);

	/*
	 * line would be too long?
	 */
	if (lines >= 0)
	    snprintf(lineField, sizeof(lineField), " L%d", lines);
	else
	    lineField[0] = 0;
	if (l >= fe->fe_BufMax) {
	    /* longer than the whole buffer: not queued, and counted */
	    logit(LOG_ERR, "%s: queue line for %s is too long (%d bytes), dropped",
						fe->fe_Label, msgid, l);
	    ++fe->fe_Lost;
	} else {
	    /*
	     * line fits in buffer with trailing nul ?
	     */
	    if (l >= fe->fe_BufMax - fe->fe_BufIdx)
		flushFeedOut(fe);

	    if (l >= fe->fe_BufMax - fe->fe_BufIdx) {
		/*
		 * The queue file cannot be opened and the buffer keeps
		 * its lines for the next try: no room for this one.
		 */
		++fe->fe_Lost;
		return;
	    }
	    snprintf(fe->fe_Buf + fe->fe_BufIdx, fe->fe_BufMax - fe->fe_BufIdx,
		"%s %s %s%s%s%s%s\n",
		file, msgid, offSize, 
		(headOnly ? " H" : ""),
		lineField,
		(cSize != NULL ? " C" : ""),
		(cSize != NULL ? cSize : "")
	    );
	    fe->fe_BufIdx += strlen(fe->fe_Buf + fe->fe_BufIdx);
	    if (rt)
		flushFeedOut(fe);
	}
    }
}

static int
countLines(const char *buf, int len)
{
    int n = 0;

    while (len-- > 0)
	if (*buf++ == '\n')
	    ++n;
    return(n);
}

/*
 * Flush and close every feed.  Returns the number of lines that were not
 * queued: lines still in a buffer because the queue file could not be
 * opened, lines dropped for lack of room meanwhile, and lines of failed
 * writes.
 */
int
flushFeeds(int justClose)
{
    Feed *fe;
    int lost = 0;

    while ((fe = FeBase) != NULL) {
	if (justClose == 0)
	    flushFeedOut(fe);
	if (fe->fe_Buf && fe->fe_BufIdx)
	    fe->fe_Lost += countLines(fe->fe_Buf, fe->fe_BufIdx);
	if (fe->fe_Lost) {
	    logit(LOG_ERR, "%s: %d lines were not queued", fe->fe_Label, fe->fe_Lost);
	    fprintf(stderr, "drequeue: %s: %d lines were not queued\n",
						fe->fe_Label, fe->fe_Lost);
	    lost += fe->fe_Lost;
	}
	if (fe->fe_Buf)
	    pagefree(fe->fe_Buf, 1);
	if (fe->fe_Fd >= 0)
	    close(fe->fe_Fd);
	fe->fe_Fd = -1;
	fe->fe_Buf = NULL;
	FeBase = fe->fe_Next;
	zfree(&ParProcMemPool, fe, sizeof(Feed) + strlen(fe->fe_Label) + 1);
    }
    return(lost);
}

/*
 * The queue file could not be opened: say so once (also on stderr, for
 * the admin running drequeue), not on every flush that tries again.
 */
static void
openFailed(Feed *fe)
{
    if (fe->fe_OpenFailed == 0) {
	char why[128];

	snprintf(why, sizeof(why), "%s", strerror(errno));

	fe->fe_OpenFailed = 1;
	fprintf(stderr, "drequeue: %s: unable to open the queue file: %s, keeping the lines and trying again\n",
						fe->fe_Label, why);
	logit(LOG_ERR, "%s: unable to open the queue file: %s, keeping the lines and trying again",
						fe->fe_Label, why);
    }
}

/*
 * dspoolout renames dqueue/<label> to <label>.S<seq> and dnewslink
 * removes that file once it has sent it.  A run that spans a rotation
 * must not go on writing to the renamed file: lines written after
 * dnewslink read it are lost.  So before each write, drop the descriptor
 * when the name no longer points at the file we hold (as the feeder does
 * when it closes its queue files on a flush); flushFeedOut() then opens
 * the queue file again by name.
 */
static int
heldFileGone(Feed *fe)
{
    struct stat fst;
    struct stat pst;
    char path[PATH_MAX];

    if (fstat(fe->fe_Fd, &fst) < 0)
	return(0);
    if (snprintf(path, sizeof(path), "%s/%s", PatExpand(DQueueHomePat),
				fe->fe_Label) >= (int)sizeof(path))
	return(0);
    if (stat(path, &pst) == 0)
	return(pst.st_dev != fst.st_dev || pst.st_ino != fst.st_ino);
    return(errno == ENOENT);
}

void
flushFeedOut(Feed *fe)
{
    if (fe->fe_BufIdx == 0 || fe->fe_Buf == NULL)
	return;
    if (fe->fe_Fd >= 0 && heldFileGone(fe)) {
	close(fe->fe_Fd);
	fe->fe_Fd = -1;
    }
    if (fe->fe_Fd < 0) {
	/*
	 * Not open (it could not be opened, or it was renamed away):
	 * open it by name.  If that fails, the lines stay in the buffer
	 * for the next flush, never go to a file the name has left.
	 */
	fe->fe_Fd = xopen(O_APPEND|O_RDWR|O_CREAT, 0644, "%s/%s",
				PatExpand(DQueueHomePat), fe->fe_Label);
	if (fe->fe_Fd < 0) {
	    openFailed(fe);
	    return;
	}
	if (fe->fe_OpenFailed) {
	    fe->fe_OpenFailed = 0;
	    logit(LOG_NOTICE, "%s: queue file open again", fe->fe_Label);
	}
    }
    {
	/*
	 * flush buffer.  If the write fails, we undo it to ensure
	 * that we do not get garbaged feed files.
	 */
	int n = write(fe->fe_Fd, fe->fe_Buf, fe->fe_BufIdx);
	if (n >= 0 && n != fe->fe_BufIdx &&
	    ftruncate(fe->fe_Fd, lseek(fe->fe_Fd, 0L, 1) - n) < 0) {
	    logit(LOG_ERR, "%s: queue file truncate failed: %s",
					fe->fe_Label, strerror(errno));
	}
	if (n != fe->fe_BufIdx) {
	    if (fe->fe_Failed == 0) {
		fe->fe_Failed = 1;
		logit(LOG_INFO, "failure writing to feed %s", fe->fe_Label);
	    }
	    fe->fe_Lost += countLines(fe->fe_Buf, fe->fe_BufIdx);
	}
    }
    fe->fe_BufIdx = 0;
}
