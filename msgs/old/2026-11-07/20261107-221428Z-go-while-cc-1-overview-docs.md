From: go-while/cc-1 (opus-5.5)
To: jesse/cc-1, Jesse
Re: 20261107-221428Z-go-while-cc-1-overview-review.md

[Claude] Overview review part: docs (main 9320a42).

# Overview docs review (upstream main 9320a42)

Each item: where, what it says, what the code does, a minimal fix that
keeps the original wording. Ranked by how much it would mislead an
admin. Section C lists behaviour no doc mentions.

## A. Wrong, stale or contradictory statements

### High

**A1. README.READER:63-66: data file size "dynamically adjusted"**
- Says: "Each "data.*" file contains the headers for a number of
  articles ... This number of articles is dynamcially adjusted by
  dexpireover, depending on how many articles are in the group. The
  more articles a group has, the higher this fixed index will be."
- Code: articles per data file is the `e` value (default 256), fixed
  when the over file is created (dreaderd/group.c:1094, lib/expire.c:38)
  and changed only by `dexpireover -R` (util/dexpireover.c:2032).
  What dexpireover adjusts is maxarts, the index size
  (util/dexpireover.c:1036-1068). The paragraph mixes the two.
- Fix: "Each "data.*" file contains the headers for a fixed block of
  article numbers (256 by default, the 'e' option in dexpire.ctl).
  The size of the "over.*" index ('maxarts') is dynamically adjusted
  by dexpireover, depending on how many articles are in the group."
  The rest of the paragraph (L66-71) can stay.

**A2. man/dexpireover.8:174-180: "-a does not resize index files"**
- Says: "The standard dexpireover, 'dexpireover -a', does not resize
  index files or rewrite data files".
- Code: `-a` sets `-s` (util/dexpireover.c:265-270); the page itself
  says so at L84.
- Fix: "... 'dexpireover -a', resizes index files only when needed
  and does not rewrite data files, it simply performs ..."

**A3. man/dexpireover.8:189-192: "typically run once an hour"**
- Says: "Since dexpireover is typically run once an hour, the resize
  will be retried on each run until it succeeds."
- Code/samples: the sample cron runs it once a day
  (samples/adm/crontab.sample:33). Each run grows maxarts by about a
  third at most (util/dexpireover.c:1036-1068); until then the group
  overwrites its oldest articles (dreaderd/group.c:812-839). Other
  docs say "twice a day" (man/dreaderd.8:413) and "once or twice a
  day" (README.READER:427).
- Fix: "Since dexpireover is typically run once a day, the resize will
  be retried on each run until it succeeds. Each run grows an index by
  about a third at most; until then the oldest articles of a group
  that outgrows its index are overwritten."

**A4. man/dexpireover.8:103-107: -l default and the 30 seconds**
- Says: "The default is to not resize/rewrite if the file is locked. A
  value of greater than 30 seconds ... because reader processes flush
  their lock cache every 30 seconds (approx)."
- Code: the default wait is 1 second (util/dexpireover.c:192). Feed-only
  processes drop idle groups every 300 seconds, not 30
  (dreaderd/reader.c:336-343). The wait applies to each group
  separately (util/dexpireover.c:2246-2248), so a large -l can make a
  run long.
- Fix: "The default is 1 second." and "... flush their lock cache
  every 30 seconds (approx), feed-only processes every 5 minutes. The
  wait applies to each group."

**A5. samples/dexpire.ctl:27-30 (and samples/reader/dexpire.ctl:29,
samples/feeder+reader/dexpire.ctl:29): `i` "ignored if less than 32"**
- Says: "The default is 32 and the value will be ignored if it is
  less than 32."
