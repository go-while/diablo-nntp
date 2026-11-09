From: go-while/cc-1 (opus-5.5)
To: jesse/cc-1, Jesse
Re: 20261107-221428Z-go-while-cc-1-overview-review.md

[Claude] Overview review part: how it works (main 9320a42).

# Diablo reader overview: how it works

For a news admin who knows INN but has never run a Diablo reader.
Source: upstream main 9320a42. Every claim cites file:line in that tree.

## 1. The short version

- dreaderd stores the **full header block** of each article, not a
  pre-formatted overview line. OVER/XOVER/HDR lines are built from the
  stored headers when a client asks (dreaderd/xover.c:630, dreaderd/group.c:729-733).
- Each group has one fixed-size index file (`over.*`), a ring of
  `maxarts` 32-byte slots, plus data files (`data.*`) that hold the
  headers, each covering a fixed block of article numbers
  (dreaderd/group.c:1149-1155, dreaderd/group.c:1635-1637).
- Slot = article number modulo maxarts. When a group gets more than
  maxarts articles, new articles silently overwrite the oldest slots
  (dreaderd/group.c:1153-1154, dreaderd/group.c:812-839).
- Only `dexpireover` changes maxarts, by about a third per run, and
  only when no dreaderd process has the group open
  (util/dexpireover.c:1036-1039, util/dexpireover.c:2247-2256).
- Article numbers, the low/high marks and the hash-collision counter
  live in the reader's `dactive.kp` (fields NB, NE, ITER)
  (dreaderd/group.c:543-609, dreaderd/group.c:973, dreaderd/group.c:1118-1122).

## 2. On-disk layout

### 2.1 Where

- Root: `path_group`, default `<path_home>/spool/group`
  (lib/global.c:83, lib/config.c:387-388). dreaderd reads it once at
  start (dreaderd/main.c:567).
- The layout below that is chosen by `readergrouphash`
  (lib/config.c:285-288). Default `crc` (lib/config.c:119-123).
- Directories are created on demand (dreaderd/group.c:988).
- `dgrpctl groupinfo GROUP` prints the over file path, honouring ITER
  (util/dgrpctl.c:305-319).

### 2.2 File names per hash method

`artBase` is the first article number a data file covers:
`artNo & ~(dataEntries-1)` (dreaderd/group.c:1637). The over file
always has artBase 0 (dreaderd/group.c:983). `iter` is the ITER field.

| method | over file | data file | directories |
|---|---|---|---|
| `crc` (default) | `over.0.H1.H2[.iter]` | `data.ARTBASE.H1.H2[.iter]` | one level, `00`..`ff` |
| `md5-32` / `md5-64` | `HASH.iter.o.32.0` (`.64`) | `HASH.iter.d.32.ARTBASE` | per `/N` or `\N` levels |
| `hierarchy` | `comp/lang/o.0.comp.lang.c` | `comp/lang/d.ARTBASE.comp.lang.c` | one per name component |

- crc: H1.H2 is `hhash()` of the group name as two 8-digit hex words;
  the directory is the low byte of H1, i.e. the last two hex digits of
  H1; `.iter` only when iter > 0 (lib/hash.c:384-430, lib/hash.c:347-351).
- md5: the hash is the MD5 of the name, base32-encoded (a-z0-5) for
  md5-32, base64 (a-zA-Z0-9+-) for md5-64, cut to `:B` minus 1 characters (default 22, so 21; lib/hash.c:361);
  `.iter` is always present (lib/hash.c:352-363, lib/hash.c:238-241,
  lib/hash.c:433-504).
- hierarchy: one-letter components are skipped; `iter` is not in the
  name at all (lib/hash.c:500-531, lib/hash.c:606-629).
- dexpireover -z adds `.gz` to data file names (util/dexpireover.c:1178).
- dexpireover -R leaves `.data.*` / `.HASH...d.*` temp files on
  interruption; the next run deletes them (util/dexpireover.c:789-798).

### 2.3 Hash collisions: ITER

