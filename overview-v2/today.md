# Diablo reader overview: the design today

Read-only map of how dreaderd stores and serves overview, written as input for a redesign
(F-08, F-09). Source: jrehmer/diablo-nntp `upstream/main` at **051b4157** (merge of #404),
read from `/diablo/test/sweep`. Every `file:line` below is at that commit. Nothing was built or
run (guard G1). Claims marked *(code reading)* are inferred from the code, not observed on a
running server.

---

## 1. On-disk layout

### 1.1 Directory and file names

- Root: `path_group`, default `%s/spool/group` (`lib/global.c:83`), held by dreaderd as
  `MyGroupHome` (`dreaderd/main.c:577`).
- Each group has **one index file** (`over.*`) and **one data file per block of article
  numbers** (`data.*`). Header comment: `dreaderd/group.c:1-21`.
- The block size is `oh_DataEntries`, a power of 2, default `OD_HARTS` = 256
  (`lib/defs.h:496`), set per group with the `e` option of dexpire.ctl
  (`lib/expire.c:131-138`, `samples/dexpire.ctl` "eDATAENTRIES"). A new value only takes effect
  after `dexpireover -R` rewrites the group. The data file holds the articles
  `artBase .. artBase+DataEntries-1`, where `artBase = artNo & ~(DataEntries-1)`
  (`dreaderd/group.c:1660`).
- Names come from `GFName()` (`lib/hash.c:384-540`), using the `readergrouphash` method
  (`lib/config.c:288-289`; the default is crc, `lib/config.c:119-123`):
  - **crc (default):** `<xx>/over.0.<h1>.<h2>[.<iter>]` and
    `<xx>/data.<artBase>.<h1>.<h2>[.<iter>]`. Here `h1.h2` is `hhash(groupname)` printed as
    `%08x.%08x` (`lib/hash.c:347-350`), and `xx` is the low byte of h1, which gives
    **256 subdirectories** (`lib/hash.c:404-428`). The index always has `artBase` 0.
  - **md5-32 / md5-64:** `<dirs>/<b32|b64 md5>.<iter>.o.32.0` and `.d.32.<artBase>`, with a
    configurable directory fan-out (`/N` levels or `\N` characters) (`lib/hash.c:431-509`).
  - **hierarchy:** `comp/sys/…/o.<artBase>.<group>` (`lib/hash.c:510-531`).
  - Sample advice: md5-32/128/128 for a full feed (`samples/diablo.config:938-940`). Changing
    the method needs the server down and `doverctl convert` (`samples/diablo.config:933-936`,
    `man/doverctl.8`).
- **Group-name hash collisions** are resolved by `iter`. `GetOverInfo()` reads the group name
  stored in the index header; if it differs, it increments `iter`, retries
  (`dreaderd/group.c:1134-1141`) and stores `ITER` in dactive.kp (`:1142-1145`). dexpireover
  and doverctl find the group of a file through a hash table of every active group
  (`util/dexpireover.c:1407-1504`). `ExtractGroupHashInfo()` parses all of the file-name
  formats ("This is pure evil because we have too many overview file formats",
  `lib/hash.c:632-640`).
- `.gz` siblings (`data.….gz`) are made by `dexpireover -z` (section 4).
- Temporary files `.data.*` / `.d.*` are left by an interrupted `-R` and deleted on the next
  run (`util/dexpireover.c:893-903`).

### 1.2 Index file `over.*` (fixed-size, circular)

`OverHead`, then `oh_MaxArts` fixed-size `OverArt` slots (`dreaderd/defs.h:690-746`).

| struct | field | type | meaning |
|---|---|---|---|
| OverHead (union, 276 bytes: `v` is 4 ints + `char[256]` + int; `u` is `int[32]` = 128) | `oh_Version` | int | 4 (`OH_VERSION`, `defs.h:715`) |
| | `oh_ByteOrder` | int | `0xF1E2D3C4`; a mismatch is refused, not converted (`group.c:1067-1071`) |
| | `oh_HeadSize` | int | `sizeof(oh)` (`group.c:1114`) |
| | `oh_MaxArts` | int | number of slots (dexpireover `-s`/`-R` changes it) |
| | `oh_Gname` | char[256] | group name (collision check) |
| | `oh_DataEntries` | int | articles per data file |
| OverArt (32 bytes with 4-byte int; ISSUES F-09 says "~40") | `oa_ArtNo` | int | low 31 bits of the article number; **-1 = cancelled, -2 = expired** (`group.c:1645`, `util/dexpireover.c:733`) |
| | `oa_SeekPos` | int | offset in the data file (32-bit signed), -1 = no data (trackonly) |
| | `oa_Bytes` | int | length of the stored header block (without the NUL) |
| | `oa_MsgHash` | hash_t (2 x int32) | `hhash(Message-ID)` (`group.c:709,843`) |
| | `oa_ArtSize` | int | article size, for the OVER `:bytes` field |
| | `oa_TimeRcvd` | int | arrival time (32-bit `time_t`) |
| | `oa_Lines` | int | body lines + 1, 0 = unknown (D-174; the former spare field, `defs.h:745-749`) |

- **Number to slot:** `slot = (artNo & 0x7FFF…) % ov_MaxArts`, at offset
  `oh_HeadSize + slot*32` (`group.c:1173-1192`). A slot is valid for `artNo` only when
  `OA_ARTNOEQ(artNo, oa_ArtNo)`, which compares the low 31 bits (`defs.h:733-736`). Nothing
  else maps numbers to records: there is no B-tree and no sparse index.
- `ov_MaxArts` is derived from the file size (`group.c:1150`); `oh_MaxArts` is the header copy
  that GROUP uses (`nntp.c:392-395`).
- New index size: `a` option of dexpire.ctl, default 512 (`lib/expire.c:34`,
  `DEFARTSINGROUP`, `defs.h:41`). So a fresh index is 276 + 512x32 = 16,660 bytes,
  preallocated with `posix_fallocate`/`ftruncate` and fsync'ed (`group.c:1104-1124`,
  `dreaderd/filealloc.c:70-93`).
