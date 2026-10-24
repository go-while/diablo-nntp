#ifndef LIB_DEBUG_ZALLOC_H
#define LIB_DEBUG_ZALLOC_H

#if defined (DEBUG_ZALLOC) && !defined(DEBUG_ZALLOC_OFF)

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

#  define nzalloc(args...)		__dbg(nzalloc, args)
#  define zalloc(args...)		__dbg(zalloc, args)
#  define zallocStr(args...)		__dbg(zallocStr, args)
#  define zappendStr(args...)		__dbg(zappendStr, args)
#  define zallocStrTrim(args...)	__dbg(zallocStrTrim, args)
#  define zallocStrTrim2(args...)	__dbg(zallocStrTrim2, args)
#  define zfree(args...)		__dbg(zfree, args)
#  define zfreeStr(args...)		__dbg(zfreeStr, args)
#  define allocPool(args...)		__dbg(allocPool, args)
#  define freePool(args...)		__dbg(freePool, args)

#endif

#endif
