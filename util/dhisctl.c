
/*
 * DHISCTL.C - Perform history maintenance operations
 *
 * (c)Copyright 2002, Russell Vincent, All Rights Reserved.  Refer to
 *    the COPYRIGHT file in the base directory of this distribution 
 *    for specific rights granted.
 */

#include "defs.h"

int VerboseOpt = 0;
int ShowProgress = 0;
char *MsgID = NULL;
int ExpireArt = 0;
int UnExpireArt = 0;
char *FileName = NULL;
char *MsgIdList = NULL;
int HistoryFd = -1;
int ForReal = 1;
int StructSizes = 0;
int HistoryHead = 0;

void DumpHeader(int fd);
void DoEntry(char *msgid);
void ScanFile(char *fname);

void
Usage(void)
{
    printf("Perform maintenance operations on the history file.\n\n");
    printf("dhisctl [-e] [-f id_file] [-p] [-S] [-v]\n");
    printf("           [-C diablo.config] [-d[n]] [-V] historyfile [<MsgId>|hash]\n");
    printf("  where:\n");
#if 0
    printf("\t-a\t- perform the action on all history entries\n");
#endif
    printf("\t-e\t- expire the article(s)\n");
    printf("\t-f\t- file containing list of msgid's or '-' for stdin\n");
    printf("\t-h\t- display history header and total size details\n");
    printf("\t-p\t- show progress on stdout\n");
    printf("\t-S\t- display sizes of internal history structures and exit\n");
    printf("\t-u\t- unexpire the article(s)\n");
    printf("\t-v\t- be a little more verbose\n");
    printf("\t-Cfile\t- specify diablo.config to use\n");
    printf("\t-d[n]\t- set debug [with optional level]\n");
    printf("\t-V\t- print version and exit\n");
    exit(1);
}

int
main(int ac, char **av)
{
    int i;

    LoadDiabloConfig(ac, av);

    for (i = 1; i < ac; ++i) {
	char *ptr = av[i];

	if (*ptr != '-') {
	    if (FileName != NULL) {
		if (MsgID != NULL) {
		    fprintf(stderr, "unexpected argument: %s\n", ptr);
		    exit(1);
		}
		MsgID = ptr;
		continue;
	    }
	    FileName = ptr;
	    continue;
	}
	ptr += 2;
	switch(ptr[-1]) {
	case 'a':
	    /* This formerly invoked an incomplete scanner which used an
	     * invalid file descriptor and misinterpreted history records.
	     * Refuse it before opening or modifying the history file.
	     */
	    fprintf(stderr, "dhisctl: -a bulk history scan is unsupported\n");
	    exit(1);
	case 'e':
	    ExpireArt = 1;
	    break;
	case 'f':
	    if (!*ptr)
		ptr = av[++i];
	    MsgIdList = ptr;
	    break;
	case 'h':
	    HistoryHead = 1;
	    break;
	case 'p':
	    ShowProgress = 1;
	    break;
	case 'S':
	    StructSizes = 1;
	    break;
	case 'u':
	    UnExpireArt = 1;
	    break;
	case 'v':
	    if (*ptr)
		VerboseOpt = strtol(ptr, NULL, 0);
	    else
		++VerboseOpt;
	    break;
	/* Common options */
	case 'C':		/* parsed by LoadDiabloConfig */
	    if (*ptr == 0)
		++i;
	    break;
	case 'd':
	    DebugOpt = 1;
	    if (*ptr)
		DebugOpt = strtol(ptr, NULL, 0);
	    break;
	case 'V':
	    PrintVersion();
	    break;
	default:
	    fprintf(stderr, "illegal option: %s\n", ptr - 2);
	    Usage();
	}
    }

    if (StructSizes) {
	printf("History Header           : %2d bytes\n", (int)sizeof(HistHead));
	printf("History Hash Entry       : %2d bytes\n", (int)sizeof(uint32));
	printf("History Entry            : %2d bytes\n", (int)sizeof(History));
	printf("Hash Table Entries       : %d\n", DOpts.HashSize);
	printf("Expected hash table size : %d bytes\n", DOpts.HashSize * (int)sizeof(uint32));
	exit(0);
    }

    if (FileName == NULL)
	Usage();

    HistoryOpen(FileName, 0);
    if ((HistoryFd = open(FileName, O_RDWR)) == -1) {
	perror("history open");
	exit(1);
    }

    if (HistoryHead)
	DumpHeader(HistoryFd);
    if (MsgID != NULL)
	DoEntry(MsgID);
    if (MsgIdList != NULL)
	ScanFile(MsgIdList);

    return(0);
}

