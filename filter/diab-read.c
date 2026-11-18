/*-
 * Copyright (c) 1998 Joe Greco and sol.net Network Services
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

#include "defs.h"

Prototype int diab_read(char *namepos, char *buf, int szbuf);

int
diab_read(char *namepos, char *buf, int szbuf)
{
	static char filename[FILENAMELEN];
	static char name[NAMELEN], *ptr, *nptr;
	static char lastname[NAMELEN] = { 0 };
	static int fd = -1;
	int offset, size, nsize;
	int rval;

	for (ptr = namepos, nptr = name; *ptr && *ptr != ':' && *ptr != ' '; ) {
		*nptr++ = *ptr++;
	}
	*nptr = '\0';

	if (*ptr != ':' && *ptr != ' ') {
#ifdef	DEBUG
		fprintf(stderr, "error: parsing article location, no colon\n");
#endif
		return(-1);
	}
	ptr++;

	/* XXX this is not right, of course */
	if (sscanf(ptr, "%d,%d", &offset, &size) != 2) {
#ifdef	DEBUG
		fprintf(stderr, "error: parsing article location, sscanf\n");
#endif
		return(-1);
	}
	/* every record starts with the spool header */
	if (size < (int)sizeof(SpoolArtHdr))
		return(-1);
	if (strcmp(lastname, name)) {
		if (fd >= 0) {
			close(fd);
			fd = -1;
		}
	}
	if (fd < 0) {
		if (name[0] == '/')
			strncpy(filename, name, sizeof(filename) -1);
		else
			snprintf(filename, sizeof(filename), "%s/%s",
						PatExpand(SpoolHomePat), name);

		if ((fd = open(filename, O_RDONLY, 0)) < 0) {
#ifdef	DEBUG
		fprintf(stderr, "error: opening article file, open fails, %d\n", errno);
#endif
			return(-1);
		}
		snprintf(lastname, sizeof(lastname), "%s", name);
	}
	/*
	 * A record of a compressed spool object (compresslvl) is the spool
	 * header, the article as one gzip member and the NUL; size is the
	 * header plus the uncompressed article.  It is inflated into buf
	 * like a plain record would be read: before, its gzip bytes failed
	 * the checks below and every article of such a spool went out
	 * unfiltered.
	 */
	if (size < szbuf) {
		SpoolArtHdr ah;

		if (pread(fd, &ah, sizeof(ah), (off_t)offset) != sizeof(ah)) {
#ifdef	DEBUG
		fprintf(stderr, "error: reading article header, %d\n", errno);
#endif
			return(-1);
		}
		if ((uint8)ah.Magic1 == STORE_MAGIC1 &&
		    (uint8)ah.Magic2 == STORE_MAGIC2 &&
		    (ah.StoreType & STORETYPE_GZIP)) {
			if (ah.HeadLen != sizeof(SpoolArtHdr) ||
			    (size_t)ah.ArtLen + sizeof(SpoolArtHdr) != (size_t)size)
				return(-1);
			bcopy(&ah, buf, sizeof(ah));
			if (SpoolGunzip(fd, (off_t)offset + sizeof(ah),
					buf + sizeof(ah), (int)ah.ArtLen) < 0)
				return(-1);
			buf[size] = '\0';
			return((int)ah.ArtLen);
		}
	}
	if (lseek(fd, (off_t)offset, SEEK_SET) < 0) {
#ifdef	DEBUG
		fprintf(stderr, "error: seeking to article location, %d\n", errno);
#endif
		return(-1);
	}
	nsize = size + 1;
	if (nsize > szbuf) {
		/* Too big for buffer - read what we can */
		if ((rval = read(fd, buf, szbuf)) != szbuf) {
#ifdef	DEBUG
		fprintf(stderr, "error: reading article data (toobig), read fails, %d\n", errno);
#endif
			return(-1);
		}
		/* Can't check last byte that didn't fit */
		return(rval);
	} else {
		/* Fits in buffer - go for it */
		if ((rval = read(fd, buf, nsize)) != nsize) {
#ifdef	DEBUG
		fprintf(stderr, "error: reading article data (itfits), read fails, %d, %d read, %d wanted, %d curloc, %d fd, %x buf\n", errno, rval, nsize, (int)lseek(fd, 0, SEEK_CUR), fd, buf);
#endif
			return(-1);
		}
		ptr = buf + sizeof(SpoolArtHdr);
		rval -= sizeof(SpoolArtHdr);

		/*
		 * The article is passed on by its length: a NUL byte in it
		 * (rejectartswithnul is off by default) failed the read
		 * before, and the article went out unfiltered.  Only the
		 * stored terminator after it must be the NUL.
		 */
		ptr += rval - 1;
		if (*ptr) {
#ifdef	DEBUG
		fprintf(stderr, "error: checking article data (itfits), last byte not null\n");
#endif
			return(-1);
		}
		return(rval - 1);
	}
}


#if 0
int main()
{
	char buffer[8192];
	char n[8192];
	int start = time(NULL);
	int i;

	while (! feof(stdin)) {
		gets(buffer);
		if (! feof(stdin)) {
			printf("%s %d\n", buffer, diab_read(buffer, n, sizeof(n)));
			i++;
			fflush(stdout);
		}
	}
	printf("\n%d arts in %d secs\n", i, time(NULL) - start);
}
#endif

#ifdef MAINLINE
int main(int argc, char *argv[])
{
	char abuf[1100000];
	int rval;

	if (argc != 2) {
		fprintf(stderr, "diab-read: usage: diab-read <article>\n");
		exit(1);
	}
	if ((rval = diab_read(argv[1], abuf, sizeof(abuf))) < 0) {
		perror("diab_read");
		exit(1);
	}
	write(fileno(stdout), abuf, rval);
	exit(0);
}
#endif

