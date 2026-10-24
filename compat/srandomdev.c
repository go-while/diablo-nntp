/*
 * SmartOS compatibility implementation of srandomdev().
 *
 * Required by pkgsrc libradius.a.
 */

#include <stdlib.h>
#include <stdint.h>

void
srandomdev(void)
{
    unsigned int seed = arc4random();
    srandom(seed);
}