If an over file's header names another group, dreaderd tries
`iter+1`, and stores the iter that worked in the group's ITER field
(dreaderd/group.c:1111-1122). Tools that ignore ITER look at the wrong
file (see 7.4).

### 2.4 The over file (index)

Native byte order, native int size: a file from a big-endian host is
rejected (dreaderd/group.c:1032-1036).

```
offset 0      OverHead (276 bytes; union with int reserved[32])
                0  int version      (4, OH_VERSION)
                4  int byteOrder    (0xF1E2D3C4)
                8  int headSize     (276 = sizeof(OverHead))
               12  int maxArts      (as written at creation/resize)
               16  char gName[256]  (group name, max 255 chars + NUL)
              272  int dataEntries  (articles per data file, power of 2)
offset 276    OverArt slot[0 .. maxarts-1], 32 bytes each:
                0  int    oa_ArtNo    low 31 bits of the article number,
                                      -1 cancelled, -2 expired
                4  int    oa_SeekPos  offset in the data file, -1 = no data
                8  int    oa_Bytes    length of the stored headers (no NUL)
               12  hash_t oa_MsgHash  2 x int32, hhash(Message-ID)
               20  int    oa_ArtSize  article size, for :bytes
               24  int    oa_TimeRcvd time() when dreaderd stored it
               28  int    oa_Lines    body lines + 1, 0 = unknown
```

Sources: dreaderd/defs.h:688-712 (header), dreaderd/defs.h:727-743
(slot), lib/defs.h:306-309 (hash_t); sizes checked with a compiled
copy of the structs (276 and 32 bytes).

- File size = 276 + 32 x maxarts. The default 512 slots is 16,660 bytes
  (dreaderd/group.c:1085, lib/expire.c:34).
- dreaderd takes maxarts from the file size, not from the header field
  (dreaderd/group.c:1126). GROUP uses the header field
  (dreaderd/nntp.c:393-394).
- A slot is valid when `oa_ArtNo` equals the low 31 bits of the wanted
  number (dreaderd/defs.h:727-730). An unused slot is all zero.

### 2.5 Article number to position

```
slot   = (artNo & 0x7FFFFFFFFFFFFFFF) % maxarts        (group.c:1153-1154)
ovpos  = 276 + slot * 32
file   = data file for artBase = artNo & ~(dataEntries-1) (group.c:1637)
record = data file [oa_SeekPos .. oa_SeekPos + oa_Bytes], then a NUL
```

Example with maxarts 512, dataEntries 256: article 1000 is slot 488,
in data file `data.768...`; article 1512 lands in the same slot 488.

### 2.6 Data files

- A data file is an append-only list of NUL-terminated header blocks,
  in arrival order (dreaderd/group.c:13-14, dreaderd/group.c:726,
  dreaderd/group.c:799).
- What is stored: the header block as received; without an Xref in
  slave mode, dreaderd inserts its own `Xref:` line
  (dreaderd/group.c:729-756). The body of a fed article is not
  buffered unless it is a control message (dreaderd/feed.c:298-326).
- The read path checks a NUL before and after each record and logs
  "corrupt overview entry" otherwise (dreaderd/group.c:1270-1286).
- `oa_SeekPos` is an int: a data file can't pass 2 GB
  (dreaderd/defs.h:734).
- A record longer than 512 KB (OVER_HMAPSIZE/2) is never returned
  (dreaderd/group.c:1182, lib/vendor.h:336).

### 2.7 maxarts, the ring, and wrap

- maxarts is the number of slots. Default 512 for a new group, or the
  `a` value of dexpire.ctl (lib/expire.c:34, dreaderd/group.c:1074-1092).
- At wrap (more than maxarts articles between NB and NE) the new
  record overwrites the slot of `artNo - maxarts`; no warning is
  logged (dreaderd/group.c:812-839).
- GROUP clamps the low mark to `NE - maxarts`
  (dreaderd/nntp.c:392-395). Without an over file it uses 1024
  (dreaderd/nntp.c:397-400, dreaderd/defs.h:40).