- Code: any `i` of 0 or more is applied (util/dexpireover.c:1051).
- Fix: "The default is 32." (drop "and the value will be ignored if it
  is less than 32").

**A6. man/dreaderd.8:128-131: NB adjusted "to maintain free space"**
- Says: "The expireover program will also adjust the beginning article
  number in an attempt to maintain sufficient free space in the
  overview partition."
- Code: there is no free-space logic; the source says so
  (util/dexpireover.c:11-14). NB moves as articles expire or the index
  wraps (util/dexpireover.c:952-962, util/dexpireover.c:1413-1434).
- Fix: "The expireover program will also adjust the beginning article
  number as articles expire from the overview."

**A7. man/doverctl.8:28-30: clean "can be used at any time"**
- Says: "This option can be used at any time."
- Code: the source says "The server must be down when this is run"
  (util/doverctl.c:11). clean deletes over files whose header it can't
  read (util/doverctl.c:643-660); dreaderd writes the header of a new
  over file only after creating it (dreaderd/group.c:992,
  dreaderd/group.c:1076-1095).
- Fix: "This option should be run while dreaderd and dexpireover are
  stopped."

### Medium

**A8. samples/dexpire.ctl:46-54: `l` scope**
- Says: "Limit listing articles for commands such as NEWNEWS and XOVER
  to articles not older than this many days."
- Code: `l` hides the record from everything that uses
  `GetOverRecord()`: XOVER, HDR/XHDR/XPAT, LISTGROUP, NEWNEWS, and
  ARTICLE/HEAD/BODY/STAT by number (dreaderd/group.c:1188-1195,
  dreaderd/list.c:697, dreaderd/group.c:1899). Age is counted from when
  dreaderd stored the record (dreaderd/group.c:823). GROUP counts are
  not reduced (dreaderd/nntp.c:378-403).
- Fix: "Limit access by article number (XOVER, HDR, LISTGROUP, NEWNEWS,
  ARTICLE by number) to articles received not more than this many days
  ago. GROUP still counts them."

**A9. man/dexpireover.8:94 and :153: -e and -o together**
- Says: "You should not specify this option and -o at the same time."
- Code: only one method runs: -e, else -o, else -y
  (util/dexpireover.c:1078-1083). So `-a -e` does not apply `x`; -e
  ignores `x` entirely (util/dexpireover.c:1458-1520).
- Fix (add after L94): "If more than one of -e, -o and -y (or -a) is
  given, only the first in that order is used; -e does not use the 'x'
  value."

**A10. man/dexpireover.8:151: dexpover.dat "removed"**
- Says: "The dexpover.dat file will be removed after being read in by
  dexpireover."
- Code: it is renamed to dexpover.dat.bak, also with -n
  (util/dexpireover.c:2525-2531).
- Fix: "The dexpover.dat file is renamed to dexpover.dat.bak when
  dexpireover reads it, even with -n."

**A11. man/dreaderd.8:407-409: Xref numbering**
- Says: "dreaderd will assign article numbers based on the supplied
  Xref: header or, if the header does not exist, will assign article
  numbers in sequence."
- Code: incoming Xref lines are stripped unless xref-slave mode is on
  (dreaderd/feed.c:286-295, dreaderd/feed.c:356-357); the page's own
  -x entry (L358-365) says so.
- Fix: "In xref-slave mode (-x) dreaderd assigns article numbers from
  the supplied Xref: header; otherwise it assigns them in sequence."

**A12. README.READER:276-278 and :324-328: who reads dexpire.ctl**
- Says: "dreaderd re-reads these files automatically when they
  change" (L276-278); dexpire.ctl "is used by dexpireover"
  (L326).
- Code: dreaderd reads dexpire.ctl too: `a` and `e` when it creates an
  over file, `l` when it opens a group (dreaderd/group.c:963-964,
  dreaderd/group.c:1074-1094). Reader processes reload it
  (dreaderd/reader.c:298-300); feed-only processes, which create most
  new over files, read it only at start (dreaderd/reader.c:188).
- Fix (L326): "This file is used by dexpireover to expire overview
  information, and by dreaderd for the size of new index files and the
  'l' limit. dreaderd's feed-only processes read it only at start."

**A13. README.READER:424-432: maintenance**
- Says: run "once or twice a day"; "articles in the spool(s) have
  expired but still exist in the overview database... I am working on
  dexpireover mechanisms to make this less of a problem."
