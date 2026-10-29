/*
 * UTIL/DCANLOCK.C	- make and check Cancel-Keys and Cancel-Locks (RFC 8315)
 *
 *	dcanlock key    message-id ...	the Cancel-Key for withdrawing articles
 *	dcanlock lock   message-id ...	the Cancel-Lock such an article carries
 *	dcanlock cancel message-id	a whole cancel article, with the key
 *	dcanlock verify keys locks	whether a key matches a lock
 *
 *	The secrets are the canlockadmin and canlockuser lines of a
 *	vserverdef in dreader.access, the ones dreaderd makes the locks of
 *	local posts from.  Making a key is how an admin cancels an article
 *	that someone else posted: put the key into the Cancel-Key header of a
 *	cancel (or a Supersedes) message; 'cancel' writes that message, like
 *	INN's gencancel.  The computations are libcanlock's.
 */

#include "defs.h"

static void
Usage(const char *progname)
{
    fprintf(stderr, "Make and check Cancel-Keys and Cancel-Locks (RFC 8315)\n");
    fprintf(stderr, "Usage: %s [-C diablo.config] [-A dreader.access] [-v vserver]\n"
		    "\t[-u user] [-t scheme] [-S] key|lock message-id ...\n", progname);
    fprintf(stderr, "       %s [options] [-n newsgroups] [-f from] [-s subject] [-b body]\n"
		    "\t[-c charset] [-d] [-m] [-a] [-L] cancel message-id\n", progname);
    fprintf(stderr, "       %s verify cancel-keys cancel-locks\n", progname);
    fprintf(stderr, "\tkey\t\tprint the Cancel-Key header value for each Message-ID\n"
		    "\tlock\t\tprint the Cancel-Lock value for each Message-ID\n"
		    "\tcancel\t\twrite a cancel article for the Message-ID to standard output,\n"
		    "\t\t\twith the Cancel-Key, ready to be posted\n"
		    "\tverify\t\texit 0 when a key of the first value matches a lock of\n"
		    "\t\t\tthe second (no secret needed), 1 when none does\n"
		    "\t-A file\t\tthe dreader.access to read (default: the one of diablo.config)\n"
		    "\t-v vserver\tthe vserverdef the secrets are in (default DEFAULT)\n"
		    "\t-u user\t\tuse the canlockuser secrets and this user (default: the\n"
		    "\t\t\tcanlockadmin secrets)\n"
		    "\t-t scheme\tonly sha256, sha1 or sha512 (default: sha256, and sha1\n"
		    "\t\t\tunless the vserverdef says canlocksha1 no)\n"
		    "\t-S\t\tread the secret from the first line of standard input\n"
		    "\t\t\tinstead of dreader.access\n"
		    "\tcancel only:\n"
		    "\t-n newsgroups\tthe Newsgroups of the cancel (default: those of the article,\n"
		    "\t\t\tread with dreadart)\n"
		    "\t-f from\t\tthe From (default: newsadmin of diablo.config)\n"
		    "\t-s subject\tthe Subject (default: cmsg cancel <message-id>)\n"
		    "\t-b body\t\tthe text of the body (default: Admin cancel.)\n"
		    "\t-c charset\tthe charset of the body (default: ISO-8859-1)\n"
		    "\t-d\t\tthe Date in UTC, not in local time\n"
		    "\t-m\t\tno Message-ID (the server makes one)\n"
		    "\t-a\t\tno Approved header (it repeats the From)\n"
		    "\t-L\t\tno Cancel-Key header\n");
    exit(2);
}

typedef struct Secrets {
    char admin[CANLOCK_MAXSECRETS][CANLOCK_SECRETLEN];
    char user[CANLOCK_MAXSECRETS][CANLOCK_SECRETLEN];
    int noSha1;
} Secrets;

static int ConfigLoaded;

/*
 * loadConfig() - read diablo.config once (for the path of dreader.access,
 * newsadmin and the host name); the -C option is found in av.
 */
static void
loadConfig(int ac, char **av)
{
    if (!ConfigLoaded) {
	LoadDiabloConfig(ac, av);
	ConfigLoaded = 1;
    }
}