- dexpireover moves NB to `NE - maxarts + 1` when the ring has wrapped
  (util/dexpireover.c:952-962) and writes it back with -NB
  (util/dexpireover.c:609-615).
- Data bytes of overwritten or expired records stay in their data file
  until the whole file falls below NB, or until `dexpireover -R`
  (util/dexpireover.c:1137-1152, util/dexpireover.c:1969-1998).

## 3. Write path (feed into dreaderd)

### 3.1 Who writes

- Feeds connect to the reader port and need `feed yes` in
  dreader.access (samples/dreader.access:278, samples/dreader.access:427-429).
  A readerdef with `feed` and no `read`/`post` goes to a feed-only
  process (samples/dreader.access:450-453). There are `readerfeedforks`
  of them, default 2 (lib/config.c:134, dreaderd/main.c:1052-1064).
- A diablo feeder sends a header-only feed with `headfeed` in
  dnewsfeeds (samples/dnewsfeeds:533-536, samples/dnewsfeeds:879-887);
  dreaderd accepts `MODE HEADFEED` only from feed connections
  (dreaderd/feed.c:53-60).
- A header feed must carry a `Bytes:` line, else 437/439
  (dreaderd/group.c:273-279). dnewslink sends `Bytes: size lines`;
  dreaderd keeps size and line count and drops the line
  (dreaderd/feed.c:270-283).
- Required headers: Newsgroups, Message-ID, Subject, Date
  (dreaderd/group.c:261-271). A message-id in the cancel cache is
  refused (dreaderd/group.c:280-291).

### 3.2 Steps per article (NNFeedOverview)

```
headers parsed ──> pass 1: AssignArticleNo() per group     (group.c:325-368)
                     KP_LOCK the group's dactive.kp record
                     artNo = NE+1, or from Xref in slave mode
                     write NE (and NB), unlock               (group.c:543-609)
               ──> Supersedes executed                     (group.c:379-382)
               ──> pass 2: WriteOverview() per group        (group.c:384-414)
                     open/create over file (GetOverInfo)
                     open/create data file (MakeOverHFile)
                     lock data file [0,4) exclusive; append headers
                     lock over file [0,4) exclusive; pwrite 32-byte slot
                     unlock
               ──> control message executed                (group.c:424-432)
               ──> 235 / 239                               (group.c:434-440)
```

- Groups not in dactive.kp are skipped and logged "not in dactive",
  unless `readerautoaddtoactive` is on (dreaderd/group.c:472-510,
  lib/config.c:650-653).
- Unapproved articles to moderated groups (flag `m`) are dropped
  (dreaderd/group.c:517-537).
- Control messages are filed under `control.<type>` instead of their
  Newsgroups (dreaderd/group.c:304-320).
- Article number: `NE+1` (dreaderd/group.c:547). In xref-slave mode
  (`readerxrefslavehost` or `-x`) the number comes from the Xref entry
  for the group, and a group without one is dropped
  (dreaderd/group.c:549-574). Outside slave mode incoming Xref lines
  are stripped (dreaderd/feed.c:286-295, dreaderd/feed.c:356-357).
- Xref slave mode detects a duplicate when the slot already holds the
  same number and message-id hash (dreaderd/group.c:769-782).
- The slot is written only after the data append succeeds; on a slot
  write error the data append is truncated away and the feed gets
  `400` and is closed (dreaderd/group.c:797-867).
- `readerxover trackonly` writes slots with `oa_SeekPos = -1` and no
  data (dreaderd/group.c:795, dreaderd/group.c:817-818,
  lib/config.c:702-712). Any other value means "on"; mode 0 can't be
  set (lib/config.c:708-710).

### 3.3 Locking

All locks are fcntl byte-range locks of 4 bytes (lib/lock.c:66-94).

| lock | who | meaning |
|---|---|---|
| over file [4,8) shared | every process that has the group open (dreaderd, dcancel, dgrpctl) | "in use, don't resize" (dreaderd/group.c:1004-1015) |
| over file [4,8) exclusive | dexpireover -s/-R | resize/rewrite (util/dexpireover.c:2244-2257, util/dexpireover.c:1721-1734) |
| over file [0,4) exclusive | slot writes: feed, cancel, dcancel, dgrpctl masscancel | one writer per slot update (dreaderd/group.c:808, dreaderd/group.c:1613, util/dcancel.c:929) |
| data file [0,4) exclusive | feed append | one appender (dreaderd/group.c:715) |