- Code/samples: daily `-a`, weekly `-a -R` (samples/adm/crontab.sample:33-34,
  samples/adm/daily.reader:39, samples/adm/weekly.reader:28). The
  mechanisms exist: -e, -o (util/dexpireover.c:1458, util/dexpireover.c:1577)
  and `l` (dreaderd/group.c:1188).
- Fix: replace the last sentence with "dexpireover -e or -o and the 'l'
  option in dexpire.ctl address this (see dexpireover(8)).", and add
  "samples/adm/daily.reader and weekly.reader run it daily and with -R
  weekly."

**A14. man/doverctl.8:76-81: `:B` for "all hash methods", default 22**
- Says: "The following option can be used with all hash methods ...
  The default is 22, which is the maximum length."
- Code: `:B` is parsed only for md5 (lib/config.c:1220-1227); `crc:10`
  is rejected (lib/config.c:1186-1191). The value is a snprintf size,
  so 22 gives 21 characters (lib/hash.c:361), and up to 200 is
  accepted (lib/config.c:1224).
- Fix: "The following option can be used with the md5 hash methods:"
  and "The default is 22 (21 characters of the hash)."

**A15. samples/diablo.config:936: readermaxartsize default**
- Says: "The default is 1000000."
- Code: 524288, OVER_HMAPSIZE/2 (lib/config.c:131, lib/vendor.h:336).
  A stored header block over 512 KB is never served anyway
  (dreaderd/group.c:1182).
- Fix: "The default is 524288."

**A16. README.READER:55-59: directory names**
- Says: first tier "00/" through "ff/"; files "named by the history
  hash function on the group name".
- Code: true only for `readergrouphash crc`, the default
  (lib/config.c:119-123, lib/hash.c:384-430); md5 and hierarchy use
  other names and levels (lib/hash.c:433-531).
- Fix: add "(with the default 'readergrouphash crc'; see
  samples/diablo.config for the other methods)".

### Low

**A17. man/dreaderd.8:132-135: blocks of 256**
- Says: "dreaderd maintains overview data files in blocks of 256
  articles".
- Code: 256 is the default of `e` (lib/expire.c:38).
- Fix: "in blocks of 256 articles by default ('e' in dexpire.ctl)".

**A18. Disk sizes contradict each other**
- man/dreaderd.8:95-96 "a 9G disk", man/dreaderd.8:134-135 "no less
  then a 4G disk", README.READER:33-34 "a >20G partition",
  README.READER:254-255 "a 9G partition".
- Code: no requirement. Data is the stored header blocks, about 1 KB
  each (lib/vendor.h:329-331), per group the article is in, plus 32
  bytes per slot (dreaderd/defs.h:732-740).
- Fix: keep one figure, or say "about 1 KB per article per group it is
  posted to".

**A19. man/dexpireover.8:214-216: -x text, resize step**
- Says: normal policy reduces or increases "maxarts by 1/3".
- Code: by a third of the next power of two at or above maxarts,
  clamped to i/j, applied only past a size threshold
  (util/dexpireover.c:1036-1068).
- Fix: "... by a third of the next power of two at or above maxarts
  ..."

**A20. man/dexpireover.8:129-130: -O and removal of files**
- Says: overview data is deleted "the next time dexpireover is run".
- Code: only by a run without -w (util/dexpireover.c:880); -O itself
  requires -w (util/dexpireover.c:456-459).
- Fix: "... until the next dexpireover run without -w."

**A21. man/dexpireover.8: SYNOPSIS and duplicates**
- SYNOPSIS (L7-64) lacks `-z` and `-RR`; `-NB` is described twice
  (L109-112 and L238-241).
- Fix: add `[ -z ]` and `-R[R]`; delete L237-241.
- `-z` (L243-246): add "Needs a zlib build. Defaults: Z24, F2"
  (util/dexpireover.c:340-346, lib/expire.c:40-41).

**A22. man/dexpireover.8:98-99: -f default**
- Says: "The default is /news/dactive.kp."
- Code: `path_reader_dactive`, default dactive.kp in path_db
  (lib/global.c:96, util/dexpireover.c:447-451).