/*
 * addSecret() - store a canlockadmin or canlockuser value like dreaderd
 * does: trailing white space is not part of it, one that doesn't fit is
 * ignored.
 */
static void
addSecret(char secrets[][CANLOCK_SECRETLEN], const char *opt, const char *val)
{
    int len = strlen(val);
    int i;

    while (len > 0 && isspace((unsigned char)val[len - 1]))
	--len;
    if (len == 0)
	return;
    if (len >= CANLOCK_SECRETLEN) {
	fprintf(stderr, "dcanlock: a %s secret is longer than %d characters, ignored\n",
		opt, CANLOCK_SECRETLEN - 1);
	return;
    }
    for (i = 0; i < CANLOCK_MAXSECRETS; ++i) {
	if (secrets[i][0] == 0) {
	    memcpy(secrets[i], val, len);
	    secrets[i][len] = 0;
	    return;
	}
    }
    fprintf(stderr, "dcanlock: more than %d %s lines, the others are ignored\n",
	    CANLOCK_MAXSECRETS, opt);
}

/*
 * readAccess() - collect the canlockadmin, canlockuser and canlocksha1 lines
 * of the first vserverdef called vsName in the dreader.access file at path,
 * read the way dreaderd reads it.  Returns 1 when the vserverdef was found.
 */
static int
readAccess(const char *path, const char *vsName, Secrets *sec)
{
    FILE *fp;
    char buf[1024];
    int inVs = 0;
    int found = 0;

    if ((fp = fopen(path, "r")) == NULL) {
	fprintf(stderr, "dcanlock: cannot open %s: %s\n", path, strerror(errno));
	exit(2);
    }
    while (fgets(buf, sizeof(buf), fp) != NULL) {
	char *opt = buf;
	char *val;

	if (strchr(buf, '\n') == NULL && !feof(fp)) {
	    int c;

	    while ((c = getc(fp)) != EOF && c != '\n')	/* skip the rest of a long line */
		;
	    continue;
	}
	while (isspace((unsigned char)*opt))
	    ++opt;
	opt = strtok(opt, " \t\r\n");
	if (opt == NULL || *opt == '#')
	    continue;
	val = strtok(NULL, "\n");
	if (val == NULL || *val == '#')
	    val = "";
	else
	    while (isspace((unsigned char)*val))
		++val;

	if (strcmp(opt, "vserverdef") == 0) {
	    int len = strlen(val);

	    while (len > 0 && isspace((unsigned char)val[len - 1]))
		--len;
	    if (found)
		break;
	    inVs = (len == (int)strlen(vsName) && strncmp(val, vsName, len) == 0);
	    found = inVs;
	} else if (strcmp(opt, "end") == 0 || strcmp(opt, "groupdef") == 0 ||
		   strcmp(opt, "authdef") == 0 || strcmp(opt, "readerdef") == 0 ||
		   strcmp(opt, "access") == 0) {
	    if (found)
		break;
	} else if (inVs && strcmp(opt, "canlockadmin") == 0) {
	    addSecret(sec->admin, opt, val);
	} else if (inVs && strcmp(opt, "canlockuser") == 0) {
	    addSecret(sec->user, opt, val);
	} else if (inVs && strcmp(opt, "canlocksha1") == 0) {
	    sec->noSha1 = (*val == 'n' || *val == 'N' || *val == '0');
	}
    }
    fclose(fp);
    return(found);
}

/*
 * validMsgId() - a Message-ID with its angle brackets, no white space.
 */
static int
validMsgId(const char *mid)
{
    int len = strlen(mid);
    int i;

    if (len < 3 || len > MAXMSGIDLEN || mid[0] != '<' || mid[len - 1] != '>')
	return(0);
    for (i = 1; i < len - 1; ++i) {
	if (mid[i] == '<' || mid[i] == '>' || isspace((unsigned char)mid[i]) || iscntrl((unsigned char)mid[i]))
	    return(0);
    }
    return(1);
}

/*
 * put() - append the element "scheme:value" to the list in out.
 */