- dreaderd keeps up to 8 groups open per process (OV_CACHE_MAX,
  dreaderd/defs.h:42, dreaderd/group.c:933-953), and so keeps their
  shared lock.
- Reader processes drop idle groups every 30 s, feed-only processes
  every 300 s (dreaderd/reader.c:296-300, dreaderd/reader.c:336-343).
- mmap: the index is mapped read-only, shared; slots are written with
  `pwrite()` (dreaderd/group.c:1128, dreaderd/group.c:103-124).
  Data files are appended with `writev()` and read through mmap
  windows (dreaderd/group.c:131-174, dreaderd/group.c:1256-1263).

### 3.4 Active file interplay

- NE moves forward at number assignment, before the overview is
  written (dreaderd/group.c:576-609).
- NB is written by dreaderd only when an article arrives below it, or
  for a new group (dreaderd/group.c:578-608, dreaderd/group.c:484-488).
  Otherwise NB only moves through `dexpireover -NB`/`-a`.
- LMTS is updated per article (dreaderd/group.c:592-599); dexpireover
  -O uses it (util/dexpireover.c:568-594).
- ITER is written when a collision moved the group (dreaderd/group.c:1118-1122).

## 4. Read path

### 4.1 GROUP

Reads NB/NE from dactive.kp; the low mark is raised to `NE - maxarts`;
the count is `high - low + 1`, holes included (dreaderd/nntp.c:378-403,
dreaderd/nntp.c:200-205). GROUP opens, and if needed creates, the
group's over file (dreaderd/nntp.c:392, dreaderd/group.c:992).

### 4.2 OVER / XOVER / XZVER / HDR / XHDR / XPAT

- All go through `NNStartListOverview()` (dreaderd/xover.c:245).
  The range is clamped to NB..NE (dreaderd/xover.c:440-492).
- Per article: `GetOverRecord()` finds the slot, checks the number,
  the `l` limit and NEWNEWS time, maps the data file window and returns
  the header block (dreaderd/xover.c:547-552, dreaderd/group.c:1172-1288).
- `OutputOverview()` picks the fields named in the format out of the
  header block. The default format is
  `Subject From Date Message-ID References Bytes Lines Xref:full`
  (dreaderd/defs.h:39, dreaderd/xover.c:630). LIST OVERVIEW.FMT prints
  that list (dreaderd/list.c:517-520).
- `:bytes` falls back to `oa_ArtSize`, `:lines` comes from `oa_Lines`
  when the feed gave it, else from a `Lines:` header
  (dreaderd/xover.c:677-698, dreaderd/xover.c:879-897).
- Because full headers are stored, HDR/XHDR/XPAT work on any header
  (dreaderd/list.c:389-391).
- One pass handles at most 10,000 slots, XPAT 50, then yields
  (dreaderd/xover.c:536-543).

### 4.3 By message-id

- OVER/XOVER/HDR/XHDR `<msgid>`: a scan of the current group's slots,
  newest first, comparing `oa_MsgHash`, at most 100,000 articles
  (dreaderd/xover.c:14, dreaderd/xover.c:334-371, dreaderd/group.c:1558-1598).
  There is no message-id index.
- HDR/XHDR then fall back to `HEAD <msgid>` from the spool
  (dreaderd/xover.c:345-361).
- ARTICLE/HEAD/BODY/STAT by number: the slot gives the stored headers
  and the Message-ID; the body is fetched by message-id from the cache
  or a spool server (dreaderd/nntp.c:601-680, dreaderd/group.c:1880-1950).
- LISTGROUP lists the numbers whose slot is valid (dreaderd/list.c:681-710).

### 4.4 Time limits

