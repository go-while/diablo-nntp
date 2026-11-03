
/*
 * UTIL/DKP.C
 *
 * DKP -w filename [key tok=data]
 * DKP filename [key]
 *
 * (c)Copyright 1998, Matthew Dillon, All Rights Reserved.  Refer to
 *    the COPYRIGHT file in the base directory of this distribution
 *    for specific rights granted.
 */

#include "defs.h"

/*
 * Read one line of any length from fi into *pbuf (grown as needed), without
 * its newline.  Returns 0 at the end of the input, else 1 (also for a last
 * line without a newline).  A line that cannot be stored (out of memory) is
 * skipped whole and reported, so its tail is never read as a line of its own.
 */
static int
readLine(FILE *fi, char **pbuf, size_t *psize)
{
    size_t len = 0;
    int c;
    int lost = 0;

    while ((c = getc(fi)) != EOF && c != '\n') {
	if (lost)
	    continue;
	if (len + 1 >= *psize) {
	    size_t nsize = (*psize) ? *psize * 2 : 1024;
	    char *nbuf = realloc(*pbuf, nsize);

	    if (nbuf == NULL) {
		lost = 1;
		continue;
	    }
	    *pbuf = nbuf;
	    *psize = nsize;
	}
	(*pbuf)[len++] = c;
    }
    if (c == EOF && len == 0 && !lost)
	return(0);
    if (lost) {
	fprintf(stderr, "Input line too long for memory, skipped\n");
	len = 0;
    }
    if (*pbuf != NULL)
	(*pbuf)[len] = 0;
    return(1);
}