static void
put(char *out, int outLen, const char *element)
{
    int n = strlen(out);
    int el = strlen(element);

    if (n + (n ? 1 : 0) + el + 1 > outLen) {
	fprintf(stderr, "dcanlock: too many elements\n");
	exit(2);
    }
    if (n)
	out[n++] = ' ';
    memcpy(out + n, element, el + 1);
}

/*
 * What the key and lock commands work with: the secrets chosen by -u and
 * the schemes chosen by -t and canlocksha1.
 */
typedef struct Setup {
    char (*secrets)[CANLOCK_SECRETLEN];
    const char *uid;
    const char *schemes[2];
    int nSchemes;
} Setup;

#define LISTLEN	(CANLOCK_MAXSECRETS * 3 * 160)

/*
 * makeList() - the elements of kind ("key" or "lock") for a Message-ID, one
 * for each secret and scheme, space separated, in out (LISTLEN bytes).
 */
static void
makeList(const Setup *st, const char *kind, const char *mid, char *out)
{
    int j;
    int s;

    out[0] = 0;
    for (j = 0; j < CANLOCK_MAXSECRETS && st->secrets[j][0]; ++j) {
	for (s = 0; s < st->nSchemes; ++s) {
	    char element[192];
	    int len;

	    if (strcmp(kind, "key") == 0)
		len = CanLockKey(st->schemes[s], st->secrets[j], st->uid, mid, element, sizeof(element));
	    else
		len = CanLockLock(st->schemes[s], st->secrets[j], st->uid, mid, element, sizeof(element));
	    if (len < 0) {
		fprintf(stderr, "dcanlock: cannot make the %s %s for %s\n", st->schemes[s], kind, mid);
		exit(2);
	    }
	    put(out, LISTLEN, element);
	}
    }
}

/*
 * plainText() - 1 when s is non-empty printable ASCII (a header value).
 */
static int
plainText(const char *s)
{
    if (*s == 0)
	return(0);
    for (; *s; ++s) {
	if (*s < ' ' || *s > '~')
	    return(0);
    }
    return(1);
}

/*
 * groupsOk() - 1 when s is a comma separated list of newsgroup names.
 */
static int
groupsOk(const char *s)
{
    if (*s == 0 || *s == ',')
	return(0);
    for (; *s; ++s) {
	if (!isalnum((unsigned char)*s) && strchr("+-._,", *s) == NULL)
	    return(0);
    }
    return(1);
}

/*
 * sameDirProgram() - dcanlock was started by name, so the shell found it in
 * the first PATH directory that has it: put the path of program in that
 * directory into path.  Returns 0 when there is none.  (A dreadart from
 * another PATH directory is not the one that belongs to this dcanlock.)
 */
static int
sameDirProgram(const char *self, const char *program, char *path, int pathLen)
{
    const char *p = getenv("PATH");

    if (p == NULL)
	return(0);
    for (;;) {
	int n = strcspn(p, ":");
	char cand[PATH_MAX];
	struct stat st;

	snprintf(cand, sizeof(cand), "%.*s%s%s", n, p, n ? "/" : "", self);
	if (n == 0)
	    snprintf(cand, sizeof(cand), "./%s", self);
	if (stat(cand, &st) == 0 && S_ISREG(st.st_mode) && access(cand, X_OK) == 0) {
	    if (n == 0)
		snprintf(path, pathLen, "./%s", program);
	    else
		snprintf(path, pathLen, "%.*s/%s", n, p, program);
	    return(1);
	}
	if (p[n] == 0)
	    break;
	p += n + 1;			/* an empty last component is the current directory */
    }
    return(0);
}

/*
 * articleGroups() - the Newsgroups header of the article mid in the spool,
 * read with dreadart (which sits next to this program), into buf.  Returns
 * 1 on success.  No shell: mid is passed as an argument.
 */