- `l` in dexpire.ctl hides records older than N days, measured from
  `oa_TimeRcvd`, the time dreaderd stored them, not the Date header
  (dreaderd/group.c:964, dreaderd/group.c:1188-1195, lib/expire.c:149-154).
- It applies to everything that uses `GetOverRecord()`: OVER, HDR,
  ARTICLE by number, LISTGROUP, STAT (dreaderd/group.c:1172-1210).
  GROUP counts do not apply it (dreaderd/nntp.c:378-403).
- The value is read when a process opens the group; reader processes
  reload dexpire.ctl at most once a minute when its mtime changes
  (dreaderd/reader.c:300, lib/expire.c:172-213).

### 4.5 Reader cache

- The article cache (`readercache`, `path_cache`) holds bodies by
  message-id; it is separate from overview (dreaderd/cache.c:87-130,
  lib/config.c:685-688).
- For a full ARTICLE from the cache, headers can still come from
  overview (dreaderd/cache.c:420-433).

## 5. Expire and maintenance

### 5.1 dexpireover

Scans every directory under path_group, maps each file name back to a
group through dactive.kp, and handles over files first, then data
files (util/dexpireover.c:691-742, util/dexpireover.c:746-814).

| option | effect | source |
|---|---|---|
| `-a` | `-y -NB -U -s` | util/dexpireover.c:265-270 |
| `-y[N]` | expire by `x` days; `-y0` off | util/dexpireover.c:337-339 |
| `-e` | expire what dhistory lists as gone or expired (local history needed); ignores `x` | util/dexpireover.c:1505-1520 |
| `-o` | expire hashes in `dexpover.dat` (written by `dexpire -o`) or older than `x` | util/dexpireover.c:1620-1635, util/dexpire.c:1095-1102 |
| `-NB` | write the new NB back | util/dexpireover.c:609-615 |
| `-s` | resize index files that need it | util/dexpireover.c:998-1070 |
| `-R` / `-RR` | rewrite every group's over and data files (-RR drops bad records) | util/dexpireover.c:1055, util/dexpireover.c:1689-2120 |
| `-z` | gzip old data files (zlib build) | util/dexpireover.c:1155-1186 |
| `-O[N]` | delete groups with LMTS older than N days (90); needs `-w` | util/dexpireover.c:297-302, util/dexpireover.c:456-459, util/dexpireover.c:568-594 |
| `-U` | add missing CTS | util/dexpireover.c:596-607 |
| `-w wild` | only these groups; no orphan removal | util/dexpireover.c:878-886 |
| `-l N` | seconds to wait for each group's resize lock, default 1 | util/dexpireover.c:192, util/dexpireover.c:2246-2256 |
| `-pN` | N workers, split by first character of the top directory, max 16 | util/dexpireover.c:306-313, util/dexpireover.c:716-717 |
| `-n` | dry run, implies `-v` | util/dexpireover.c:292-296 |
| `-x` | size by article age instead of fill ratio | util/dexpireover.c:1016-1035 |

Only one expire method runs: `-e` wins over `-o`, `-o` over `-y`
(util/dexpireover.c:1078-1083).

Expire marks a slot `-2` when `oa_TimeRcvd` is older than `x`, or more
than a day in the future (util/dexpireover.c:1384-1398). NB then
advances over leading invalid slots (util/dexpireover.c:1413-1434).
A data file is deleted when its whole block lies below NB, or above
NE + dataEntries, or is misaligned (util/dexpireover.c:1137-1152).

A file whose name maps to no group in dactive.kp is deleted
(util/dexpireover.c:874-886). So a wrong `readergrouphash` or a
`-f` with another active file deletes the overview.

### 5.2 Sizing (-s and -R)

```
numArts = NE - NB + 1
if numArts < maxarts/2:   new = maxarts - nextpow2(maxarts)/3
if numArts > maxarts*2/3: new = maxarts + nextpow2(maxarts)/3
clamp to [i, j]
resize only if |new - maxarts| >= 10 (<100), > 20 (<1000), > 200
```

Source: util/dexpireover.c:1036-1068, util/dexpireover.c:2501-2508.

