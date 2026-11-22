/*-
 * Copyright (c) 2000 Russell Vincent
 * All rights reserved.
 *
 * Redistribution and use in source and binary forms, with or without
 * modification, are permitted provided that the following conditions
 * are met:
 * 1. Redistributions of source code must retain the above copyright
 *    notice, this list of conditions and the following disclaimer.
 * 2. Redistributions in binary form must reproduce the above copyright
 *    notice, this list of conditions and the following disclaimer in the
 *    documentation and/or other materials provided with the distribution.
 *
 * THIS SOFTWARE IS PROVIDED BY THE AUTHOR AND CONTRIBUTORS ``AS IS'' AND
 * ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
 * IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE
 * ARE DISCLAIMED.  IN NO EVENT SHALL THE AUTHOR OR CONTRIBUTORS BE LIABLE
 * FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
 * DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS
 * OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION)
 * HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT
 * LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY
 * OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF
 * SUCH DAMAGE.
 *
 */

/*
 * Program to connect to a news server and execute commands and optionally
 * report on the performance handling the commands
*/

#include "defs.h"

int verbose = 0;
char *hostname;
int s;
struct timeval connectstart;
struct timeval connectend;
struct timeval commandstart;
struct timeval commandend;
struct timeval totaltime;
FILE *fp;
char line[512];
int response;
int numarts;
int lowmark;
int highmark;

void
usage(char *progname) {
    printf("Usage: %s [-f filename] [-p port] [-v] hostname [command]\n\n", progname);
    printf(" Execute a command on an NNTP server and write output to stdout\n");
    printf(" If no command is specified then read commands from STDIN\n\n");
    printf("\t-f file = read NNTP commands from a file and send them to server\n");
    printf("\t-v      = be more verbose in output\n");
    printf("\t-p port = specify the remote port (default: 119)\n");
    exit(1);
}

/*
 * Keep the start for reply-code and dot checks, but consume the line through
 * its newline.  Continuation pieces are printed without interpreting them.
 */
static int
ReadReplyLine(char *line, int size, int show)
{
    char part[MAXLINE];
    size_t len = 0;
    int kept = 0;
    int ch;
    int seen = 0;

    while ((ch = fgetc(fp)) != EOF) {
	seen = 1;
	if (kept < size - 1)
	    line[kept++] = ch;
	part[len++] = ch;
	if (len == sizeof(part) || ch == '\n') {
	    if (show)
		fwrite(part, 1, len, stdout);
	    len = 0;
	}
	if (ch == '\n')
	    break;
    }
    line[kept] = 0;
    if (show && len)
	fwrite(part, 1, len, stdout);
    /* A clean EOF may end a final fragment, but a read error is not a reply. */
    if (ferror(fp))
	return(-1);
    return(seen);
}

void
connectnews(char *hostname, char *port) {
    struct addrinfo hints, *res, *ai;
    int err;
    char line[512];

    memset(&hints, 0, sizeof(hints));
    hints.ai_family = AF_UNSPEC;
    hints.ai_socktype = SOCK_STREAM;
    if ((err = getaddrinfo(hostname, port, &hints, &res)) != 0) {
	fprintf(stderr, "getaddrinfo: %s\n", gai_strerror(err));
	exit(1);
    }

    /* try each address in the order the resolver gave them */
    s = -1;
    gettimeofday(&connectstart, NULL);
    for (ai = res; ai != NULL; ai = ai->ai_next) {
	if ((s = socket(ai->ai_family, ai->ai_socktype, ai->ai_protocol)) < 0)
	    continue;
	if (connect(s, ai->ai_addr, ai->ai_addrlen) == 0)
	    break;
	close(s);
	s = -1;
    }
    freeaddrinfo(res);
    if (s < 0) {
	perror("connect");
	exit(1);
    }
    if ((fp = fdopen(s, "r+")) == NULL) {
	perror("fdopen");
	exit(1);
    }
    setvbuf(fp, NULL, _IOLBF, 0);
    if (ReadReplyLine(line, sizeof(line), verbose) <= 0) {
	printf("No response from server\n");
	exit(2);
    }
    if (NntpCode(line) != 200 && NntpCode(line) != 201) {
	printf("Unexpected response from server: %s", line);
	exit(2);
    }
}

/*
 * RFC 3977 replies that are followed by a multi-line block.  211 is
 * multi-line only after LISTGROUP.  -1: the line has no reply code
 * (three digits, then a space or the end of the line).
 */
static int
MultiLineReply(const char *cmd, const char *line)
{
    int code = NntpCode(line);

    if (code < 0)
	return(-1);
    switch (code) {
    case 100: case 101: case 215: case 220: case 221: case 222:
    case 202: case 224: case 225: case 230: case 231: case 282:
	return(1);
    case 211:
	return(strncasecmp(cmd, "LISTGROUP", 9) == 0);
    }
    return(0);
}

