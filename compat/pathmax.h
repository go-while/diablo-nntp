#ifndef DIABLO_PATHMAX_H
#define DIABLO_PATHMAX_H

#include <limits.h>

/* Some systems (notably GNU/Hurd) have no filesystem-wide path limit.
 * This is an application buffer bound, not a promise about accepted paths.
 * Keep the system's value where one exists; never use this to size a buffer
 * for an API such as realpath() that has no explicit size argument.
 */
#ifndef PATH_MAX
#define PATH_MAX 4096
#endif

#endif /* DIABLO_PATHMAX_H */
