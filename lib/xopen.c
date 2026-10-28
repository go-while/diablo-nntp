
/*
 * LIB/XOPEN.C
 *
 * (c)Copyright 1997, Matthew Dillon, All Rights Reserved.  Refer to
 *    the COPYRIGHT file in the base directory of this distribution 
 *    for specific rights granted.
 *
 */

#include "defs.h"

Prototype int xopen(int modes, int perms, const char *ctl, ...);
Prototype FILE *xfopen(const char *modes, const char *ctl, ...);
Prototype FILE *PrivateFOpen(const char *path, const char *modes, int perms);
Prototype int xfprintf(FILE *fo, const char *ctl, ...);
Prototype void ddprintf(const char *ctl, ...);
Prototype void vddprintf(const char *ctl, va_list va);
Prototype FILE *fcdopen(const char *path, const char *modes);
Prototype int cdopen(const char *path, int modes, int perms);
Prototype const char *cdcache(const char *path);
Prototype void cdreset(void);

Prototype int DDUseSyslog;

int DDUseSyslog;

int
xopen(int modes, int perms, const char *ctl, ...)
{
    char path[1024];
    va_list va;

    va_start(va, ctl);
    vsnprintf(path, sizeof(path), ctl, va);
    va_end(va);
    return(open(path, modes, perms));
}


FILE *
xfopen(const char *modes, const char *ctl, ...)
{
    char path[1024];
    va_list va;

    va_start(va, ctl);
    vsnprintf(path, sizeof(path), ctl, va);
    va_end(va);
    return(fopen(path, modes));
}

/*
 * PrivateFOpen() - fopen() for a file that is created here: modes is "a" or
 * "w+" and perms the mode of a file that doesn't exist yet, whatever the
 * umask (an existing file keeps its mode, which the admin may have set).
 * For "w+" the file is first removed, so that a file left from an earlier
 * run, or a link in its place, can't keep another mode or reach elsewhere.
 */
FILE *
PrivateFOpen(const char *path, const char *modes, int perms)
{
    int fd;
    FILE *fp;

    if (modes[0] == 'w') {
	unlink(path);
	fd = open(path, O_RDWR|O_CREAT|O_EXCL, perms);
    } else {
	fd = open(path, O_WRONLY|O_APPEND|O_CREAT, perms);
    }
    if (fd < 0)
	return(NULL);
    if ((fp = fdopen(fd, modes)) == NULL) {
	close(fd);
	return(NULL);
    }
    return(fp);
}

int
xfprintf(FILE *fo, const char *ctl, ...)
{
    va_list va;
    int r;

    if (fo == NULL) {
	logit(LOG_CRIT, "xfprintf() - fo NULL!");
	exit(1);
    }

    va_start(va, ctl);
    r = vfprintf(fo, ctl, va);
    va_end(va);
    if (DebugOpt > 2) {
	va_start(va, ctl);
	vddprintf(ctl, va);
	va_end(va);
    }
    return(r);
}

void 
ddprintf(const char *ctl, ...)
{
    va_list va;

    if (DebugOpt) {
	va_start(va, ctl);
	vddprintf(ctl, va);
	va_end(va);
    }
}

void 
vddprintf(const char *ctl, va_list va)
{
    if (DebugOpt) {
	if (DDUseSyslog) {
	    vsyslog(LOG_DEBUG, ctl, va);
	} else {
	    vprintf(ctl, va);
	    printf("\n");
	}
    }
}

FILE *
fcdopen(const char *path, const char *modes)
{
    const char *p = cdcache(path);
    FILE *fi;

    fi = fopen(p, modes);
    if (DebugOpt && strstr(path, "/feeds/") == NULL) {
        printf("%*.*s^%s\t%s\n", (int)(p - path), (int)(p - path), path, p,
            (fi == NULL) ? "open failure" : "open ok"
        );
    }
    return(fi);
}

int
cdopen(const char *path, int modes, int perms)
{
    const char *p = cdcache(path);
    int fd;

    fd = open(p, modes, perms);
    if (DebugOpt) {
        printf("%*.*s^%s\t%s\n", (int)(p - path), (int)(p - path), path, p,
            (fd < 0) ? "open failure" : "open ok"
        );
    }
    return(fd);
}

static char DirCache[256];

/*
 * cdreset() - forget the directory cdcache() changed into, so the next call
 * changes into it again: needed when it was removed (and maybe recreated)
 * behind our back, e.g. a spool directory deleted by dexpire.
 */
void
cdreset(void)
{
    DirCache[0] = 0;
}

const char *
cdcache(const char *path)
{
    const char *p;

    if ((p = strrchr(path, '/')) == NULL)
	return(path);

    ++p;

    if (strncmp(DirCache, path, p - path) != 0 || DirCache[p - path] != 0) {
        bcopy(path, DirCache, p - path);
        DirCache[p - path] = 0;
        if (chdir(DirCache) != 0) {
#ifdef NOTDEF
	    /*
	     * this is actually ok for diablo, because it tries to write the
	     * file before creating the directory.
	     */
	    logit(LOG_CRIT, "fcdopen() - unable to chdir(\"%s\")\n", DirCache);
#endif
            DirCache[0] = 0;
            p = path;
        }
    }
    return(p);
}