- One step per run, about x1.33-x1.5. From 512 to 50,000 slots takes
  about 14 runs; with the daily sample cron, two weeks of overwritten
  articles (samples/adm/crontab.sample:33).
- The resize copies valid slots into a new ring and rewrites the file
  in place (util/dexpireover.c:2381-2448). It needs the exclusive
  [4,8) lock, i.e. no process may have the group open
  (util/dexpireover.c:2244-2257). Otherwise "resize ... failed, file in
  use" and the next run tries again.
- `-R` always rebuilds: new data files are written as `.name`, the old
  ones deleted, the over file replaced by rename, the new data files
  renamed in (util/dexpireover.c:1969-2108). A crash between the delete
  and the rename loses the group's overview (util/dexpireover.c:1969-1972).
  `e` changes take effect here (samples/dexpire.ctl:43-44).

### 5.3 dexpire.ctl

Path `path_dexpire`, default `<path_home>/dexpire.ctl`
(lib/global.c:97). Format `wildmat:opt:opt...`; every matching line
applies, later values override earlier ones (lib/expire.c:42-62).

| opt | meaning | default | source |
|---|---|---|---|
| `xN` | expire after N days (float) | never | lib/expire.c:33, lib/expire.c:146-148 |
| `lN` / `lx` | hide records older than N days; `lx` = x | 0 (off) | lib/expire.c:34, lib/expire.c:149-154, lib/expire.c:63-64 |
| `aN` | slots for a new over file | 512 | lib/expire.c:35, dreaderd/group.c:1074-1092 |
| `iN` | resize minimum | 32 | lib/expire.c:36, util/dexpireover.c:1051 |
| `jN` | resize maximum | none | lib/expire.c:37, util/dexpireover.c:1049 |
| `eN` | articles per data file, power of 2 | 256 | lib/expire.c:38, lib/expire.c:137-145 |
| `ZN` | -z: only files older than N hours | 24 | lib/expire.c:40, util/dexpireover.c:1168 |
| `FN` | -z: never the newest N files | 2 | lib/expire.c:41, util/dexpireover.c:1160 |

dreaderd needs the file: it exits if it is missing (lib/expire.c:192-198).

### 5.4 What the sample cron runs

- Daily 02:45: `daily.reader` = `dexpireover -a -l180` under plock,
  plus a 3-day cache cleanup (samples/adm/crontab.sample:33,
  samples/adm/daily.reader:28-44).
- Weekly Sunday 03:30: `weekly.reader` = `dexpireover -a -R -l720`
  (samples/adm/crontab.sample:34, samples/adm/weekly.reader:28).
- Both lines are commented out; uncomment them on a reader
  (samples/adm/crontab.sample:31-34, INSTALL:157-159).
- `-l` is per group, so a run with many busy groups can take
  groups x lockwait.

### 5.5 Other tools

- `doverctl convert SRC DST`: moves files to another
  `readergrouphash` layout; all Diablo processes must be stopped
  (util/doverctl.c:84-100, util/doverctl.c:286-360).
- `doverctl clean`: deletes empty directories, unknown files, files of
  groups not in dactive.kp, over files with a bad header, and data
  files outside NB..NE (util/doverctl.c:457-700). The source says the
  server must be down (util/doverctl.c:11).
- `dreadover group:N[-M]` prints slot and data file details, `-v` the
  headers, `-a` maxarts per group, `-h group` the header, `-H file` a
  header by path (util/dreadover.c:123-133, util/dreadover.c:181-345).
- `dcancel -r group:N` marks one slot cancelled (util/dcancel.c:277-300).
- `dgrpctl masscancel GROUP < msgids` cancels a list in one group
  (util/dgrpctl.c:140-240).
- A cancel control message marks the slot `-1` in every group of the
  cancelled article (dreaderd/control.c:820-828, dreaderd/group.c:1605-1631).

### 5.6 Resize one group now

1. Stop dreaderd, or wait for its processes to drop the group
   (30 s for reader processes, 300 s for feed-only processes).
2. Set `i`/`j` (or `a` for new groups) in dexpire.ctl.
3. `dexpireover -s -w 'the.group' -l600 -v` (util/dexpireover.c:998-1070).