int
main(int ac, char **av)
{
    char *fileName = NULL;
    char *key = NULL;
    char *tokval = NULL;
    char *val;
    int i;
    int readMode = 0;
    int writeMode = 0;
    int forceSort = 0;
    int deleteMode = 0;
    int trimMode = 0;
    KPDB *db;

    for (i = 1; i < ac; ++i) {
	char *ptr = av[i];

	if (*ptr != '-') {
	    if (fileName == NULL)
		fileName = ptr;
	    else if (key == NULL)
		key = ptr;
	    else if (tokval == NULL)
		tokval = ptr;
	    continue;
	}
	ptr += 2;
	switch(ptr[-1]) {
	case 'd':
	    DebugOpt = 1;
	    break;
	case 'x':
	    deleteMode = 1;
	    break;
	case 'r':
	    readMode = 1;
	    break;
	case 'w':
	    readMode = 1;
	    writeMode = 1;
	    break;
	case 's':
	    forceSort = 1;
	    break;
	case 't':
	    trimMode = 1;
	    break;
	case 'V':
	    PrintVersion();
	    break;
	default:
	    break;
	}
    }

    if (fileName == NULL) {
	printf("DKP [-d] [-s] [-r] [-w] filename [key [tok=data]]\n");
	exit(0);
    }

    if (trimMode == 1) {
	/*
	 * trim file, remove dead entries.  Only works if there are no other
	 * users of the file.
	 */
	int fd = open(fileName, O_RDWR);

	forceSort = 1;

	if (fd >= 0) {
	    struct stat st;
	    char *buf = malloc(strlen(fileName) + 32);
	    FILE *fi;
	    FILE *fo;
	    int tfd;
	    int failed = 0;

	    if (buf == NULL) {
		fprintf(stderr, "Unable to allocate temporary filename\n");
		close(fd);
		return(1);
	    }

	    if (hflock(fd, 0, XLOCK_EX|XLOCK_NB) < 0) {
		printf("Unable to exclusively lock %s, someone else is using it\n", fileName);
		exit(1);
	    }
	    if (fstat(fd, &st) < 0 || st.st_nlink == 0) {
		printf("File was ripped out from under us, exiting\n");
		exit(1);
	    }
	    snprintf(buf, strlen(fileName) + 32, "%s.tmp%d", fileName, (int)getpid());
	    fi = fdopen(fd, "r");
	    if (fi == NULL) {
		fprintf(stderr, "Unable to read %s: %s\n", fileName, strerror(errno));
		close(fd);
		free(buf);
		return(1);
	    }
	    tfd = open(buf, O_WRONLY|O_CREAT|O_EXCL, 0640);
	    fo = (tfd >= 0) ? fdopen(tfd, "w") : NULL;
	    if (tfd >= 0 && fo == NULL) {
		int error = errno;
		close(tfd);
		unlink(buf);
		errno = error;
	    }
	    if (fo) {
		int c;
		int bol = 1;	/* at the start of a record */
		int skip = 0;	/* the current record is deleted */

		/*
		 * Copy byte by byte: a record of any length is dropped up to
		 * and including its newline, and a NUL byte (a torn write)
		 * neither ends nor truncates a record, as it did with fgets()
		 * and strlen().
		 */
		while ((c = getc(fi)) != EOF) {
		    if (bol)
			skip = (c == '-');
		    if (!skip && putc(c, fo) == EOF)
			break;
		    bol = (c == '\n');
		}
		if (ferror(fi)) {
		    fprintf(stderr, "Error reading %s: %s\n", fileName, strerror(errno));
		    failed = 1;
		}
		if (ferror(fo)) {
		    fprintf(stderr, "Error writing %s\n", buf);
		    failed = 1;
		}
		if (fclose(fo) != 0) {
		    fprintf(stderr, "Error writing %s: %s\n", buf, strerror(errno));
		    failed = 1;
		}
		if (!failed && rename(buf, fileName) != 0) {
		    fprintf(stderr, "Unable to install %s: %s\n", buf, strerror(errno));
		    failed = 1;
		}
		if (failed)
		    unlink(buf);
		fclose(fi);
		free(buf);
		if (failed)
		    return(1);
	    } else {
		fprintf(stderr, "Unable to create %s: %s\n", buf, strerror(errno));
		fclose(fi);
		free(buf);
		return(1);
	    }
	} else {
	    fprintf(stderr, "Unable to open %s: %s\n", fileName, strerror(errno));
	    return(1);
	}
    }

    db = KPDBOpen(fileName, O_RDWR | ((writeMode) ? O_CREAT : 0));

    if (db == NULL) {
	fprintf(stderr, "Unable to open KPDB %s\n", fileName);
	exit(0);
    }

    if (forceSort) {
	KPDBReSort(db);
    }

    if (key == NULL && (readMode || writeMode)) {
	char *buf = NULL;
	size_t bufSize = 0;

	/*
	 * A line of any length is one record: with fgets() into a fixed
	 * buffer the rest of a long line was taken as a record of its own.
	 */
	while (readLine(stdin, &buf, &bufSize)) {
	    char *key = (buf != NULL) ? strtok(buf, " \t\n") : NULL;
	    char *tokval = (key) ? strtok(NULL, " \t\n") : NULL;

	    if (deleteMode && key) {
		KPDBDelete(db, key);
	    } else if (writeMode && key && tokval && (val = strchr(tokval, '='))) {
		*val++ = 0;
		KPDBWrite(db, key, tokval, val, 0);
	    } else if (readMode && key) {
		const char *data;
		int dataLen;

		if ((data = KPDBReadRecord(db, key, 0, &dataLen)) == NULL) {
		    printf("key %s data <not-found>\n", key);
		} else {
		    printf("key %s data (%d) %*.*s", key, dataLen, dataLen, dataLen, data);
		}
	    }
	}
	free(buf);
    } else if (deleteMode && key) {
	KPDBDelete(db, key);
    } else if (writeMode && key && tokval && (val = strchr(tokval, '='))) {
	*val++ = 0;
	(void)KPDBWrite(db, key, tokval, val, 0);
    } else if (readMode && key) {
	const char *data;
	int dataLen;

	if ((data = KPDBReadRecord(db, key, 0, &dataLen)) == NULL) {	/* XXX not locking thru printf */
	    printf("<not found>\n");
	} else {
	    printf("key %s data (%d) %*.*s", key, dataLen, dataLen, dataLen, data);
	}
    }
    KPDBClose(db);
    return(0);
}