- Because the index is a circular slot table, **its size is the group's retention window**
  whenever it is smaller than the range NB..NE. When more articles arrive than there are slots,
  older slots are overwritten (section 5).

### 1.3 Data file `data.*`

- An append-only list of NUL-terminated header blocks "in no particular order"
  (`group.c:13-14`). Each record is the article as dreaderd buffered it: **the whole header
  block plus the blank line** (the body only for control messages and POSTs,
  `dreaderd/feed.c:338-367`). When the feed gave no acceptable Xref, an `Xref:` line that
  dreaderd builds itself is inserted before the blank line (`group.c:758-782`). Then a NUL
  (`iov_len = artLen + 1`, `group.c:754-756,780`).
- The index slot points at the record (`oa_SeekPos`, `oa_Bytes`). Readers check that a NUL
  comes before and after the record (`group.c:1296-1310`).
- Records of a cancelled or expired article stay in the data file until the whole file falls
  below NB and is deleted, or until `dexpireover -R` rewrites it (section 4).

### 1.4 Per-group state in dactive.kp

The group record in the shared text KPDB `dactive.kp` (`lib/kpdb.c:1-48`) holds the fields
`NB` (low), `NE` (high, written by dreaderd), `NX` (feeder side), `LMTS` (last article time,
hex), `CTS`, `S` (flags) and `ITER` (`samples/diablo.config:208-219`, `group.c:504-511`). The
article-number high-water mark lives **only** here, not in the overview files.

### 1.5 How many files a real spool has (estimate from the code)

- Files = sum over groups of `1 + ceil((NE − NB + 1) / DataEntries)` (plus `.gz` copies).
  Data files are made per 256-number block, not per article.
- **An index file exists for every group that was ever selected**, not only for groups with
  articles: `SelectGroup()` (GROUP/LISTGROUP) calls `GetOverInfo()` (`nntp.c:392`), which opens
  with `O_CREAT` and initialises the file (`group.c:1015,1104-1124`). The same is true of
  cancels (`group.c:1520-1523`, comment) and of `dcancel -r` (`util/dcancel.c:719-905`, a copy
  of that code).
- Worked estimate from ISSUES numbers (not measured here): Jesse's archive zone has about
  244 GB of uncompressed header data (`ISSUES.md:1551`), at about 1.2 KB per entry
  (`ISSUES.md:1573-1578`). That is roughly 2x10^8 (group, article) entries, so about 800,000
  data files at e=256, plus about 30,000 index files (the F-09 box has "about 30,000 groups",
  `ISSUES.md:1580`). With the crc layout that is about 3,000 files in each of the 256
  directories.

---

## 2. Write path

### 2.1 Who writes and when

