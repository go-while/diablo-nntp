
/*
 * LOG.C
 *
 */

#include "defs.h"

Prototype void OpenLog(const char *ident, int option);
Prototype void ChownGeneralLog(uid_t uid, gid_t gid);
Prototype void CloseLog(LogInfo *LI, int killit);
Prototype void Log(LogInfo *LI, int priority, const char *ctl, ...);
Prototype void VLog(LogInfo *LI, int priority, const char *ctl, va_list va);
Prototype void logit(int priority, const char *ctl, ...);
Prototype void vlogit(int priority, const char *ctl, va_list va);
Prototype void LogPost(int priority, const char *ctl, ...);
Prototype void LogIncoming(const char *format, char *label, const char *msgid, char *err);
Prototype void CloseIncomingLog(void);
Prototype void WritePath(char *path);
Prototype void ClosePathLog(int killit);
Prototype void WriteArtLog(char *path, int size, char *arttype, char *nglist);
Prototype void CloseArtLog(int killit);

static LogInfo GeneralLog = { &GeneralLogPat, NULL, NULL, 0, 0, 0, 0, 0, {0} };
static LogInfo IncomingLog = { &IncomingLogPat, NULL, NULL, 0, 0, 0, 0, 0, {0} };
static LogInfo PathLog = { &FPathLogPat, NULL, NULL, 0, 0, 0, 0, 0, {0} };
static LogInfo ArtLog = { &FArtLogPat, NULL, NULL, 0, 0, 0, 0, 0, {0} };
static LogInfo PostLog = { &PostLogPat, NULL, NULL, 0, 0, 0, 0, 0, {0} };

#if USE_INTERNAL_VSYSLOG
void
vsyslog(int priority, const char *ctl, va_list va) 
{
    char buf[1024];

    vsnprintf(buf, sizeof(buf), ctl, va);
    syslog(priority, "%s", buf);
}
#endif

/*
 * openProg() - start the program of a "|prog" log and write to it through
 *		a pipe.
 *
 *	When that fails the log falls back to syslog until openLog() tries
 *	again: with LI->Fd left NULL and nothing else set, VLog() wrote to a
 *	NULL FILE, and for the general log the report of the failure came
 *	back here through logit() without end.  So the failure is reported
 *	with syslog() itself, as for a log file that cannot be opened.
 *	A program that cannot be started (dup2() or execl() fails in the
 *	child) counts as a failure too: the child reports it through a
 *	second pipe, and the parent reaps it and falls back.
 */
void
openProg(LogInfo *LI, const char *prog)
{
    int fds[2];
    int st[2];		/* the child's report: errno of a failed start */
    int cerr = 0;
    ssize_t n;

    if (pipe(fds) < 0) {
	syslog(LOG_ERR, "log unable to pipe(): %s", strerror(errno));
	LI->UseSyslog = 1;
	return;
    }
    /*
     * The child writes the errno of a failed dup2() or execl() into st[1].
     * st[1] is closed at a successful exec, so the parent reads end of file
     * then.  Without this the parent took a child that could not start for
     * a running program and wrote the log into a pipe nobody reads.
     */
    if (pipe(st) < 0) {
	syslog(LOG_ERR, "log unable to pipe(): %s", strerror(errno));
	close(fds[0]);
	close(fds[1]);
	LI->UseSyslog = 1;
	return;
    }
    (void)fcntl(st[1], F_SETFD, FD_CLOEXEC);
    if ((LI->Pid = fork()) < 0) {
	syslog(LOG_ERR, "log unable to fork(): %s", strerror(errno));
	close(fds[0]);
	close(fds[1]);
	close(st[0]);
	close(st[1]);
	LI->UseSyslog = 1;
	return;
    }
    if (!LI->Pid) {
	/*
	 * The child must never return into the daemon's code.
	 */
	close(st[0]);
	if (dup2(fds[0], fileno(stdin)) < 0) {
	    cerr = errno;
	    syslog(LOG_ERR, "log unable to dup2(): %s", strerror(cerr));
	    if (write(st[1], &cerr, sizeof(cerr)) < 0)
		;	/* nothing more a child that cannot start can do */
	    _exit(1);
	}
	close(fds[0]);
	close(fds[1]);
	execl(prog, prog, NULL);
	cerr = errno;
	syslog(LOG_ERR, "log unable to execl(%s): %s", prog, strerror(cerr));
	if (write(st[1], &cerr, sizeof(cerr)) < 0)
	    ;	/* as above */
	close(fileno(stdin));
	_exit(1);
    }
    /*
     * Only parent should reach here
     */
    close(fds[0]);
    close(st[1]);
    do {
	n = read(st[0], &cerr, sizeof(cerr));
    } while (n < 0 && errno == EINTR);
    close(st[0]);
    if (n > 0) {
	/* the program did not start: the child is gone, reap it here */
	close(fds[1]);
	while (waitpid(LI->Pid, NULL, 0) < 0 && errno == EINTR)
	    ;
	LI->Pid = -1;
	LI->UseSyslog = 1;
	return;
    }
    if (fcntl(fds[1], F_SETFD, 1) < 0)
	syslog(LOG_ERR, "log unable to fcntl(): %s", strerror(errno));
    if ((LI->Fd = fdopen(fds[1], "a")) == NULL) {
	syslog(LOG_ERR, "log unable to fdopen(): %s", strerror(errno));
	/*
	 * The program runs but gets no lines: stop it and reap it, so that
	 * the next try does not leave it behind under a forgotten pid.
	 */
	close(fds[1]);
	kill(LI->Pid, SIGTERM);
	for (n = 0; n < 50; ++n) {
	    pid_t w = waitpid(LI->Pid, NULL, WNOHANG);

	    if (w != 0 && !(w < 0 && errno == EINTR))
		break;
	    usleep(10000);
	}
	if (n == 50) {
	    /* it does not stop on SIGTERM within half a second */
	    kill(LI->Pid, SIGKILL);
	    while (waitpid(LI->Pid, NULL, 0) < 0 && errno == EINTR)
		;
	}
	LI->Pid = -1;
	LI->UseSyslog = 1;
	return;
    }
    LI->UseSyslog = 0;
}

