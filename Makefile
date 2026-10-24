# Diablo 6 portable GNU Make build system
# Platforms: Linux, FreeBSD, SmartOS / illumos
#
# Linux:   make -j4
# FreeBSD: gmake -j4
# SmartOS: gmake -j4
#
# Native FreeBSD make uses the separate BSDmakefile.

.DEFAULT_GOAL := all

UNAME_S := $(shell uname -s)

# Build tools
ifeq ($(UNAME_S),FreeBSD)
CC = clang
else
CC = gcc
endif

AR ?= ar
RANLIB ?= ranlib

# Compiler settings
CPPFLAGS += -I. -DVERS=\"6\" -DSUBREV=\"CUR-20170901-00\"
CPPFLAGS += -DDEBUG_ZALLOC -DDEBUG_MBUF

CFLAGS ?= -O2 -g -Wall -Wstrict-prototypes -fcommon
CFLAGS += -MMD -MP

LDFLAGS += -Lobj
LDLIBS += -lfilter -ldiablo -lm -lz -lcrypto

# Platform-specific settings

ifeq ($(UNAME_S),FreeBSD)
LDLIBS += -lmd -lradius
endif

ifeq ($(UNAME_S),Linux)
CPPFLAGS += -D_FILE_OFFSET_BITS=64
endif

ifneq ($(filter SunOS illumos,$(UNAME_S)),)
CPPFLAGS += -D_LARGEFILE_SOURCE -D_FILE_OFFSET_BITS=64
CPPFLAGS += -D_XOPEN_SOURCE=600 -D__EXTENSIONS__
CPPFLAGS += -I/opt/local/include/db4

LDFLAGS += -L/opt/local/lib
LDLIBS += -lnsl -lsocket -lposix4 -ldb4 -lradius -lmd

# Compatibility implementation required by pkgsrc libradius.a
SMARTOS_COMPAT_OBJS = obj/compat_srandomdev.o
endif

# Source files

LIB_SRCS = \
    lib/global.c lib/node.c lib/xopen.c lib/buffer.c \
    lib/wildcmp.c lib/history.c lib/expire.c lib/newsfeed.c \
    lib/parsedate.c lib/sigs.c lib/lock.c lib/alloc.c \
    lib/subs.c lib/xmap.c lib/precommit.c lib/spamfilter.c \
    lib/strerror.c lib/memcpy.c lib/zalloc.c lib/config.c \
    lib/kpdb.c lib/active.c lib/msgid.c lib/hash.c \
    lib/psstat.c lib/runprog.c lib/snprintf.c lib/fatal.c \
    lib/log.c lib/logtime.c lib/iplist.c lib/dgp.c \
    lib/pgp.c lib/hostauth.c lib/strsep.c lib/groupfind.c \
    lib/spool.c lib/arttype.c lib/stats.c lib/dmd5.c \
    lib/notify.c lib/wildmat.c lib/include.c lib/hashfeed.c \
    lib/dgzip.c

FILTER_SRCS = \
    filter/diab-filter.c filter/diab-read.c filter/socket.c

DR_SRCS = \
    dreaderd/main.c dreaderd/thread.c dreaderd/reader.c \
    dreaderd/dns.c dreaderd/mbuf.c dreaderd/subs.c \
    dreaderd/list.c dreaderd/feed.c dreaderd/xover.c \
    dreaderd/nntp.c dreaderd/misc.c dreaderd/post.c \
    dreaderd/server.c dreaderd/group.c dreaderd/spool.c \
    dreaderd/cache.c dreaderd/rtstatus.c dreaderd/control.c \
    dreaderd/wildorcmp.c dreaderd/cancel.c \
    dreaderd/post-addr-ck.c dreaderd/cleanfrom.c \
    dreaderd/msg.c dreaderd/dfa.c dreaderd/filealloc.c \
    dreaderd/crypto.c

UTIL_PROGS = \
    dicmd drcmd didump dilookup dexpire didate \
    diconvhist diload doutq dspaminfo dspoolout \
    diloadfromspool dreadart dkp pgpverify dsyncgroups \
    dexpireover dreadover dpath dprimehostcache dstart \
    dclient dfeedinfo dlockhistory dfeedtest doverctl \
    drequeue dhisbench dhisexpire dhisctl dexpirecache \
    dcancel dexpirescoring diablo dnewslink dgrpctl

SUP_PROGS = plock showlocks

# Object file lists

