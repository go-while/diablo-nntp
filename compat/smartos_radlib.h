#ifndef DIABLO_SMARTOS_RADLIB_H
#define DIABLO_SMARTOS_RADLIB_H

#include <stdint.h>

/* BSD compatibility macros required by radlib.h */
#ifndef __BEGIN_DECLS
#define __BEGIN_DECLS
#endif

#ifndef __END_DECLS
#define __END_DECLS
#endif

/* BSD integer type compatibility */
#define u_int32_t uint32_t

#endif /* DIABLO_SMARTOS_RADLIB_H */