void
openLog(LogInfo *LI)
{
    struct stat st;
    const char *Fname;

    if ((!LI->Fd && !LI->UseSyslog && !LI->Disabled) ||
    		(time(NULL) > LI->NextCheck)) {
	LI->NextCheck = time(NULL) + 10;
	if (strcmp(*LI->Pat, "SYSLOG") == 0) {
	    /*
	     * I'm sure this seemed like a good idea at the time, but
	     * this is really and truly broken.  dreaderd ends up calling
	     * closelog() all the time because of the way this was hacked
	     * in to things.  Combined with some changes made to FreeBSD
	     * in the late rev. 4's, some weird stuff starts happening
	     * and so I've commented this out.  JG20031020
	     */
	    /*CloseLog(LI, 1);*/
	    LI->UseSyslog = 1;
	} else if (strcmp(*LI->Pat, "NONE") == 0) {
	    CloseLog(LI, 1);
	    LI->Disabled = 1;
	} else if (**LI->Pat == '|') {
	    Fname = PatLogExpand(*LI->Pat);
	    if (LI->Fd == NULL || strcmp(Fname, LI->Fname) != 0) {
		CloseLog(LI, 1);
		strncpy(LI->Fname, Fname, sizeof(LI->Fname) - 1);
		LI->Fname[sizeof(LI->Fname) - 1] = '\0';
		openProg(LI, Fname + 1);
	    }
	} else {
	    Fname = PatLogExpand(*LI->Pat);
	    if (stat(Fname, &st) != 0)
		LI->LastInode = 0;
	    if (!LI->Fd || !LI->LastInode || (LI->LastInode != st.st_ino)) {
		/* Need to reopen the log file */
		CloseLog(LI, 1);
		LI->Fd = PrivateFOpen(Fname, "a", 0640);
		if (LI->Fd == NULL) {
		    fprintf(stderr, "Unable to open log file: %s\n", Fname);
		    /* Do not log this failure through the log we cannot open. */
		    syslog(LOG_ERR, "Unable to open log file: %s: %s",
			Fname, strerror(errno));
	            LI->UseSyslog = 1;
		} else {
		    if (stat(Fname, &st) == 0)
			LI->LastInode = st.st_ino;
		    strncpy(LI->Fname, Fname, sizeof(LI->Fname) - 1);
		    LI->Fname[sizeof(LI->Fname) - 1] = '\0';
		}
	    }
	}
    }

}

/*
 * The feeder opens its general log as root during startup, then drops
 * privileges to news.  Make a root-created regular log writable by the
 * runtime user before the privilege change.  Use the open descriptor to
 * avoid races with log rotation or pathname substitution.  Do not change
 * ownership of an existing log belonging to a different account.
 */
void
ChownGeneralLog(uid_t uid, gid_t gid)
{
    struct stat st;

    if (geteuid() != 0 || GeneralLog.Fd == NULL || GeneralLog.UseSyslog)
        return;
    if (fstat(fileno(GeneralLog.Fd), &st) != 0) {
        syslog(LOG_ERR, "Unable to stat general log: %s", strerror(errno));
        return;
    }
    if (!S_ISREG(st.st_mode) || st.st_uid != 0)
        return;
    if (fchown(fileno(GeneralLog.Fd), uid, gid) != 0)
        syslog(LOG_ERR, "Unable to chown general log to %lu:%lu: %s",
            (unsigned long)uid, (unsigned long)gid, strerror(errno));
}

