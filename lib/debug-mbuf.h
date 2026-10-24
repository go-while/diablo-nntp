#ifndef LIB_DEBUG_MBUF_H
#define LIB_DEBUG_MBUF_H

#if defined(DEBUG_MBUF) && !defined(DEBUG_MBUF_OFF)

extern char *__dbg_file;
extern char *__dbg_func;
extern int __dbg_line;

#  ifndef __dbg
#    define __dbg(func, args...) ({ \
                __dbg_line = __LINE__; \
                __dbg_file = __FILE__; \
                __dbg_func = #func; \
                func(args); \
          })
#  endif

#define MBFlush(args...)	__dbg(MBFlush, args)
#define MBFree(args...)		__dbg(MBFree, args)
#define MBPoll(args...)		__dbg(MBPoll, args)
#define MBInit(args...)		__dbg(MBInit, args)
#define MBWrite(args...)	__dbg(MBWrite, args)
#define MZInit(args...)		__dbg(MZInit, args)
#define MZWrite(args...)	__dbg(MZWrite, args)
#define MZPrintf(args...)	__dbg(MZPrintf, args)
#define MZFinish(args...)	__dbg(MZFinish, args)
#define MBWriteDecode(args...)	__dbg(MBWriteDecode, args)
#define MBCopy(args...)		__dbg(MBCopy, args)
#define MBPrintf(args...)	__dbg(MBPrintf, args)
#define MBLogPrintf(args...)	__dbg(MBLogPrintf, args)
#define MBRead(args...)		__dbg(MBRead, args)
#define MBReadLine(args...)	__dbg(MBReadLine, args)
#define MBNormalize(args...)	__dbg(MBNormalize, args)

#endif

#endif