- **dreaderd writes overview itself, inline, in the process that accepts the feed
  connection.** No separate writer process exists. A feed (IHAVE/TAKETHIS from the feeder's
  `dnewslink -H`, usually `MODE HEADFEED`, `dreaderd/feed.c:53-61`) goes to a feed-only fork
  (`readerfeedforks`, default 2, `lib/config.c:134`; forked at `dreaderd/main.c:1093-1109`;
  routed at `main.c:3191-3200`). A connection with feed and read rights may also land in a
  reader fork (`readerforks`, default 10, `config.c:132`). Each fork is a single-threaded event
  loop serving up to `readerthreads` connections (default 40, `config.c:133`).
- When the terminating "." arrives, `NNAcceptArticle()` calls `NNFeedOverview()`
  (`feed.c:265-273`) synchronously, before the 235/239 answer.

### 2.2 Steps for one article (`NNFeedOverview`, `group.c:180-466`)

1. Scan the headers for Newsgroups, Message-ID, Subject, Date, Xref, Control, Supersedes and
   Approved (`group.c:208-256`). Refuse the article when one of them is missing, on a
   header-feed article without `Bytes:`, or when the Message-ID is in the cancel cache
   (`:282-306`).
2. **Pass 1, for every group in Newsgroups:** `AssignArticleNo()` (`:479-640`). It takes the
   KPDB record lock (creating the group with `readerautoaddtoactive`, `:494-521`), drops
   unapproved posts to moderated groups (`:539-560`), and takes `artNo = NE+1` (`:569`). In
   `readerxrefslavehost` mode it takes the number from the feed's Xref instead (`:571-581`;
   without that mode the feed's Xref is stripped on input, `feed.c:326-335,451-478`). It then
   writes `LMTS` (when the second changed), `NB` if needed and `NE`, and unlocks (`:616-631`).
   **NE is committed before any overview is written.**