To force a size: set `i` and `j` to the same value and run `-s`; it
resizes only if the change passes the thresholds in 5.2.

### 5.7 Rebuild and recovery

- Bad records: `dexpireover -RR -w 'group'` drops slots whose data is
  missing or fails the NUL guards (util/dexpireover.c:2204-2215).
- A broken over file header (wrong byte order, too short) is replaced
  by an empty one the next time dreaderd opens the group
  (dreaderd/group.c:1048-1104). All articles of that group are lost
  from overview.
- There is no rebuild from the spool. Lost overview comes back only
  through a refeed, e.g. `dspoolrefeed` on the feeder (ISSUES.md:1161).

## 6. diablo.config knobs that matter

| option | default | effect | source |
|---|---|---|---|
| `path_group` | `%s/spool/group` | overview root | lib/global.c:83, lib/config.c:387 |
| `path_dexpire` | `%s/dexpire.ctl` | dexpire.ctl path | lib/global.c:97, lib/config.c:413 |
| `path_dexpover_list` | `%s/dexpover.dat` (path_db) | -o list | lib/global.c:110, lib/config.c:433 |
| `path_reader_dactive` | `%s/dactive.kp` (path_db) | reader active | lib/global.c:96, lib/config.c:403 |
| `readergrouphash` | `crc` | file layout | lib/config.c:119-123, lib/config.c:1183-1258 |
| `readerxover` | on | `trackonly`: slots only | lib/config.c:143, lib/config.c:702-712 |
| `readerxrefslavehost` / `-x` | none | take numbers from Xref | lib/config.c:791-793, dreaderd/main.c:532 |
| `readerxrefhost` / `-X` | slave host, pathhost, hostname | host in generated Xref | lib/config.c:786-788, dreaderd/main.c:888-893 |
| `readerautoaddtoactive` | off | create unknown groups | lib/config.c:145, lib/config.c:650-653 |
| `readermaxartsize` | 524288 | larger fed header blocks are refused | lib/config.c:131, dreaderd/feed.c:308 |
| `readerfeedforks` | 2 | feed-only processes | lib/config.c:134, lib/config.c:356 |
| `readerthreads` | 40 | also the per-group data file fd cache | lib/config.c:133, dreaderd/group.c:1804 |

## 7. Set up a reader with overview: checklist

1. Install; `spool/group` exists, owned by news (INSTALL:116-117).
   Give it its own filesystem (INSTALL:126-129).
2. Pick `readergrouphash` now; changing it later needs `doverctl
   convert` with everything stopped (samples/diablo.config:880-882).
   For many groups use md5 with directory levels
   (samples/diablo.config:884-886).
3. Create dactive.kp (dsyncgroups, or `dgrpctl newgroup`); dreaderd
   needs it (README.READER:293-307). Only groups in it get overview
   (dreaderd/group.c:500-509).
4. Write dexpire.ctl: `x`, `lx`, and `a`/`i` big enough for your
   busiest groups so they don't start at 512 slots (samples/dexpire.ctl:71-83).
5. dreader.access: a `feed yes` readerdef for the feeding host
   (samples/dreader.access:427-429, samples/dreader.access:525).
6. Feeder: a dnewsfeeds label with `headfeed` and `alias dummy`
   (samples/dnewsfeeds:879-887). Decide who numbers articles: dreaderd
   (default) or the feeder with xref-slave mode (README.READER:170-221).
7. Start dreaderd. Check the log for "+ msgid group:N" lines
   (dreaderd/group.c:858) and "not in dactive" (dreaderd/group.c:492).
8. Install the cron lines for daily.reader and weekly.reader
   (samples/adm/crontab.sample:31-34).
9. Watch dexpireover output for "resize ... failed, file in use" and
   for groups that keep growing (util/dexpireover.c:2251-2254).

## 8. Why it feels painful

1. **Fixed ring per group, silent overwrite.** A group that outgrows
   maxarts loses its oldest overview without any log line
   (dreaderd/group.c:812-839). The only signal is NB jumping in the
   next dexpireover run (util/dexpireover.c:952-962).