static int
articleGroups(const char *progname, const char *mid, char *buf, int bufLen)
{
    char path[PATH_MAX];
    char data[65536];
    const char *slash = strrchr(progname, '/');
    int fds[2];
    int n = 0;
    int r;
    int status;
    pid_t pid;
    char *p;

    if (slash) {
	snprintf(path, sizeof(path), "%.*s/dreadart", (int)(slash - progname), progname);
    } else if (!sameDirProgram(progname, "dreadart", path, sizeof(path))) {
	return(0);
    }
    if (pipe(fds) < 0)
	return(0);
    if ((pid = fork()) < 0) {
	close(fds[0]);
	close(fds[1]);
	return(0);
    }
    if (pid == 0) {
	int nul = open("/dev/null", O_WRONLY);

	dup2(fds[1], 1);
	if (nul >= 0)
	    dup2(nul, 2);
	close(fds[0]);
	close(fds[1]);
	execl(path, "dreadart", "-h", mid, (char *)NULL);
	_exit(127);
    }
    close(fds[1]);
    while (n < (int)sizeof(data) - 1 && (r = read(fds[0], data + n, sizeof(data) - 1 - n)) > 0)
	n += r;
    close(fds[0]);
    data[n] = 0;
    if (waitpid(pid, &status, 0) < 0 || !WIFEXITED(status) || WEXITSTATUS(status) != 0)
	return(0);

    for (p = data; *p; ) {
	char *e = strchr(p, '\n');
	int len = e ? e - p : (int)strlen(p);

	if (len > 0 && p[len - 1] == '\r')
	    --len;
	if (len == 0)
	    break;				/* end of the headers */
	if (len > 11 && strncasecmp(p, "Newsgroups:", 11) == 0) {
	    int i = 11;
	    int o = 0;

	    for (;;) {			/* the value, with its folded lines */
		for (; i < len; ++i) {
		    if (isspace((unsigned char)p[i]))
			continue;
		    if (o + 1 >= bufLen)
			return(0);	/* a cut list would name other groups */
		    buf[o++] = p[i];
		}
		if (e && (e[1] == ' ' || e[1] == '\t')) {
		    p = e + 1;
		    e = strchr(p, '\n');
		    len = e ? e - p : (int)strlen(p);
		    if (len > 0 && p[len - 1] == '\r')
			--len;
		    i = 0;
		    continue;
		}
		break;
	    }
	    buf[o] = 0;
	    return(o > 0);
	}
	if (e == NULL)
	    break;
	p = e + 1;
    }
    return(0);
}

/*
 * messageId() - a new Message-ID for the cancel article, unique enough:
 * time, process id and some random bytes.
 */
static void
messageId(char *buf, int bufLen, const char *host)
{
    unsigned char rnd[6];
    unsigned long salt = 0;
    int fd = open("/dev/urandom", O_RDONLY);
    int i;

    memset(rnd, 0, sizeof(rnd));
    if (fd >= 0) {
	if (read(fd, rnd, sizeof(rnd)) < 0)
	    memset(rnd, 0, sizeof(rnd));
	close(fd);
    }
    for (i = 0; i < (int)sizeof(rnd); ++i)
	salt = (salt << 8) | rnd[i];
    snprintf(buf, bufLen, "<cancel.%lu.%lu.%lx@%.100s>", (unsigned long)time(NULL),
		(unsigned long)getpid(), salt & 0xffffffffUL, host);
}

