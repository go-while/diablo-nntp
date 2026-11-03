/* Standalone OpenBSD regression; link against the built obj/libdiablo.a. */
#include "lib/defs.h"

static void
check(int ok, const char *message)
{
    if (!ok) {
        fprintf(stderr, "FAIL: %s: %s\n", message, strerror(errno));
        exit(1);
    }
}

int
main(void)
{
    char path[] = "/tmp/diablo-xreadmap-XXXXXXXX";
    char *before, *after, *tail;
    int fd = mkstemp(path);
    size_t page = (size_t)getpagesize();

    check(fd >= 0, "mkstemp");
    check(unlink(path) == 0, "unlink");
    check(pwrite(fd, "abc", 3, 0) == 3, "initialize");
    before = xreadmap(fd, 4, 0);
    check(before != NULL && before[0] == 'a' && before[3] == 0,
        "initial snapshot and partial-page zero");
    check(pwrite(fd, "z", 1, 0) == 1, "change file");
    after = xreadmap(fd, 4, 0);
    check(after != NULL && after[0] == 'z' && before[0] == 'a',
        "new snapshot sees write without changing old snapshot");
    tail = xreadmap(fd, 2, 2);
    check(tail != NULL && tail[0] == 'c' && tail[1] == 0,
        "unaligned offset and zero padding");
    check(xreadmap(fd, page + 1, 0) == NULL,
        "reject access to page beyond EOF");
    xunmap(before, 4);
    xunmap(after, 4);
    xunmap(tail, 2);
    close(fd);
    puts("PASS: xreadmap snapshots, EOF guard, and offsets");
    return(0);
}