LIB_OBJS := $(patsubst lib/%.c,obj/lib_%.o,$(LIB_SRCS))
FILTER_OBJS := $(patsubst filter/%.c,obj/filter_%.o,$(FILTER_SRCS))
DR_OBJS := $(patsubst dreaderd/%.c,obj/dr_%.o,$(DR_SRCS))

DR_LIB_OBJS := $(filter-out obj/dr_main.o,$(DR_OBJS))

UTIL_OBJS := $(addprefix obj/util_,$(addsuffix .o,$(UTIL_PROGS)))
SUP_OBJS := $(addprefix obj/sup_,$(addsuffix .o,$(SUP_PROGS)))

PROTOS := obj/lib-protos.h obj/filter-protos.h obj/dreaderd-protos.h

BINS := dbin/dreaderd $(addprefix dbin/,$(UTIL_PROGS) $(SUP_PROGS))

# Main targets

.PHONY: all clean distclean install bininstall uninstall help

all: $(BINS) obj/libdreader.a

# Output directories
# These are order-only prerequisites so directory timestamp
# changes do not trigger unnecessary rebuilds.

obj dbin:
	mkdir -p $@

# Generated prototype headers

obj/lib-protos.h: $(LIB_SRCS) | obj
	@grep -h 'Prototype' $(LIB_SRCS) > $@.tmp || test $$? -eq 1
	@mv $@.tmp $@

obj/filter-protos.h: $(FILTER_SRCS) | obj
	@grep -h 'Prototype' $(FILTER_SRCS) > $@.tmp || test $$? -eq 1
	@mv $@.tmp $@

obj/dreaderd-protos.h: $(DR_SRCS) | obj
	@grep -h 'Prototype' $(DR_SRCS) > $@.tmp || test $$? -eq 1
	@mv $@.tmp $@

# Compilation rules

obj/lib_%.o: lib/%.c obj/lib-protos.h | obj
	$(CC) $(CPPFLAGS) $(CFLAGS) -c $< -o $@

obj/filter_%.o: filter/%.c obj/filter-protos.h obj/lib-protos.h | obj
	$(CC) $(CPPFLAGS) $(CFLAGS) -c $< -o $@

obj/dr_%.o: dreaderd/%.c $(PROTOS) | obj
	$(CC) $(CPPFLAGS) $(CFLAGS) -c $< -o $@

obj/util_%.o: util/%.c $(PROTOS) | obj
	$(CC) $(CPPFLAGS) $(CFLAGS) -c $< -o $@

obj/sup_%.o: sup/%.c $(PROTOS) | obj
	$(CC) $(CPPFLAGS) $(CFLAGS) -c $< -o $@

# SmartOS compatibility object

obj/compat_srandomdev.o: compat/srandomdev.c | obj
	$(CC) $(CPPFLAGS) $(CFLAGS) -c $< -o $@

# Static libraries

obj/libdiablo.a: $(LIB_OBJS)
	$(AR) rcs $@ $^
	$(RANLIB) $@

obj/libfilter.a: $(FILTER_OBJS)
	$(AR) rcs $@ $^
	$(RANLIB) $@

obj/libdreader.a: $(DR_LIB_OBJS)
	$(AR) rcs $@ $^
	$(RANLIB) $@

# Reader daemon

dbin/dreaderd: $(DR_OBJS) $(SMARTOS_COMPAT_OBJS) obj/libfilter.a obj/libdiablo.a | dbin
	$(CC) $(LDFLAGS) $(DR_OBJS) $(SMARTOS_COMPAT_OBJS) -o $@ $(LDLIBS)

# Utility executables

define util_rule
dbin/$(1): obj/util_$(1).o obj/libfilter.a obj/libdiablo.a | dbin
	$$(CC) $$(LDFLAGS) obj/util_$(1).o -o $$@ $$(LDLIBS)
endef

$(foreach prog,$(UTIL_PROGS),$(eval $(call util_rule,$(prog))))

# Support executables

define sup_rule
dbin/$(1): obj/sup_$(1).o obj/libfilter.a obj/libdiablo.a | dbin
	$$(CC) $$(LDFLAGS) obj/sup_$(1).o -o $$@ $$(LDLIBS)
endef

$(foreach prog,$(SUP_PROGS),$(eval $(call sup_rule,$(prog))))

# Maintenance

clean:
	rm -rf obj dbin

distclean: clean

install bininstall uninstall:
	@echo 'Installation is not yet ported; consult XMakefile.inst / XMakefile.uninst.' >&2; exit 1

help:
	@echo 'Targets: all clean distclean help; installation requires original xmake rules.'

# Automatically generated compiler dependencies
-include $(wildcard obj/*.d)