void
DumpHeader(int fd)
{
    HistHead hh;

    if (read(fd, &hh, sizeof(hh)) != sizeof(hh)) {
	perror("ERROR in history header read");
	exit(1);
    }
    printf("Magic     : 0x%x\n", hh.hmagic);
    printf("HashSize  : %d\n", hh.hashSize);
    printf("Version   : %d\n", hh.version);
    printf("EntrySize : %d\n", hh.henSize);
    printf("HeaderSize: %d\n\n", hh.headSize);
    {
	struct stat sb;
	if (fstat(fd, &sb) == -1)
	    exit(0);
	printf("Total Size      : %.0f Bytes (%s)\n", (double)sb.st_size,
						ftos((double)sb.st_size));
	printf("Hash Table Size : %d Bytes (%s)\n",
				hh.hashSize * (int)sizeof(HistIndex),
				ftos((double)hh.hashSize * sizeof(HistIndex)));
	printf("Total Entries   : %.0f\n",
	    ((double)sb.st_size - (double)hh.headSize -
				(double)hh.hashSize * sizeof(HistIndex)) /
								hh.henSize);
    }
    exit(0);
}

int
FixEntry(History *h, char *msgid)
{
    int mod = 0;

    if (UnExpireArt) {
	if (H_EXPIRED(h->exp)) {
	    h->exp &= ~EXPF_EXPIRED;
	    mod = 1;
	    if (VerboseOpt)
		printf("UnExpiring: %s\n", msgid);
	} else if (VerboseOpt)
	    printf("Not expired: %s\n", msgid);
    } else if (ExpireArt) {
	if (!H_EXPIRED(h->exp)) {
	    h->exp |= EXPF_EXPIRED;
	    mod = 1;
	    if (VerboseOpt)
		printf("Expiring: %s\n", msgid);
	} else if (VerboseOpt)
	    printf("Already expired: %s\n", msgid);
    }
    return(mod);
}

/*
 * XXX This could be handled better by doing a HistoryExpire()
 * XXX but we also need the capability to modify other history fields
 * XXX FUTURE WORK: Check for (un)expire and run HistoryExpire()
 */
void
DoEntry(char *msgid)
{
    History h;
    hash_t hv;
    int32 pos;
    char *p;

    if (msgid[0] == '<' && (p = strchr(msgid, '>')) != NULL)
        hv = hhash(msgid);
    else if (sscanf(msgid, "%x.%x", &hv.h1, &hv.h2) != 2) {
	fprintf(stderr, "Invalid msgid/hash: %s\n", msgid);
	exit(1);
    }
    pos = HistoryPosLookupByHash(hv, &h);
    if (pos == -1) {
	fprintf(stderr, "History lookup failed for %s\n", msgid);
	return;
    }
    if (FixEntry(&h, msgid) && ForReal)
	HistoryStoreExp(&h, (HistIndex)pos);
}

void
ScanFile(char *fname)
{
    FILE *f;
    char buf[1024];

    if (strcmp(fname, "-") == 0)
	f = stdin;
    else
	f = fopen(fname, "r");
    if (f == NULL) {
	fprintf(stderr, "Unable to open msgid file: %s\n", fname);
	exit(1);
    }
    while (fgets(buf, sizeof(buf), f) != NULL) {
	buf[strlen(buf) - 1] = '\0';
	DoEntry(buf);
    }
    fclose(f);
}