2. **Slow, lock-gated growth.** maxarts grows about a third per run
   (util/dexpireover.c:1038-1039), only when no process has the group
   open (util/dexpireover.c:2244-2257). Busy groups are the ones held
   open. The default lock wait is 1 s (util/dexpireover.c:192).
3. **Sizing is guesswork.** You set `a`, `i`, `j`, `e` per wildmat
   with no tool that reports fill or overwrite rates. dreadover -a only
   lists maxarts (util/dreadover.c:426-428).
4. **Settings don't apply where you expect.** `a` and `e` only matter
   at file creation (dreaderd/group.c:1074-1094); `e` changes need
   `-R`. Feed-only processes load dexpire.ctl once at start
   (dreaderd/reader.c:188) and never reload it (dreaderd/reader.c:298-300
   is the reader-only branch); they create most new over files.
5. **Two numbering truths.** NB/NE live in dactive.kp, slots in over
   files; GROUP, dexpireover and the slots each derive "low" differently
   (dreaderd/nntp.c:392-395, util/dexpireover.c:952-962,
   util/dexpireover.c:1413-1434).
6. **Duplicated code in separate tools.** dcancel and dgrpctl carry
   their own GetOverInfo/GetOverArt (util/dcancel.c:706-918,
   util/dgrpctl.c:946-1160); dexpireover and doverctl carry their own Group
   table and EnterGroup (util/dexpireover.c:1253-1305,
   util/doverctl.c:392-441). They drift: dcancel never loads
   dexpire.ctl (util/dcancel.c:832), dreadover ignores ITER
   (util/dreadover.c:213).
7. **Destructive defaults.** dexpireover and doverctl clean delete any
   file they can't map to a group in the current dactive.kp with the
   current hash method (util/dexpireover.c:874-886, util/doverctl.c:611-629).
8. **No message-id index.** Lookup by message-id scans one group's ring
   (dreaderd/group.c:1558-1598); cancels scan every listed group
   (dreaderd/group.c:1491-1546).
9. **No rebuild.** If over or data files are lost, only a refeed
   restores them (5.7).
10. **Native binary format.** Byte order and int size are the host's;
    files don't move between architectures (dreaderd/group.c:1032-1036,
    dreaderd/defs.h:732-740).

## 9. Things I tripped over (for the bug review, unverified unless noted)

- dexpireover's expire passes write slots without the [0,4) lock; a
  slot dreaderd just reused after wrap can be overwritten with the old
  record marked -2 (util/dexpireover.c:1386-1398 vs dreaderd/group.c:808).
- GROUP low `NE - maxarts` is one below the oldest slot still held, so
  after wrap the low article is always missing (dreaderd/nntp.c:393-394).
- dreadover and `dgrpctl listgrouphashfile` use iter 0 always
  (util/dreadover.c:213, util/dreadover.c:285, util/dreadover.c:393,
  util/dgrpctl.c:451, util/dgrpctl.c:464).
- `dgrpctl masscancel` compares `oa_ArtNo == artEnd` (fails above
  2^31) and holds the group's active record lock for the whole walk
  (util/dgrpctl.c:182, util/dgrpctl.c:198).
- hierarchy layout: `d.%lld.%[^/].gz` is greedy, so a `.gz` data file
  maps to group "name.gz" and is deleted as an orphan (lib/hash.c:688-693).
- doverctl clean only enters top directories with two alphanumeric
  characters, so `md5-32\1` layouts are not cleaned (util/doverctl.c:475-476).
- A 256-character group name fits MAXGNAME but not `oh_Gname` (255 +
  NUL), so GetOverInfo would loop on iter (dreaderd/group.c:1093,
  dreaderd/group.c:1111-1117).
- `dexpireover -o -n` still renames dexpover.dat to .bak
  (util/dexpireover.c:2530).
- In-code comment says dreaderd locks data files at offset 4; it does
  not (util/dexpireover.c:28-31, dreaderd/group.c:1663-1669).
