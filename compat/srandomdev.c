/*
 * SmartOS compatibility implementation of srandomdev().
 *
 * Required by pkgsrc libradius.a, which takes the Request Authenticator
 * and the request identifier from random().  Like FreeBSD's srandomdev(),
 * this fills the whole random() state from the kernel's generator, not
 * just a 32-bit seed: initstate() sets up a 256-byte state (and its
 * header word), then everything after the header is replaced with
 * arc4random() bytes and the state is made current again.
 */

#include <stdlib.h>
#include <stdint.h>

void
srandomdev(void)
{
    static long state[256 / sizeof(long)];
    const size_t hdr = sizeof(long);	/* the type/position word(s) at the start */

    (void)initstate(arc4random(), (char *)state, sizeof(state));
    arc4random_buf((char *)state + hdr, sizeof(state) - hdr);
    (void)setstate((char *)state);
}