void
newscommand(char *command) {
    char line[MAXLINE];
    char inp[MAXLINE];

    if (verbose)
	printf(">>%s\r\n", command ? command : "STDIN");

    while (command || fgets(inp, sizeof(inp), stdin) != NULL) {
	const char *cmd = command;
	int multi = -1;
	int got;

	if (cmd == NULL) {
	    inp[strcspn(inp, "\r\n")] = 0;
	    cmd = inp;
	}
	fflush(fp);
	fprintf(fp, "%s\r\n", cmd);
	rewind(fp);
	while ((got = ReadReplyLine(line, sizeof(line), 1)) > 0) {
	    if (multi < 0) {
		multi = MultiLineReply(cmd, line);
		if (multi < 0) {
		    /* what follows could not be told from the next reply */
		    printf("Unexpected response from server\n");
		    exit(2);
		}
		if (multi == 0)
		    break;
	    } else if (strncmp(line, ".\r\n", 3) == 0) {
		break;
	    }
	}
	if (got < 0) {
	    printf("Error reading response from server\n");
	    exit(2);
	}
	if (command)
	    break;
    }
}

/* set by dodumpfile(): replies to the dumped commands were not read */
static int dumped = 0;

void
dodumpfile(char *filename)
{
    FILE *f;
    char line[MAXLINE];
    int heldcr = 0;
    int partial = 0;	/* the last piece was full: its line is not ended */

    if ((f = fopen(filename, "r")) == NULL) {
	printf("Unable to open %s\n", filename);
	exit(1);    
    }
    /*
     * Each line goes out ending in CR LF.  fgets() cuts a long line into
     * pieces of 16383 bytes: the piece that ends the line, or
     * the last one of the file (which may have no newline, or end exactly
     * at 16383 bytes), gets the CR LF.  A CR at the end of a full
     * piece may belong to the CR LF that follows, so it is held back
     * until we know.
     *
     * fp was last read (the greeting): flush before the first write, as
     * newscommand() does.  Without it a libc may write the bytes just read
     * back to the server (seen on illumos: the greeting in front of the file).
     */
    fflush(fp);
    while (fgets(line, sizeof(line), f) != NULL) {
	size_t len = strlen(line);
	int eol = (len > 0 && line[len - 1] == '\n');
	int full = (len == sizeof(line) - 1 && !eol);

	if (heldcr) {
	    if (!(eol && len == 1))
		fputc('\r', fp);
	    heldcr = 0;
	}
	if (eol) {
	    len--;
	    if (len > 0 && line[len - 1] == '\r')
		len--;
	} else if (full && line[len - 1] == '\r') {
	    len--;
	    heldcr = 1;
	}
	fwrite(line, 1, len, fp);
	partial = full;
	if (!full)
	    fputs("\r\n", fp);
    }
    /* the file ended in the middle of a long line, or after a held CR */
    if (heldcr)
	fputc('\r', fp);
    if (partial)
	fputs("\r\n", fp);
    fclose(f);
    dumped = 1;
}

void
newsquit(void) {
    char line[512];
    int code;

    fflush(fp);
    fprintf(fp,"QUIT\r\n");
    rewind(fp);
    if (ReadReplyLine(line, sizeof(line), verbose) <= 0)
	line[0] = 0;
    /*
     * After -f the replies to the dumped commands were not read, so this
     * line is the reply to the first of them, not to QUIT: as before, a
     * 238 (a dumped CHECK) counts too.
     */
    code = NntpCode(line);
    if (code != 205 && !(dumped && code == 238)) {
	printf("Unexpected response from server: %s", line);
	exit(2);
    }
}

int
main(int argc, char **argv) {
    extern char *optarg;
    extern int optind;
    int ch;
    char *progname;
    char *port = "119";
    char *command;
    char *dumpfile = NULL;

    progname = *argv;

    optind = 1;
    while ((ch = getopt(argc, argv, "f:p:Vv")) != -1) {
	switch(ch) {
	    case 'f':
		dumpfile = optarg;
		break;
	    case 'p':
		port  = optarg;
		break;
	    case 'V':
		PrintVersion();
		break;
	    case 'v':
		verbose = 1;
		break;
	    default:
		usage(argv[0]);
	}
    }

    argv += optind;
    if (*argv == NULL)
	usage(progname);
    hostname = *argv++;
    if (*argv != NULL)
	command = *argv++;
    else
	command = NULL;

    connectnews(hostname, port);
    gettimeofday(&connectend, NULL);

    gettimeofday(&commandstart, NULL);
    if (dumpfile != NULL)
	dodumpfile(dumpfile);
    else
	newscommand(command);
    gettimeofday(&commandend, NULL);

    newsquit();

    gettimeofday(&totaltime, NULL);

    if (verbose) {
    	printf("Connect time: %3.3f\n",
		(((connectend.tv_sec * 1000000.0) + connectend.tv_usec) -
		((connectstart.tv_sec * 1000000.0) + connectstart.tv_usec)) /
				1000000);
	printf("Elapsed time: %3.3f\n",
		(((totaltime.tv_sec * 1000000.0) + totaltime.tv_usec) -
		((connectstart.tv_sec * 1000000.0) + connectstart.tv_usec)) /
				1000000);
	printf("Command time: %3.3f\n",
		(((commandend.tv_sec * 1000000.0) + commandend.tv_usec) -
		((commandstart.tv_sec * 1000000.0) + commandstart.tv_usec)) /
				1000000);
    }
    exit(0);
}
