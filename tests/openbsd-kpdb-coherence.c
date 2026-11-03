/* Persistent KPDB reader versus another process's updates. */
#include "lib/defs.h"
#include <sys/wait.h>

static void
check(int ok, const char *message)
{
    if (!ok) {
        fprintf(stderr, "FAIL: %s (%s)\n", message, strerror(errno));
        exit(1);
    }
}

static int
value_is(KPDB *db, const char *key, const char *value)
{
    int len = 0;
    const char *p = KPDBRead(db, key, "v", 0, &len, NULL);
    return(p != NULL && len == (int)strlen(value) &&
        memcmp(p, value, (size_t)len) == 0);
}

int
main(int argc, char **argv)
{
    KPDB *db;
    int status;
    pid_t child;

    if (argc != 2) {
        fprintf(stderr, "usage: %s new-kpdb-path\n", argv[0]);
        return(2);
    }
    db = KPDBOpen(argv[1], O_RDWR | O_CREAT | O_EXCL);
    check(db != NULL, "open");
    check(KPDBWrite(db, "same", "v", "old", 0) == 0, "initial same");
    check(KPDBWrite(db, "deleted", "v", "yes", 0) == 0, "initial deleted");
    check(value_is(db, "deleted", "yes"), "initial deleted lookup");
    /* Leave this exact record cached across the child's in-place update. */
    check(value_is(db, "same", "old"), "initial lookup");
    child = fork();
    check(child >= 0, "fork");
    if (child == 0) {
        /* Reopen drops O_EXCL on the inherited handle. */
        KPDBClose(db);
        db = KPDBOpen(argv[1], O_RDWR);
        check(db != NULL, "child open");
        check(KPDBWrite(db, "same", "v", "new", 0) == 0, "child replace");
        check(KPDBWrite(db, "added", "v", "created", 0) == 0, "child append");
        (void)KPDBDelete(db, "deleted");
        KPDBReSort(db);
        KPDBClose(db);
        _exit(0);
    }
    check(waitpid(child, &status, 0) == child && WIFEXITED(status) &&
        WEXITSTATUS(status) == 0, "child status");
    check(value_is(db, "same", "new"), "persistent reader sees in-place update");
    check(value_is(db, "added", "created"), "persistent reader sees appended record");
    check(KPDBRead(db, "deleted", "v", 0, NULL, NULL) == NULL,
        "persistent reader sees deletion");
    KPDBClose(db);
    puts("PASS: persistent KPDB reader sees replacement, append, deletion and sort");
    return(0);
}
