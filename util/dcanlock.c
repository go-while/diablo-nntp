/*
 * UTIL/DCANLOCK.C	- make and check Cancel-Keys and Cancel-Locks (RFC 8315)
 *
 *	dcanlock key  message-id ...	the Cancel-Key for withdrawing articles
 *	dcanlock lock message-id ...	the Cancel-Lock such an article carries
 *	dcanlock verify keys locks	whether a key matches a lock
 *
 *	The secrets are the canlockadmin and canlockuser lines of a
 *	vserverdef in dreader.access, the ones dreaderd makes the locks of
 *	local posts from.  Making a key is how an admin cancels an article
 *	that someone else posted: put the key into the Cancel-Key header of a
 *	cancel (or a Supersedes) message.
 */

#include "defs.h"

static void
Usage(const char *progname)
{
    fprintf(stderr, "Make and check Cancel-Keys and Cancel-Locks (RFC 8315)\n");
    fprintf(stderr, "Usage: %s [-C diablo.config] [-a dreader.access] [-v vserver]\n"
		    "\t[-u user] [-s scheme] [-S] key|lock message-id ...\n", progname);
    fprintf(stderr, "       %s verify cancel-keys cancel-locks\n", progname);
    fprintf(stderr, "\tkey\t\tprint the Cancel-Key header value for each Message-ID\n"
		    "\tlock\t\tprint the Cancel-Lock value for each Message-ID\n"
		    "\tverify\t\texit 0 when a key of the first value matches a lock of\n"
		    "\t\t\tthe second (no secret needed), 1 when none does\n"
		    "\t-a file\t\tthe dreader.access to read (default: the one of diablo.config)\n"
		    "\t-v vserver\tthe vserverdef the secrets are in (default DEFAULT)\n"
		    "\t-u user\t\tuse the canlockuser secrets and this user (default: the\n"
		    "\t\t\tcanlockadmin secrets)\n"
		    "\t-s scheme\tonly sha256, sha1 or sha512 (default: sha256, and sha1\n"
		    "\t\t\tunless the vserverdef says canlocksha1 no)\n"
		    "\t-S\t\tread the secret from the first line of standard input\n"
		    "\t\t\tinstead of dreader.access\n");
    exit(2);
}

typedef struct Secrets {
    char admin[CANLOCK_MAXSECRETS][CANLOCK_SECRETLEN];
    char user[CANLOCK_MAXSECRETS][CANLOCK_SECRETLEN];
    int noSha1;
} Secrets;

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

int
main(int ac, char **av)
{
    const char *accessFile = NULL;
    const char *vsName = "DEFAULT";
    const char *user = NULL;
    const char *only = NULL;
    const char *cmd;
    Secrets sec;
    int stdinSecret = 0;
    int c;
    int i;

    while ((c = getopt(ac, av, "C:a:v:u:s:S")) != -1) {
	switch (c) {
	case 'C':
	    break;			/* LoadDiabloConfig() reads it from av */
	case 'a':
	    accessFile = optarg;
	    break;
	case 'v':
	    vsName = optarg;
	    break;
	case 'u':
	    user = optarg;
	    break;
	case 's':
	    only = optarg;
	    break;
	case 'S':
	    stdinSecret = 1;
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
    if ((strcmp(cmd, "key") != 0 && strcmp(cmd, "lock") != 0) || optind >= ac)
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
    if (stdinSecret) {
	char line[CANLOCK_SECRETLEN + 8];

	if (fgets(line, sizeof(line), stdin) == NULL) {
	    fprintf(stderr, "dcanlock: no secret on standard input\n");
	    return(2);
	}
	addSecret(user ? sec.user : sec.admin, user ? "canlockuser" : "canlockadmin", line);
    } else {
	if (accessFile == NULL) {
	    LoadDiabloConfig(ac, av);	/* for the path of dreader.access */
	    accessFile = PatLibExpand(DReaderAccessPat);
	}
	if (!readAccess(accessFile, vsName, &sec)) {
	    fprintf(stderr, "dcanlock: no vserverdef %s in %s\n", vsName, accessFile);
	    return(2);
	}
    }
    {
	char (*secrets)[CANLOCK_SECRETLEN] = user ? sec.user : sec.admin;
	const char *uid = user ? user : "";
	static const char *schemes[] = { "sha256", "sha1" };
	int nSchemes = 2;

	if (secrets[0][0] == 0) {
	    fprintf(stderr, "dcanlock: no %s secret in vserverdef %s\n",
		    user ? "canlockuser" : "canlockadmin", vsName);
	    return(2);
	}
	if (only) {
	    schemes[0] = only;
	    nSchemes = 1;
	} else if (sec.noSha1) {
	    nSchemes = 1;
	}
	for (i = optind; i < ac; ++i) {
	    char out[CANLOCK_MAXSECRETS * 3 * 160];
	    int j;
	    int s;

	    if (!validMsgId(av[i])) {
		fprintf(stderr, "dcanlock: not a Message-ID (with its <>): %s\n", av[i]);
		return(2);
	    }
	    out[0] = 0;
	    for (j = 0; j < CANLOCK_MAXSECRETS && secrets[j][0]; ++j) {
		for (s = 0; s < nSchemes; ++s) {
		    char element[192];
		    int len;

		    if (strcmp(cmd, "key") == 0)
			len = CanLockKey(schemes[s], secrets[j], uid, av[i], element, sizeof(element));
		    else
			len = CanLockLock(schemes[s], secrets[j], uid, av[i], element, sizeof(element));
		    if (len < 0) {
			fprintf(stderr, "dcanlock: cannot make the %s %s for %s\n",
				schemes[s], cmd, av[i]);
			return(2);
		    }
		    put(out, sizeof(out), element);
		}
	    }
	    printf("%s\n", out);
	}
    }
    return(0);
}