void
OpenLog(const char *ident, int option)
{
    GeneralLog.Ident = ident;
    openlog(ident, option, LOG_NEWS);
}

void
Log(LogInfo *LI, int priority, const char *ctl, ...)
{
    va_list va;

    va_start(va, ctl);
    VLog(LI, priority, ctl, va);
    va_end(va);
}

void
VLog(LogInfo *LI, int priority, const char *ctl, va_list va)
{
    /* check & (re)open the log if neccessary */
    openLog(LI);

    /* print message itself */
    if (LI->UseSyslog) {
	vsyslog(priority, ctl, va);
    } else if (LI->Disabled || (priority == LOG_DEBUG && !DebugOpt)) {
	/* Do nothing. */
    } else {
	static char logLine[8192];
	char *p = logLine;

	/* Add date and time. Syslog has its own time stamps. */
	p += sprintf(logLine, "%s %s%s ", LogTime(),
				LI->Ident ? LI->Ident : "",
				LI->Ident ? ":" : "");
	vsnprintf(p, sizeof(logLine) - (p - logLine) - 1, ctl, va);
	/*
	 * One line per message: a message that ends in a newline of its
	 * own (some formats have one, and so may a Perl auth result) would
	 * otherwise leave a blank line in the log, and a CR or LF inside
	 * one (a logged value such as script output) would split it.
	 */
	{
	    size_t n = strlen(logLine);
	    char *q;

	    while (n > 0 && (logLine[n - 1] == '\n' || logLine[n - 1] == '\r'))
		logLine[--n] = 0;
	    for (q = p; *q; ++q) {
		if (*q == '\n' || *q == '\r')
		    *q = ' ';
	    }
	}
	fprintf(LI->Fd, "%s\n", logLine);
	fflush(LI->Fd);
    }
}

void
CloseLog(LogInfo *LI, int killit)
{
    if (LI == NULL)
	LI = &GeneralLog;
    if (LI->UseSyslog)
	closelog();
    if (LI->Fd != NULL) {
	if (LI->Pid > 0 && killit) {
	    kill(LI->Pid, SIGINT);
	    LI->Pid = -1;
	}
	fclose(LI->Fd);
	LI->Fd = NULL;
	LI->LastInode = 0;
    }
}

void
logit(int priority, const char *ctl, ...)
{
    va_list va;

    va_start(va, ctl);
    vlogit(priority, ctl, va);
    va_end(va);
}

void
vlogit(int priority, const char *ctl, va_list va)
{
    VLog(&GeneralLog, priority, ctl, va);
}

/*
 * LogPost() - a line about a post (dreaderd: accepted or refused) into the
 *	       post log, path_postlog in diablo.config.  Without one the line
 *	       goes into the general log, as it always did; "NONE" drops it,
 *	       "SYSLOG" sends it to syslog.
 */
void
LogPost(int priority, const char *ctl, ...)
{
    va_list va;

    va_start(va, ctl);
    if (PostLogPat == NULL || *PostLogPat == 0) {
	VLog(&GeneralLog, priority, ctl, va);
    } else {
	PostLog.Ident = GeneralLog.Ident;
	VLog(&PostLog, priority, ctl, va);
    }
    va_end(va);
}

void
LogIncoming(const char *format, char *label, const char *msgid, char *err)
{
    Log(&IncomingLog, LOG_INFO, format, label, msgid, err);
}

void
CloseIncomingLog(void)
{
    CloseLog(&IncomingLog, 1);
}

void
WritePath(char *path)
{
    openLog(&PathLog);

    /* print message itself */
    if (PathLog.Fd != NULL) {
	if (fprintf(PathLog.Fd, "Path: %s\n", path) == 0)
	    ClosePathLog(1);
	else
	    fflush(PathLog.Fd);
    }
}

void
ClosePathLog(int killit)
{
    CloseLog(&PathLog, killit);
}

void
WriteArtLog(char *path, int size, char *arttype, char *nglist)
{
    char host[255];
    char *p;

    openLog(&ArtLog);

    if ((p = strchr(path, '!')) != NULL && p - path < sizeof(host)) {
	int n = p - path;
	if (n >= sizeof(host))
	    n = sizeof(host) - 1;
	strncpy(host, path, n);
	host[n] = '\0';
    } else {
	strncpy(host, path, sizeof(host) - 1);
	host[sizeof(host) - 1] = '\0';
    }
    /* print message itself */
    if (ArtLog.Fd != NULL) {
	if (fprintf(ArtLog.Fd, "%s %d %s %s\n", host, size, arttype, nglist) == 0)
	    CloseArtLog(1);
	else
	    fflush(ArtLog.Fd);
    }
}

void
CloseArtLog(int killit)
{
    CloseLog(&ArtLog, killit);
}

