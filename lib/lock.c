
/*
 * LIB/LOCK.C
 *
 * (c)Copyright 1997, Matthew Dillon, All Rights Reserved.  Refer to
 *    the COPYRIGHT file in the base directory of this distribution 
 *    for specific rights granted.
 */

#include "defs.h"

Prototype int xflock(int fd, int flags);
Prototype int hflock(int fd, off_t offset, int flags);
Prototype int rflock(int fd, off_t offset, off_t len, int flags);

/*
 * setlock() - F_SETLK, or F_SETLKW without XLOCK_NB.  A blocking wait that
 * a signal interrupts is started again, unless the caller passed
 * XLOCK_INTR to end the wait with a signal (an alarm() timeout).
 */
static int
setlock(int fd, struct flock *fl, int flags)
{
    int r;

    if (flags & XLOCK_NB)
	return(fcntl(fd, F_SETLK, fl));
    do {
	r = fcntl(fd, F_SETLKW, fl);
    } while (r < 0 && errno == EINTR && (flags & XLOCK_INTR) == 0);
    return(r);
}

/*
 * xflock() - a global lock.  
 *
 * NOTE: cannot be used in conjuction with hflock() due to odd overlap
 * interaction.  If we create other locks, they may wind up sticking
 * around while the unlocks take holes out of the original.
 */

int 
xflock(int fd, int flags)
{
    int r;
    struct flock fl = { 0 };

    fl.l_whence = SEEK_SET;

    switch(flags & 0x0F) {
    case XLOCK_SH:
	fl.l_type = F_RDLCK;
	break;
    case XLOCK_EX:
	fl.l_type = F_WRLCK;
	break;
    case XLOCK_UN:
	fl.l_type = F_UNLCK;
	break;
    }
    r = setlock(fd, &fl, flags);
    return(r);
}

int 
hflock(int fd, off_t offset, int flags)
{
    int r;
    struct flock fl = { 0 };

    switch(flags & 0x0F) {
    case XLOCK_SH:
	fl.l_type = F_RDLCK;
	break;
    case XLOCK_EX:
	fl.l_type = F_WRLCK;
	break;
    case XLOCK_UN:
	fl.l_type = F_UNLCK;
	break;
    }
    fl.l_whence = SEEK_SET;
    fl.l_start = offset;
    fl.l_len = 4;

    r = setlock(fd, &fl, flags);
    if (DebugOpt > 4) {
	int error = errno;	/* callers report errno */

	printf("hflock fd %d offset %d flags %02x\n", fd, (int)offset, flags);
	errno = error;
    }
    return(r);
}

/*
 * rflock() - lock len bytes at offset (hflock() locks 4).
 */
int
rflock(int fd, off_t offset, off_t len, int flags)
{
    struct flock fl = { 0 };

    switch(flags & 0x0F) {
    case XLOCK_SH:
	fl.l_type = F_RDLCK;
	break;
    case XLOCK_EX:
	fl.l_type = F_WRLCK;
	break;
    case XLOCK_UN:
	fl.l_type = F_UNLCK;
	break;
    }
    fl.l_whence = SEEK_SET;
    fl.l_start = offset;
    fl.l_len = len;
    return(setlock(fd, &fl, flags));
}