3. Execute Supersedes (`:393-400`).
4. **Pass 2, for every group:** `WriteOverview()` (`:644-903`):
   - `GetOverInfo(group)` (`:930-1171`): looks in the per-process cache. On a miss it builds
     the path, mkdirs, does `open(O_RDWR|O_CREAT)`, takes a **blocking shared fcntl lock on
     offset 4** (`:1032`), fstat, reads the header (re-initialises a short or new file under an
     exclusive lock, `:1083-1129`), and **mmaps the whole index read-only** (`:1151`).
   - `MakeOverHFile(ov, artNo, 1)` (`:1658-1853`) opens or creates the data file of the block.
   - Takes an **exclusive fcntl lock on offset 0 of the data file** (`:738`), `lseek(END)`
     (`:749`), preallocates (Linux `fallocate KEEP_SIZE` in 16-256 KB steps,
     `filealloc.c:40-66`), and appends the record with `writev` (`:818-822`, `OverWriteV`
     `:118-167`).
   - Takes an **exclusive fcntl lock on offset 0 of the index** (`:831`) and `pwrite`s the
     32-byte slot (`:833-862`). On failure it truncates the data append back off (`:866`).
   - Unlocks both, writes one log line per group ("XXX We should find some way to aggregate
     these", `:673`).
5. Execute the control message, if any (`:443-454`).

**Cost per article and group, cache hit** *(code reading)*: about 6 fcntl lock calls (2 on
dactive.kp, 4 on overview), one or two in-place writes to dactive.kp, one `lseek`, one
`writev` (about 1 KB, the full header block), one `pwrite` (32 bytes), and occasionally
`fallocate`. **No fsync on the article path**; fsync happens only when an index is created
(`:1110,1119`). A crosspost to N groups costs all of this N times, including N copies of the
header block (F-08).

**Cost on a cache miss:** the per-process cache holds `OV_CACHE_MAX` = **8 groups**
(`defs.h:42`, eviction `group.c:956-978`). Each group keeps a list of open data-file
descriptors, trimmed once it has more than `readerthreads` entries (`:1831-1835`). A feed
covering thousands of groups therefore pays open + lock + fstat + read + mmap of an index, and
an open of a data file, for most articles *(code reading)*. Feed-only forks also flush the
cache every 300 s, reader forks every 30 s (`reader.c:304-310,336-354`).

**Errors:** a failure in any group answers `400 Error writing data` and **terminates the feed
connection** (`:715-722,727-734,739-746,871-878,886-894`). NE has already been advanced in every group
(pass 1), and the groups before the failing one keep their entries.

### 2.3 Concurrency

- **Writer vs writer** (several feed forks, or a feed in a reader fork): serialised per file by
  the fcntl locks above, and per group by the dactive.kp record lock during number assignment.
- **Reader vs writer:** readers take no lock to read. They read slots through the shared index
  mmap and data through windowed mmaps (section 3). They rely on the order "data first, then
  slot" and on the NUL guard checks. The design needs a unified buffer cache: on OpenBSD,
  readers do not see `pwrite`s through their maps (D-171, `ISSUES.md:1152-1153`, won't fix).
- **dexpireover vs dreaderd:** every dreaderd that has a group in its cache holds the shared
  lock on offset 4 of its index. Resize and rewrite need the exclusive lock (section 4). While
  dexpireover holds it, **`GetOverInfo()` in dreaderd blocks in `F_SETLKW`** (`group.c:1032`;
  `lib/lock.c:17-31`). That stalls the whole fork and every client it serves until the group
  is done *(code reading)*. Slot changes by dexpireover take the offset-0 lock per slot
  (`util/dexpireover.c:719-738`).
- The dexpireover header says data files carry an offset-4 lock from dreaderd
  (`util/dexpireover.c:28-31`). The code takes none: data files are only locked at offset 0
  while appending (`group.c:738`). The comment is stale.

---

## 3. Read path

All reads start with `GetOverInfo()` (which may create the index, section 1.5) and resolve an
article number to a slot with `GetOverArt()`.

`GetOverRecord()` (`group.c:1195-1311`) does the following:
- checks the slot (number match, `oa_Bytes ≤ OVER_HMAPSIZE/2` = 512 KB, `lib/vendor.h:293`);
- applies the `l` age limit of dexpire.ctl (`:1211-1219`) and the NEWNEWS time (`:1221`);
- maps a 1 MB window (`OVER_HMAPSIZE`, aligned to 64 KB, `group.c:33,1254-1287`) of the data
  file, re-mapping when the record is outside the current window;
- checks the NUL guards.

With `plen == NULL` it answers from the slot alone (`:1231-1232`). The batched
`OutputOverRange()` (`:1350-1505`) is compiled only with `USE_OVER_MADVISE`, which is off
(`lib/vendor.h:277-283`), so ranges go one record at a time.

| Command | Code | Cost *(code reading)* |
|---|---|---|
| GROUP | `nntp.c:180-205`, `SelectGroup` `:367-417` | KPDB read of NB/NE, `GetOverInfo` (open/create + mmap on a miss), clamps NB to `NE − oh_MaxArts`; count = NE−NB+1 (an estimate, not a real count) |
| XOVER/OVER range, XZVER | `xover.c:274-470,541-608` | one `GetOverRecord` per number in the range, holes included; at most 10,000 records per pass (`xover.c:30,575-578`) and until the output buffer is full. For each record `OutputOverview()` (`xover.c:674-974`) scans the whole stored header block once per OVER field |
| HDR/XHDR range | same | same; **any header** works because the full header block is stored. LIST HEADERS answers `:` (`list.c:389-408`). `:lines` comes from `oa_Lines` (`xover.c:720-741`); `:bytes` has no case in `OutputOverview` |
| XPAT | `xover.c:231-248`, `:776-806` | as HDR plus a wildmat per record; 50 records per pass (`xover.c:579-582`) |
| OVER/HDR by Message-ID | `xover.c:362-397`, `FindMsgInGroup` `group.c:1582-1621` | **linear scan of the current group's slots, newest first, up to 100,000** (`READER_MSGID_SCAN`, `xover.c:15`), comparing `oa_MsgHash` only (D-211). On a miss HDR falls back to an async `HEAD` from the spool (D-193/#331); OVER answers 430 |
| LISTGROUP | `list.c:589-707` | one `GetOverRecord` per number, **with `plen` set, so each listed number maps and checks its data record** (`list.c:697`), though only the number is printed |
| ARTICLE/HEAD/BODY/STAT by number | `NNArticleRetrieveByArtNo` `nntp.c:601-683`, `NNRetrieveHead` `group.c:1908-1980` | fetches the record and scans it for Message-ID. **HEAD is served from the overview data** (`DumpOVHeaders`, `nntp.c:642-644,728-792`); BODY/ARTICLE then fetch by Message-ID from the reader cache or a spool server (`server.c:1308-1398`). STAT by number also reads the data record |
| NEXT/LAST | `nntp.c:449-529` | walks numbers one at a time, a full `NNRetrieveHead` each, until a valid record |
| ARTICLE by Message-ID | `server.c:1308-1398` | **does not use overview**: reader cache file named by `hhash(msgid)` (`cache.c:1-20`, D-211), else a spool server |
| NEWNEWS | `xover.c:43-160` | matches the wildmat against the active list, then **scans NB..NE of every matching group**, filtering on `oa_TimeRcvd` (`group.c:1221`); output is Message-ID only |
| LIST OVERVIEW.FMT | `list.c:517-522` | fixed `OVERVIEW_FMT` (`defs.h:39`): Subject, From, Date, Message-ID, References, Bytes, Lines, `Xref:full` |

Article numbering for clients: `XNUMBERING rfc3977|rfc977|window` (`nntp.c:51-79`,
`subs.c:26-125`) maps the 64-bit `artno_t` (`lib/defs.h:168`) into the 31-bit RFC 3977 range
or a sliding window. The store keeps the real 64-bit number (NB/NE text, the data file
`artBase`); the slot keeps only 31 bits.

---

## 4. Expiry and maintenance

### 4.1 dexpireover (`util/dexpireover.c`, `man/dexpireover.8`)

Run daily as `dexpireover -a -l180` and weekly as `dexpireover -a -R -l720`
(`samples/adm/daily.reader:39`, `weekly.reader:28`). `-a` = `-NB -U -s -y`.

Each run:
1. Reads all of dactive.kp into a 1,024-bucket hash table (`:488-508`, `GHSIZE` `:107`).
2. Walks every file under `path_group` (`ScanDirectories` `:797-850`): first all index files
   in a directory, then its data files (`:852-922`). `-pN` (up to 16 forks) splits the work by
   the first character of the top-level directory name (`:120-123,813-818`).
3. For each index (`ProcessOverviewFile` type 1, `:1015-1256`):
   - Takes the offset-4 lock shared, waiting at most `-l` seconds (default 1, `:196`).
   - Refreshes NE and clamps NB to `NE − MaxArts + 1` (`:1099-1116`).
   - **Resize (`-s`/`-R`)** (`:1169-1243`): the slot count grows or shrinks by a third of the
     nearest power of 2, bounded by `i`/`j` (`lib/expire.c:128-136`), and only on "radical"
     changes (`:1145-1223`). `ResizeGroup()` (`:2557-2785`) needs the **exclusive** offset-4 lock, so it fails
     whenever any dreaderd has the group cached ("you must unfortunately kill dreaderd",
     `man/dexpireover.8` `-s`). It copies the slots `NB..NE` into a new array and rewrites the
     file **in place**, "no longer an atomic write" (`:2735-2765`). Shrinking below the range
     moves NB up and drops the older entries (`:2681-2692`).
   - **Expire** (`-y` with `x` days, `-e` spool/history, `-o` list file): maps the whole index
     and visits **every slot** (`ExpireByDays` `:1521-1630`, `ExpireBySpool` `:1638-1745`,
     `ExpireFromFile` `:1752-1858`). Each expired slot costs its own lock + `pread` + `write`
     (sets `oa_ArtNo = -2`, `expireOverArt` `:719-738`). NB then advances over the leading
     slots that no longer match (`:1588-1615`). With no `x` value set, nothing expires by age
     (`lib/expire.c:32`), so the index size is the only retention.
   - **Rewrite (`-R`)** (`RewriteData` `:1860-2394`): under the exclusive lock, copies the live
     records of `NB..NE` into new `.data.*` files (can change `e`), renames them over the old
     ones, then renames the new index over the old one (`:2280-2378`). A slot that does not
     match its number aborts the whole program: "artNo != oa_ArtNo, panic XXXXXXXXXXXX" then
     `exit(1)` (`:1999-2001`).
4. For each data file (type 2, `:1257-1344`): deletes it when its block is wholly below NB or
   beyond NE + DataEntries. With `-z` it gzips files that are old enough (`Z` hours) and not
   among the newest `F` files (`:1306-1341`).
5. Files of groups not in dactive.kp are deleted, unless `-w` was given (`:991-1003`).
6. Writes NB/LMTS/CTS back (`:526-604`). `-O[n]` deletes groups idle for n days by `LMTS`
   (`:549-575`).

### 4.2 Reading compressed data

A `.gz` data file is inflated **whole into a `mkstemp` file in /tmp each time a process opens
it** (`group.c:1694-1810`). Its size is capped at 64 MB uncompressed, read from the gzip
ISIZE (`:1725`). A larger file is "Bad zlen" and its records are unreadable.

### 4.3 Cancels and other per-article changes

- **Cancel control** (`CtlCancel` `control.c:822-940`): for each group in the cancel's
  Newsgroups, `FindCanceledMsg()` scans the slots newest-first, over up to `MaxArts` slots
  under a read-locked active record (`group.c:1515-1574`), matching on `oa_MsgHash` only. Then
  `canLockDecide()` reads the target's headers from the overview data for Cancel-Lock
  (`control.c:790`). `CancelOverArt()` sets `oa_ArtNo = -1` (`group.c:1628-1655`). A target not
  yet present goes to the pre-cancel cache only under `readercanlock all` (`control.c:902-913`).
- **`dcancel -r group:number`**: a copy of `GetOverInfo`/`GetOverArt`/`CancelOverArt` in
  `util/dcancel.c:719-960`.
- **`dgrpctl masscancel GROUP`** (in the usage text, `util/dgrpctl.c:68`; missing from the man
  page): reads Message-IDs on stdin, holds the record lock while it scans the group, and
  compares **`oa->oa_ArtNo == artEnd` unmasked** (`util/dgrpctl.c:198`), so it misses every
  article above 2^31 *(code reading)*.

### 4.4 Groups, numbering and recovery

- **Group creation:** newgroup (control message, dgrpctl) writes the active record only. The
  index is created lazily by the first feed article or GROUP.
- **Group removal:** `CtlRmGroup` deletes the active record only (`control.c:1052-1072`); its
  files go at the next dexpireover run without `-w`. `-O` behaves the same way
  (`man/dexpireover.8`).
- **Renumbering:** no tool renumbers a group. `dgrpctl adjust NB|NE|NX` edits the range
  (`man/dgrpctl.8`). Resize re-slots by number and keeps the numbers.
- **Hash method change:** `doverctl convert srchash dsthash` moves or renames every file,
  offline (`man/doverctl.8`; open items D-150/D-151, `ISSUES.md:1080-1085`). `doverctl clean`
  removes stale files and empty directories.
- **Rebuild from spool:** no overview rebuild tool exists. The spool has to be fed again
  (`dspoolrefeed` + `dnewslink -H`, `man/dspoolrefeed.8:45-62`). Without `readerxrefslavehost`
  the Xref is stripped and **articles get new numbers**, NE+1 (`feed.c:451-478`,
  `group.c:569`). `diloadfromspool` rebuilds only the history.
- **Crash recovery:** there is no journal and no fsync. NE is advanced before the overview is
  written (2.2), so a crash or error leaves holes. A slot can point past the data a crash lost;
  readers catch that with the size check and the NUL guards and log
  "corrupt overview entry"; a record past the end of the file is skipped silently (`group.c:1276-1310`; the "data file is too small" logs are in the compiled-out `OutputOverRange`, `:1415-1441`). A short or
  bad index header is re-initialised **empty** (`group.c:1083-1129`). An interrupted
  dexpireover "will cause, at worst, one newsgroup to lose its overview" (`man/dexpireover.8`,
  INTERRUPTING).
- **Consistency checks:** `dreadover group:art[-end]` (`-f`, `-v`, `-h`, `-H file`, `-a`)
  prints slots and records for debugging (`man/dreadover.8`). No fsck-style verifier exists.

---

## 5. Known pain points, with evidence

**Size and write amplification**
1. **The full header block is stored, once per group** (F-08, `ISSUES.md:1545-1567`;
   `group.c:754-782`). The archive zone has 122 GB on disk (244 GB logical) of overview for a
   276 GB spool; INN's overview for a 5+ TB spool is 202 GB.
2. **Writes are scattered over thousands of files** (F-09, `ISSUES.md:1569-1619`): every
   (article, group) does an append to `data.*` plus a 32-byte `pwrite` in `over.*`. On ZFS the
   pool wrote about 46x the daemon bytes (58 KB per entry); with tuning that came down to
   about 18x. The spool, written sequentially, stays near 1.1x.
3. dactive.kp is written for every (article, group): NE always, LMTS about once per second
   (`group.c:616-631`). It is one shared text file with record locks (`lib/kpdb.c:1-48`).
4. The per-process cache holds 8 groups (`defs.h:42`), so a broad feed reopens and re-mmaps
   indexes and data files continuously *(code reading)*.

**Index design**
5. **A circular index of fixed size is the retention.** A burst larger than `MaxArts` between
   dexpireover runs overwrites older slots, which then vanish from OVER/LISTGROUP/ARTICLE by
   number while their data stays on disk. The samples work around it per hierarchy
   (`samples/dexpire.ctl`: `a2048` for binaries, `i2048`, `j6000`). Resizing happens only in
   dexpireover and only when no dreaderd has the group open (`man/dexpireover.8` `-s`).
6. In xref-slave mode a late article with a number from Xref takes slot `n % MaxArts`, whoever
   is in it (`group.c:571-581,833`) *(code reading)*.
7. **Nothing indexes by Message-ID.** Lookups by ID inside a group are linear scans (up to
   100,000 slots for OVER/HDR, up to MaxArts for each group of a cancel) and trust the 64-bit
   hash alone (D-211, `ISSUES.md:1323-1325`). NEWNEWS scans whole groups.
8. LISTGROUP and STAT by number read the data record when the slot would do
   (`list.c:697`, `nntp.c:613`) *(code reading)*.

**Maintenance**
9. Resize and `-R` need an exclusive lock that every cached reader blocks. A reader or feed
   fork that opens the group meanwhile waits in `F_SETLKW` with all its clients
   (`group.c:1032`). `-R` rewrites every data file of the group while it holds that lock.
10. Expiry walks every slot of every index and costs syscalls per expired slot
    (`util/dexpireover.c:719-738,1557-1582`). Space is freed only per whole data file or by
    the weekly `-R`.
11. gzip'ed data files are inflated whole to /tmp on every open by every process, and are
    unreadable above 64 MB (`group.c:1694-1810`).
12. Code and format duplication: `GetOverInfo`/`GetOverArt` exist three times (dreaderd,
    `util/dcancel.c:719-930`, `util/dgrpctl.c:946-1160`); five file-name formats
    (`lib/hash.c:632-640`).

**Limits** *(from the types)*
13. `oa_SeekPos`/`oa_Bytes` are `int`, so data files are limited to 2 GB and records to
    512 KB served (`OVER_HMAPSIZE/2`, `group.c:1205`; the window is 1 MB, `lib/vendor.h:293`).
14. `oa_TimeRcvd` is a 32-bit `int`, which breaks in 2038.
15. `oa_ArtNo` keeps 31 bits. Lookups mask (`defs.h:733-736`), but `dgrpctl masscancel` does
    not (`util/dgrpctl.c:198`).
16. `oh_ByteOrder` is checked, never converted, so files are not portable between endiannesses
    (`group.c:1067-1071`).
17. No fsync and no commit marker: crash consistency rests on the NUL guards (4.4).

**Robustness**
18. One group's write error drops the whole feed connection after NE has been advanced in all
    groups (2.2).
19. GROUP and cancels create an index for every group they touch (1.5).
20. The design needs mmap coherence with `pwrite`, so it is not supported on OpenBSD (D-171).

---

## 6. What a replacement must keep compatible

**Protocol behaviour (the tests pin most of it)**
- GROUP/LISTGROUP replies (`211 count low high`, count = NE−NB+1) and the NB/NE semantics,
  including the clamp to the retention window (`nntp.c:366-417`).
- OVER/XOVER/XZVER field order and `LIST OVERVIEW.FMT` (`defs.h:39`), including the `Xref:full`
  form, whitespace folding (`xover.c:860-917`), `:bytes` from the feed size + 1
  (`xover.c:928-937`) and `:lines` from `oa_Lines` with the fallback to `Lines:` (D-174).
- HDR/XHDR/XPAT on **any header** (`LIST HEADERS` answers `:`). A smaller store (F-08) needs a
  spool fallback or a configured extra-header list.
- OVER/HDR by Message-ID (D-193, #331): 430 for unknown, 0 or the Message-ID as the first
  field, and the HDR spool fallback.
- **HEAD by number and the header part of ARTICLE by number are served from overview**
  (`nntp.c:642-644`, `cache.c:431-433`, `spool.c:158,420`), with the `vserverdef` Path/Xref
  rewrite (`DumpOVHeaders`). The store must keep the full header block, or those paths must
  move to the spool.
- The reply codes of strict HDR/OVER versus XHDR/XOVER (`xover.c:286-353`), NEWNEWS time
  filtering, the `l` age limit (`group.c:1211-1219`), and the D-195 access checks.
- `XNUMBERING` modes over 64-bit numbers (`subs.c`).

**Article-number stability**
- Numbers are assigned once (NE+1, or from Xref in slave mode) and never renumbered. Readers'
  newsrc files depend on that. A conversion must keep every number. Rebuilding from the spool
  by refeeding does not keep them today (4.4).
- NE/NB live in dactive.kp and are shared with the feeder (NX), dsyncgroups and dgrpctl
  (`samples/diablo.config:208-219`).

**Config directives and files**
- diablo.config: `readerxover on|trackonly` (`config.c:723-735`; trackonly writes slots
  without data, and `readercanlock` needs headers, `samples/diablo.config:760-767`),
  `readergrouphash`, `readerxrefhost`, `readerxrefslavehost`, `readerautoaddtoactive`,
  `readerfeedforks`/`readerforks`/`readerthreads`, `path_group`, `path_dexpire`,
  `path_dexpover_list`.
- dexpire.ctl per-wildmat options `x a i j e l Z F` (`lib/expire.c:121-161`, `samples/dexpire.ctl`).
  `a i j e` describe the slot table and data blocks, `Z F` the gzip step. A new store must
  map, accept or deprecate each of them.
- `vendor.h` switches: `OVER_HMAPSIZE`, `USE_OVER_MADVISE`.

**Tools and options**
- `dexpireover -a -e -f -l -NB -n -O[n] -o -pN -R -s -U -v -w -x -y -z -C -d -V`, run by
  `samples/adm/daily.reader` and `weekly.reader` (the plock name `.dexpireover`).
- `doverctl clean|convert` (`-f -w -v -m -n -C -d -V`), `dreadover` (`-f -v -a -h -H -C -d -V`),
  `dgrpctl groupinfo|listgrouphashfile|masscancel|adjust|rmgroup|newgroup`, `dcancel -r`, and
  `dexpire -o` (writes `dexpover.dat`, `util/dexpire.c:1159-1181,1380`).
- F-08/F-09 both plan the conversion as a `dexpireover -R` step, with the open question of what
  an older dreaderd does with converted groups (`ISSUES.md:1562-1564,1610-1612`).

**History, cancels and control messages**
- **`oa_MsgHash` is the link to the history.** `dexpireover -e` looks each slot's hash up in
  dhistory (`util/dexpireover.c:1691`), and `-o` matches it against the hashes `dexpire -o`
  wrote. D-210 (decided 2026-11-15, `ISSUES.md:1315-1320`) moves every Message-ID hash, the
  overview's included, to a new 62-bit MD5 method chosen by a config option, with a full
  history and overview rebuild on switch. The **group-name** hash (`GFHash`, file names) must
  stay `hhash()`. Any new store has to carry the Message-ID hash in the history's method (or
  the Message-ID itself, which D-211 also needs for its compare).
- Cancels and supersedes find their target by Message-ID within each group named and mark it
  without deleting the number (`oa_ArtNo = -1`). Cancel-Lock checks read the target's
  `Cancel-Lock` from the stored headers (`control.c:790-795`). The pre-cancel cache and the
  `FindCancelCache` refusal on arrival (`group.c:294-306`) sit in front of the store.
- Control messages are stored with their body under `control.<type>` pseudo-groups
  (`group.c:316-334`, `feed.c:338-367`), and pgpverify needs that body.
- `ITER` (group-name collisions) also keys the reader cache scoreboard `cache.hits`
  (`SetCacheHash`, `lib/hash.c:543-563`; `UpdateCacheHits` takes `grpIter`/`endNo`), so a store
  that drops `iter` must keep the cache key stable or empty the cache.

**What any store has to provide** (from the call sites, no opinion on the engine)
- Append (group, number) → {header record or fields, Message-ID hash, size, lines, arrival
  time}, from several processes at once, with readers never seeing a half-written record.
- A point lookup by (group, number); a range scan by number in order; a "does it exist" check
  without reading the record.
- Lookup by (group, Message-ID); today this is a bounded scan, and an index would remove it.
- A time filter over a group (NEWNEWS) and an age limit (`l`).
- Mark cancelled or expired; expire by age, by history hash or by list; move NB.
- Bulk delete of a group; tolerance of numbers arriving out of order (xref slave); no
  capacity limit tied to a slot count.
- Crash consistency that never exposes a number without its record, and tools to dump,
  verify and convert.
