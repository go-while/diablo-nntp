
/*
 * DICMD.C	- diablo command issuer
 *
 * Issue one or more commands to the diablo parent
 *
 * (c)Copyright 1997, Matthew Dillon, All Rights Reserved.  Refer to
 *    the COPYRIGHT file in the base directory of this distribution 
 *    for specific rights granted.
 */

#include "defs.h"

int
main(int ac, char **av)
{
    FILE *fi;
    FILE *fo;
    char buf[256];
    int r = 0;

    LoadDiabloConfig(ac, av);

    /*
     * UNIX domain socket
     */

    {
	struct sockaddr_un soun;
	int ufd;
	const char *sockpath;

	memset(&soun, 0, sizeof(soun));

	if ((ufd = socket(AF_UNIX, SOCK_STREAM, 0)) < 0) {
	    perror("udom-socket");
	    exit(1);
	}
	soun.sun_family = AF_UNIX;
	sockpath = PatRunExpand(DiabloSocketPat);
	if (strlen(sockpath) >= sizeof(soun.sun_path)) {
	    fprintf(stderr, "socket path too long: %s\n", sockpath);
	    exit(1);
	}
	snprintf(soun.sun_path, sizeof(soun.sun_path), "%s", sockpath);
	stprintf("connecting to %s", soun.sun_path);
	if (connect(ufd, (struct sockaddr *)&soun, offsetof(struct sockaddr_un, sun_path[strlen(soun.sun_path)+1])) < 0) {
	    perror("udom-connect");
	    exit(1);
	}
	stprintf("connected to %s", soun.sun_path);
	fo = fdopen(dup(ufd), "w");
	fi = fdopen(ufd, "r");
    }

    if (ac == 1) {
	/*
	 * A line longer than buf comes in pieces (D-127): send them all
	 * before reading the reply, and take ".\n" as the end of the reply
	 * only when it is a whole line.
	 */
	while(fgets(buf, sizeof(buf), stdin) != NULL) {
	    size_t n = strlen(buf);
	    int bol = 1;

	    r = 1;
	    stprintf("sending %s", buf);
	    fprintf(fo, "%s", buf);
	    fflush(fo);
	    if (n == 0 || buf[n - 1] != '\n')
		continue;
	    while (fgets(buf, sizeof(buf), fi) != NULL) {
	        stprintf("received %s", buf);
		printf("%s", buf);
		if (bol && strcmp(buf, ".\n") == 0) {
		    r = 0;
		    break;
		}
		n = strlen(buf);
		bol = (n > 0 && buf[n - 1] == '\n');
	    }
	}
    } else {
	int i;
	int sent = 0;

	for (i = 1; i < ac; ++i) {
	    char *ptr = av[i];
	    if (sent == 0 && *ptr == '-') {
		/* options before the command: skip -C file */
		if (ptr[1] == 'C' && ptr[2] == 0)
		    ++i;
		continue;
	    }
	    stprintf("sending %s%s", sent ? " " : "", ptr);
	    fprintf(fo, "%s%s", sent++ ? " " : "", ptr);
	}
	fprintf(fo, "\n");
	fprintf(fo, "quit\n");
	fflush(fo);
	r = 1;
	{
	    int bol = 1;	/* see above: a long reply line comes in pieces */

	    while (fgets(buf, sizeof(buf), fi) != NULL) {
		size_t n = strlen(buf);

		stprintf("received %s", buf);
		if (bol && strcmp(buf, ".\n") == 0) {
		    r = 0;
		    break;
		}
		printf("%s", buf);
		bol = (n > 0 && buf[n - 1] == '\n');
	    }
	}
    }
    return(r);
}