- Fix: "The default is path_reader_dactive (dactive.kp in path_db)."

**A23. man/dexpireover.8:75-77: what updates the active file**
- Says: "If the -a or -NB options are given, it will update the active
  file."
- Code: -U and -O write it too (util/dexpireover.c:566-620).
- Fix: "If the -a, -NB, -U or -O options are given, ..."

**A24. man/dexpireover.8:164-165: -p spread**
- Says: "Values other than powers of 2 are likely to generate uneven
  load".
- Code: work is split by the first character of each top directory
  (util/dexpireover.c:716-717). With the crc layout that is 16 values
  (0-9a-f); with -p16 four workers get nothing (base64 index,
  lib/hash.c:240-241).
- Fix: add "With the default crc layout, use at most 8."

**A25. man/doverctl.8:48-49 and samples/diablo.config:865: md5-32**
- Says: "a 32 bit base64 type conversion".
- Code: base32 encoding (a-z0-5) of the MD5 digest (lib/hash.c:238,
  lib/hash.c:358-360).
- Fix: "uses a base32 conversion of an md5 hash of the newsgroup name."

**A26. man/doverctl.8:89-91 and SYNOPSIS (L5-19)**
- `-w` has no effect on `clean`; clean uses the full group list
  (util/doverctl.c:234-236, util/doverctl.c:251). SYNOPSIS lacks -m,
  -n, -C, -d, -V.
- Fix: add "It has no effect on clean." and the missing options.

**A27. man/dreadover.8: options**
- SYNOPSIS (L7-17) lacks -h, -H, -d and "-" (read group:art lines from
  stdin, util/dreadover.c:68-79).
- L50-52 "-d[n] Set the debug level": -d only shows byte-order
  errors, no level (util/dreadover.c:94-96).
- L38-40 "-h": needs a group argument after it
  (util/dreadover.c:66-71, util/dreadover.c:104-106).
- Fix: "-d  Report over files with the wrong byte order." and "-h
  group  Print the overview header details of group."

**A28. man/dreaderd.8:413-414: cron**
- Says: "usually twice a day".
- Samples: daily, plus -R weekly (samples/adm/crontab.sample:33-34).
- Fix: "usually once a day, and with -R once a week (see
  samples/adm/daily.reader and weekly.reader)."

**A29. man/diablo-files.5:145-177: DEXPIRE.CTL section incomplete**
- Lists x, a, i, j only.
- Code: also e, l, Z, F (lib/expire.c:137-160); all matching lines
  apply, later values override (lib/expire.c:42-62).
- Fix: add one line each, or "See samples/dexpire.ctl for the e, l,
  Z and F options."

**A30. man/diablo-kp.5:9-11 and :38-39: overview in KP**
- Says the KP module is intended to manage "the overview cache" and
  "overview caching databases".
- Code: overview uses its own binary files (dreaderd/group.c:1-27);
  KP holds dactive.kp only.
- Fix: drop "the overview cache and" and ", and overview caching
  databases for the same reason".

**A31. samples/reader/dexpire.ctl and samples/feeder+reader/dexpire.ctl**
- Lack the Z and F entries and "it must be a power of 2" for e
  (compare samples/dexpire.ctl:36-66).
- Fix: copy those lines from samples/dexpire.ctl.

**A32. samples/dexpire.ctl:56-59: Z "must always be at least 1 hour"**
- Code: not enforced; default 24, F default 2, not stated
  (lib/expire.c:40-41, util/dexpireover.c:1160-1168).
- Fix: add "The default is 24." and for F "The default is 2."

**A33. README.READER:132-134: trackonly on the L2 cache box**
- Says the cache-only box (D), which "takes NO FEEDS", may use
  `readerxover trackonly`.
- Code: trackonly only changes what a feed writes
  (dreaderd/group.c:795, dreaderd/group.c:817-818).
  samples/diablo.config:755-760 gives the real use.
- Fix: drop the bullet, or move it to topology (E).

