/*-
 * Copyright (c) 2001 Russell Vincent
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
 * Program to lock the history file hash table in memory using mlock.
 */

#include "defs.h"

/*
 * -a: lock the grown file once it is this fraction (1/64) larger than what is locked.
 * The unlocked tail holds the newest records, which a running feeder keeps cached.
 */
#define GROW_SHIFT	6

void
usage(char *progname) {
    printf("Usage: %s [-a] [-f] [-s sleeptime] historypath\n\n", progname);
    printf(" Mlock the history hash table into memory\n");
    printf("\t-a           = lock the entire history file into memory\n");
    printf("\t-f           = don't check for root user\n");
    printf("\t-s sleeptime = specify the time (in secs) between checks for new history\n");
    printf("\n");
    printf("WARNING: This program has to be run as the root user due to OS\n");
    printf("  restrictions on the mlock() function call.\n");
    printf("  Doing so can compromise security. Make sure you understand\n");
    printf("  what it does, how and why before trusting it.\n");
    exit(1);
}

/*
 * Map and lock the whole file; its size and inode come from the open file, so a file
 * replaced after the caller's stat() is never mapped past its end. Exits on failure:
 * mlock() refused (privilege, RLIMIT_MEMLOCK, memory) is a setup problem to fix.
 */
static char *
lockFile(const char *path, off_t *size, ino_t *ino)
{
    struct stat st;
    char *map;
    int fd;

    if ((fd = open(path, O_RDONLY)) < 0 || fstat(fd, &st) < 0) {
	fprintf(stderr, "dlockhistory: %s: %s\n", path, strerror(errno));
	exit(1);
    }
    if (st.st_size <= 0 || (off_t)(size_t)st.st_size != st.st_size) {
	fprintf(stderr, "dlockhistory: %s: cannot map %lld bytes\n", path, (long long)st.st_size);
	exit(1);
    }
    map = xmap(NULL, (size_t)st.st_size, PROT_READ, MAP_SHARED, fd, 0);
    close(fd);
    if (map == NULL) {
	fprintf(stderr, "dlockhistory: mmap %s: %s\n", path, strerror(errno));
	exit(1);
    }
    if (mlock(map, (size_t)st.st_size) < 0) {
	fprintf(stderr, "dlockhistory: mlock %lld bytes of %s: %s\n",
		(long long)st.st_size, path, strerror(errno));
	exit(1);
    }
    *size = st.st_size;
    *ino = st.st_ino;
    printf("Locked %lld bytes of %s\n", (long long)st.st_size, path);
    fflush(stdout);
    return(map);
}

static void
unlockFile(char *map, off_t size)
{
    munlock(map, (size_t)size);
    xunmap((void *)map, (size_t)size);
}

/*
 * -a: keep the whole file locked. The feeder appends to the history all the time, so
 * a grown file is locked before the old lock is dropped (the pages stay locked
 * throughout), and only after GROW_SHIFT growth. A replaced (or shorter) file is
 * locked after the old lock is dropped, since two locked copies may not fit.
 */
static void
lockWhole(const char *path, int sleepTime)
{
    struct stat st;
    char *map;
    off_t mapsize;
    ino_t ino;

    map = lockFile(path, &mapsize, &ino);
    for (;;) {
	sleep(sleepTime);
	if (stat(path, &st) != 0)
	    continue;	/* between dhisexpire's unlink and rename */
	if (st.st_ino == ino && st.st_size >= mapsize) {
	    char *nmap;
	    off_t nsize;
	    ino_t nino;

	    if (st.st_size - mapsize < (mapsize >> GROW_SHIFT))
		continue;
	    nmap = lockFile(path, &nsize, &nino);
	    unlockFile(map, mapsize);
	    map = nmap;
	    mapsize = nsize;
	    if (nino == ino)
		continue;
	    ino = nino;	/* replaced after the stat: already locked, just report it */
	} else {
	    unlockFile(map, mapsize);
	    map = lockFile(path, &mapsize, &ino);
	}
	printf("New history\n");
	fflush(stdout);
    }
}

int
main(int argc, char **argv) {
    extern char *optarg;
    extern int optind;
    char *progname = *argv;
    char *HistoryFile;
    int SleepTime = 5;
    int Opened = 0;
    int ch;
    struct stat st;
    int PrevIno = -1;
    int Force = 0;
    int All = 0;

    optind = 1;
    while ((ch = getopt(argc, argv, "afs:V")) != -1) {
	switch(ch) {
	    case 'a':
		All = 1;
		break;
	    case 'f':
		Force = 1;
		break;
	    case 's':
		SleepTime = strtol(optarg, NULL, 0);
		break;
	    case 'V':
		PrintVersion();
		break;
	    default:
		usage(argv[0]);
	}
    }

    argv += optind;
    if (*argv == NULL)
	usage(progname);
    HistoryFile = *argv;

    if (!Force && geteuid() != 0) {
	printf("This daemon must be run as root due to the use of mlock()\n");
	printf("which is only allowed to be executed by the root user\n");
	exit(1);
    }

    if (All)
	lockWhole(HistoryFile, SleepTime);	/* does not return */

    while (1) {
	if (!Opened) {
	    if (stat(HistoryFile, &st) == 0)
		PrevIno = st.st_ino;
	    HistoryOpen(HistoryFile, HGF_MLOCK);
	    Opened = 1;
	}
	sleep(SleepTime);
	if (stat(HistoryFile, &st) != 0 || st.st_ino != PrevIno) {
	    HistoryClose();
	    Opened = 0;
	    printf("New history\n");
	    fflush(stdout);
	}
    }
}
