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
	int offset, size, skip, alen;
	ssize_t n;
	SpoolArtHdr ah;

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
	if (offset < 0 || size <= 0)
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
	 * The record, read the way the feeder reads it to serve it
	 * (SendArticle(), MapArticle()).  With the magic: a spool header
	 * of HeadLen bytes, the article and a NUL; size is the header plus
	 * the article.  A header that fails the feeder's checks is refused.
	 * Without the magic: an old record with no header, size is the
	 * article.  On a compressed spool object (compresslvl) the article
	 * is one gzip member after a full header: it is inflated.  Before,
	 * every record was taken to have a full header, and a NUL byte
	 * anywhere (gzip data, or an article with a NUL, rejectartswithnul
	 * being off by default) failed the read: the article went out
	 * unfiltered.
	 *
	 * In every case the article is put into buf after
	 * sizeof(SpoolArtHdr) bytes, followed by a NUL, and its length is
	 * returned; -1 if it cannot be read.
	 */
	memset(&ah, 0, sizeof(ah));
	if ((n = pread(fd, &ah, sizeof(ah), (off_t)offset)) < 0) {
#ifdef	DEBUG
		fprintf(stderr, "error: reading article header, %d\n", errno);
#endif
		return(-1);
	}
	skip = 0;
	if (n >= 2 && (uint8)ah.Magic1 == STORE_MAGIC1 &&
	    (uint8)ah.Magic2 == STORE_MAGIC2) {
		if (n != (ssize_t)sizeof(ah) || size < (int)sizeof(ah) ||
		    ah.HeadLen < 8 || ah.HeadLen > sizeof(ah) ||
		    ah.ArtHdrLen > (uint32)(size - ah.HeadLen))
			return(-1);
		if (ah.StoreType & STORETYPE_GZIP) {
			if (ah.HeadLen != sizeof(ah) || ah.ArtLen == 0 ||
			    (size_t)ah.ArtLen + sizeof(ah) != (size_t)size ||
			    size >= szbuf)
				return(-1);
			if (SpoolGunzip(fd, (off_t)offset + sizeof(ah),
					buf + sizeof(ah), (int)ah.ArtLen) < 0)
				return(-1);
			buf[size] = '\0';
			return((int)ah.ArtLen);
		}
		skip = ah.HeadLen;
	}
	alen = size - skip;
	if (alen > szbuf - (int)sizeof(SpoolArtHdr) - 1)
		return(-1);

	/* the article and the stored NUL after it, by length */
	n = pread(fd, buf + sizeof(SpoolArtHdr), (size_t)alen + 1,
						(off_t)offset + skip);
	if (n != (ssize_t)alen + 1) {
#ifdef	DEBUG
		fprintf(stderr, "error: reading article data, %d read, %d wanted, %d\n", (int)n, alen + 1, errno);
#endif
		return(-1);
	}
	if (buf[sizeof(SpoolArtHdr) + alen] != '\0') {
#ifdef	DEBUG
		fprintf(stderr, "error: checking article data, last byte not null\n");
#endif
		return(-1);
	}
	return(alen);
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
	write(fileno(stdout), abuf + sizeof(SpoolArtHdr), rval);
	exit(0);
}
#endif