**A34. ISSUES.md:1163 (D-174 plan)**
- Says LIST OVERVIEW.FMT "may then say `:lines`".
- Code: still `Lines:` (dreaderd/defs.h:39). It is a "may"; mark it
  not done if the entry is touched.

**In-code comments (not admin docs, same kind of drift)**
- dreaderd/group.c:4-7 and :17: "256 subdirectories" (crc only); the
  header is "struct OverInfo", it is OverHead (dreaderd/defs.h:688).
- dreaderd/defs.h:684-685: "over.groupname", "numa.groupname (article
  number assignment file)": no numa files exist.
- util/dexpireover.c:28-29: "The data file will have an advisory lock
  at offset 4 from dreaderd": dreaderd never locks data files there
  (dreaderd/group.c:1663-1669, dreaderd/group.c:715 locks offset 0
  while appending).

Checked and found accurate for overview: INSTALL:116-159,
samples/adm/daily.reader, samples/adm/weekly.reader,
samples/adm/crontab.sample, README.md (only changelog lines mention
overview), samples/dreader.access (feed readerdef, xzver).

## B. Ranking note

A1-A7 lead to wrong capacity planning or lost overview: admins think
data files or `-a` handle growth, run dexpireover too rarely, keep the
1 s lock wait, trust an `i` below 32 to be ignored, or run doverctl
clean live.

## C. Important behaviour no doc mentions

Ranked by impact.

1. **Overwrite at maxarts is silent.** No log line; GROUP just raises
   the low mark (dreaderd/group.c:812-839, dreaderd/nntp.c:392-395).
   README.READER:66-68 mentions overwriting, not how to notice it. The
   only trace is dexpireover's "adjust ... startno" line with -v
   (util/dexpireover.c:953-961). Suggest one sentence in
   dexpireover.8 and README.READER.
2. **Changing readergrouphash, or -f with another active file, deletes
   overview.** dexpireover removes every file it can't map to a group
   (util/dexpireover.c:874-886). doverctl.8:99-101 warns for convert;
   dexpireover.8 doesn't.
3. **dexpire.ctl `a`/`e` only apply when an over file is created**, and
   feed-only processes never reload the file (dreaderd/group.c:1074-1094,
   dreaderd/reader.c:188, dreaderd/reader.c:298-300). After editing
   `a`, restart dreaderd or the feed side keeps the old value.
4. **Expire age is the reader's receive time**, not the Date header;
   records more than a day in the future are expired too
   (util/dexpireover.c:1384-1388, dreaderd/group.c:823).
5. **What is stored**: the full header block, so any header works for
   HDR/XPAT; one data file can't exceed 2 GB (int offset); a header
   block over 512 KB is never served (dreaderd/group.c:729-733,
   dreaderd/defs.h:734, dreaderd/group.c:1182).
6. **By message-id**: OVER/XOVER/HDR `<msgid>` search only the current
   group, newest 100,000 articles (dreaderd/xover.c:14,
   dreaderd/group.c:1558-1598).
7. **GROUP creates an empty over file** (16,660 bytes at 512 slots) for
   each group a client selects (dreaderd/nntp.c:392, dreaderd/group.c:992,
   dreaderd/group.c:1085).
8. **Compressed data files**: dreaderd inflates a whole .gz file to
   /tmp on access, max 64 MB (dreaderd/group.c:1673-1700). A late
   article for a gzipped block creates a plain file that hides the .gz
   (dreaderd/group.c:1669-1673); that is why `F` exists.
9. **Over files are host binary** (byte order, int size); they can't
   move between architectures (dreaderd/group.c:1032-1036).
10. **Expired bytes stay in data files** until the whole file is below
    NB or `-R` runs (util/dexpireover.c:1137-1152,
    util/dexpireover.c:1969-1998).
11. **No rebuild from the spool.** Lost overview returns only by a
    refeed (no tool in util/ reads the spool into overview).
12. **Locks**: fcntl, 4 bytes at offset 0 (writes) and 4 (in use /
    resize) (lib/lock.c:66-94, dreaderd/group.c:1004-1015). Useful when
    debugging "file in use" with lslocks/fstat.