int
main(int ac, char **av)
{
    const char *accessFile = NULL;
    const char *vsName = "DEFAULT";
    const char *user = NULL;
    const char *only = NULL;
    const char *newsgroups = NULL;
    const char *from = NULL;
    const char *subject = NULL;
    const char *body = "Admin cancel.";
    const char *charset = "ISO-8859-1";
    int utc = 0;
    int noMid = 0;
    int noApproved = 0;
    int noKey = 0;
    const char *cmd;
    Secrets sec;
    Setup st;
    static const char *defSchemes[] = { "sha256", "sha1" };
    int stdinSecret = 0;
    int c;
    int i;

    while ((c = getopt(ac, av, "C:A:v:u:t:Sn:f:s:b:c:dmaL")) != -1) {
	switch (c) {
	case 'C':
	    break;			/* LoadDiabloConfig() reads it from av */
	case 'A':
	    accessFile = optarg;
	    break;
	case 'v':
	    vsName = optarg;
	    break;
	case 'u':
	    user = optarg;
	    break;
	case 't':
	    only = optarg;
	    break;
	case 'S':
	    stdinSecret = 1;
	    break;
	case 'n':
	    newsgroups = optarg;
	    break;
	case 'f':
	    from = optarg;
	    break;
	case 's':
	    subject = optarg;
	    break;
	case 'b':
	    body = optarg;
	    break;
	case 'c':
	    charset = optarg;
	    break;
	case 'd':
	    utc = 1;
	    break;
	case 'm':
	    noMid = 1;
	    break;
	case 'a':
	    noApproved = 1;
	    break;
	case 'L':
	    noKey = 1;
	    break;
	default:
	    Usage(av[0]);
	}
    }
    if (optind >= ac)
	Usage(av[0]);
    cmd = av[optind++];

    if (strcmp(cmd, "verify") == 0) {
	if (ac - optind != 2)
	    Usage(av[0]);
	if (CanLockVerify(av[optind], av[optind + 1])) {
	    printf("match\n");
	    return(0);
	}
	printf("no match\n");
	return(1);
    }
    if (strcmp(cmd, "key") != 0 && strcmp(cmd, "lock") != 0 && strcmp(cmd, "cancel") != 0)
	Usage(av[0]);
    if (optind >= ac || (strcmp(cmd, "cancel") == 0 && ac - optind != 1))
	Usage(av[0]);
    if (only && strcmp(only, "sha256") != 0 && strcmp(only, "sha1") != 0 && strcmp(only, "sha512") != 0) {
	fprintf(stderr, "dcanlock: the scheme must be sha256, sha1 or sha512\n");
	return(2);
    }
    if (user && (*user == 0 || strchr(user, '<') || strchr(user, '>'))) {
	fprintf(stderr, "dcanlock: the user must not be empty or contain angle brackets\n");
	return(2);
    }

    memset(&sec, 0, sizeof(sec));
    memset(&st, 0, sizeof(st));
    if (strcmp(cmd, "cancel") == 0 && noKey) {
	/* no secret is needed */
    } else if (stdinSecret) {
	char line[CANLOCK_SECRETLEN + 8];

	if (fgets(line, sizeof(line), stdin) == NULL) {
	    fprintf(stderr, "dcanlock: no secret on standard input\n");
	    return(2);
	}
	addSecret(user ? sec.user : sec.admin, user ? "canlockuser" : "canlockadmin", line);
	/*
	 * Only the secret comes from standard input.  With -A the vserverdef
	 * of that file still says whether sha1 locks are made (canlocksha1);
	 * without it nothing is read, as the machine may have no diablo.config.
	 */
	if (accessFile != NULL) {
	    Secrets fromFile;

	    memset(&fromFile, 0, sizeof(fromFile));
	    if (!readAccess(accessFile, vsName, &fromFile)) {
		fprintf(stderr, "dcanlock: no vserverdef %s in %s\n", vsName, accessFile);
		return(2);
	    }
	    sec.noSha1 = fromFile.noSha1;
	}
    } else {
	if (accessFile == NULL) {
	    loadConfig(ac, av);		/* for the path of dreader.access */
	    accessFile = PatLibExpand(DReaderAccessPat);
	}
	if (!readAccess(accessFile, vsName, &sec)) {
	    fprintf(stderr, "dcanlock: no vserverdef %s in %s\n", vsName, accessFile);
	    return(2);
	}
    }
    st.secrets = user ? sec.user : sec.admin;
    st.uid = user ? user : "";
    st.schemes[0] = defSchemes[0];
    st.schemes[1] = defSchemes[1];
    st.nSchemes = sec.noSha1 ? 1 : 2;
    if (only) {
	st.schemes[0] = only;
	st.nSchemes = 1;
    }
    if (!(strcmp(cmd, "cancel") == 0 && noKey) && st.secrets[0][0] == 0) {
	fprintf(stderr, "dcanlock: no %s secret in vserverdef %s\n",
		user ? "canlockuser" : "canlockadmin", vsName);
	return(2);
    }

    if (strcmp(cmd, "cancel") != 0) {
	for (i = optind; i < ac; ++i) {
	    char out[LISTLEN];

	    if (!validMsgId(av[i])) {
		fprintf(stderr, "dcanlock: not a Message-ID (with its <>): %s\n", av[i]);
		return(2);
	    }
	    makeList(&st, cmd, av[i], out);
	    printf("%s\n", out);
	}
	return(0);
    }

    /*
     * cancel: a whole cancel article for av[optind], like INN's gencancel.
     */
    {
	const char *mid = av[optind];
	char groups[1024];
	char out[LISTLEN];
	char host[256];
	char newMid[MAXMSGIDLEN + 1];
	char date[64];
	char subj[MAXMSGIDLEN + 32];
	time_t now = time(NULL);
	struct tm *tm;

	if (!validMsgId(mid)) {
	    fprintf(stderr, "dcanlock: not a Message-ID (with its <>): %s\n", mid);
	    return(2);
	}
	if (from == NULL) {
	    loadConfig(ac, av);		/* newsadmin */
	    if (DOpts.NewsAdmin != NULL && *DOpts.NewsAdmin)
		from = DOpts.NewsAdmin;
	}
	if (from == NULL || !plainText(from)) {
	    fprintf(stderr, "dcanlock: give the From with -f (plain ASCII; newsadmin is not set)\n");
	    return(2);
	}
	if (newsgroups == NULL) {
	    if (!articleGroups(av[0], mid, groups, sizeof(groups))) {
		fprintf(stderr, "dcanlock: cannot read the Newsgroups of %s with dreadart; give them with -n\n", mid);
		return(2);
	    }
	    newsgroups = groups;
	}
	if (!groupsOk(newsgroups)) {
	    fprintf(stderr, "dcanlock: bad Newsgroups: %s\n", newsgroups);
	    return(2);
	}
	if (subject == NULL) {
	    snprintf(subj, sizeof(subj), "cmsg cancel %.*s", MAXMSGIDLEN, mid);
	    subject = subj;
	}
	if (strpbrk(body, "\r\n") != NULL || body[0] == '.') {
	    fprintf(stderr, "dcanlock: the body must be one line that does not start with '.'\n");
	    return(2);
	}
	if (!plainText(subject) || !plainText(charset)) {
	    fprintf(stderr, "dcanlock: the subject and the charset must be plain ASCII (MIME-encode the subject yourself)\n");
	    return(2);
	}
	if (!noKey)
	    makeList(&st, "key", mid, out);

	if (ConfigLoaded && DOpts.ReaderHostName != NULL && *DOpts.ReaderHostName)
	    snprintf(host, sizeof(host), "%s", DOpts.ReaderHostName);
	else if (gethostname(host, sizeof(host)) < 0 || *host == 0)
	    snprintf(host, sizeof(host), "localhost.invalid");
	if (utc) {
	    tm = gmtime(&now);
	    strftime(date, sizeof(date), "%a, %d %b %Y %H:%M:%S +0000", tm);
	} else {
	    tm = localtime(&now);
	    strftime(date, sizeof(date), "%a, %d %b %Y %H:%M:%S %z", tm);
	}

	printf("From: %s\r\n", from);
	printf("Newsgroups: %s\r\n", newsgroups);
	printf("Subject: %s\r\n", subject);
	printf("Date: %s\r\n", date);
	if (!noMid) {
	    messageId(newMid, sizeof(newMid), host);
	    printf("Message-ID: %s\r\n", newMid);
	}
	if (!noApproved)
	    printf("Approved: %s\r\n", from);
	printf("Control: cancel %s\r\n", mid);
	if (!noKey)
	    printf("Cancel-Key: %s\r\n", out);
	printf("MIME-Version: 1.0\r\n");
	printf("Content-Type: text/plain; charset=%s\r\n", charset);
	printf("Content-Transfer-Encoding: 8bit\r\n");
	printf("\r\n%s\r\n", body);
    }
    return(0);
}
