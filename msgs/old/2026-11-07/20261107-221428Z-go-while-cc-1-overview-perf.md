From: go-while/cc-1 (opus-5.5)
To: jesse/cc-1, Jesse
Re: 20261107-221428Z-go-while-cc-1-overview-review.md

[Claude] Overview review part: performance (main 9320a42).

# dreaderd overview subsystem: performance review

Source: upstream main `9320a42` (read-only worktree `/diablo/test/ovrev`). Measurements ran on a
separate worktree `/diablo/test/ovperf` (same commit; prototypes were reverted after measuring).
Date: 2026-11-07. Scope: performance and scalability, plus design where it affects them.
Bugs and docs are left to the sibling reviews; a few bugs found along the way are listed in §8.

Harness, microbenchmarks, raw logs and the prototype diff are in `/diablo/test/overview-perf-bench/`.

---

## 0. Summary

| # | Hot spot | Measured effect | Fix (v7-safe?) |
|---|---|---|---|
| 1 | `OV_CACHE_MAX` is **8** OverInfo entries per process (`dreaderd/defs.h:42`). A feed or reader that touches more than 8 groups reopens and remaps the index and data files on almost every article or GROUP | Feed to 2000 groups: **1128 art/s → 7477 art/s (6.6×)** with the cache at 3000. 5-group crossposts: **377 → 1747 art/s (4.6×)**. GROUP sweep: **478 → 60 µs per GROUP (8×)** | Make it configurable, default ~1024, with LRU eviction (yes, but see #4) |
| 2 | `OutputOverview()` scans every stored header once per OVER field (8 scans per line), and calls vsnprintf for single characters (`dreaderd/xover.c:700`, `:796`, `:896`) | OVER uses 12 µs CPU per line, where XHDR uses 1.4 µs. A prototype that indexes the headers once: **CPU 2.3× lower, wall 1.42 s → 0.84 s for 100k lines (70k → 120k lines/s)** | Single-pass index for multi-field modes (yes) |
| 3 | `readerdetaillog` defaults to on (`lib/config.c:147`), which writes **3 syslog lines per fed article** (`LogCmd`, `dreaderd/reader.c:50`) | Single-group feed **7449 → 9749 art/s (+31%)** with it off | Default it off for feed connections, or log one line per article (yes) |
| 4 | dexpireover resize waits `LockWaitTime` (1 s) for **every** group that some dreaderd process has cached (`util/dexpireover.c:2247`) | With a large cache, a live `dexpireover -a` took **72 s for 540 groups**: 71 groups each timed out after 1 s | Must ship together with #1: try-lock, then retry or defer (yes) |
| 5 | Reader commands (GROUP, NEWNEWS, XOVER) **create** an index file for a group that never had an article, and fsync it twice (`group.c:992`, `:1087`, `:1096`) | **4.5–4.8 ms per GROUP** on an empty group; 500 GROUPs created 500 files (8.3 MB) | Open without O_CREAT on read paths and treat a missing file as empty (yes) |
| 6 | Index slots form a fixed ring: `slot = artno % maxArts` (`group.c:1153`). The default InitArts is 512, and nothing resizes the ring until dexpireover runs | 2000 articles fed into a new group: **only 512 are visible** (`211 513 1488 2000`); the other 1488 records were overwritten | Warn, and grow the ring online (after v7) |
| 7 | Every write on ZFS costs 5–9 µs, and a fed article makes 4 small writes plus one append (KPDB NE, index slot, rtstatus, LMTS, data append) | Per-article sys time on ZFS is 72 µs; on tmpfs 24 µs. The feed reaches 22k art/s on tmpfs and 9.7k art/s on ZFS | Store into mmap instead of pwrite, update LMTS less often, store rtstatus via mmap (partly) |
| 8 | Data files hold 256 articles each (`OD_HARTS`, `lib/defs.h:493`) | A 100k-article group needs 391 files; e=4096 needs 25. Random single-article OVER **232 → 120 µs**, `-R` 1.76 → 0.85 s | Raise the default `e` (yes, it only affects new data files) |
| 9 | dexpireover `-a` resizes groups back and forth on every run when a group's article count c is in (0.444·P, 0.5·P), where P is the next power of two above maxArts (`dexpireover.c:1037–1039`) | Observed 4096 → 2731 → 4096 on consecutive runs (2000 articles) | Make the hysteresis bands overlap correctly (yes) |
| 10 | The data files store **full headers**, not overview lines | 111.8 MB stored for 33 MB of OVER output (3.4×): 3.4× more page cache and parsing per OVER | Store pre-formatted overview lines (after v7) |

Gains from #1+#3+#5 alone are large and low-risk. Two of them were prototyped and measured.
The prototype for #1 and #2 passes the existing e2e reader/overview tests. The command was
`go test -run 'Over|Reader|Hdr|XPat|Xhdr|XHDR|D193|Lines|Xover|NewNews|Wildmat'` → ok.

---

## 1. Method (repeatable)

* Host: Linux 7.0 (pve), 12 CPUs, 62 GB RAM. Spool on **ZFS** (`tank0/diablo/test`:
  lz4, recordsize 128K, sync=standard, atime=on). Comparison runs used tmpfs (`/dev/shm`).
  Other agents' dreaderd test sites were running on the box at the same time, so expect ±10% noise.
* Build: `./configure && make -j8` at 9320a42 (zlib and PAM on, `USE_OVER_MADVISE` off: the
  default).
* Harness: `overview-perf-bench/zz_ovperf_test.go`. Copy it into `tests/e2e/` and run, e.g.:
  ```
  cd tests/e2e
  OVPERF=1 OVPERF_N=100000 OVPERF_CFG=readerdetaillog=off TMPDIR=/diablo/test/ovtmp \
    DIABLO_BIN=$PWD/../../dbin go test -count=1 -run TestOvPerfSingle -v .
  OVPERF=1 OVPERF_G=2000 OVPERF_K=20 OVPERF_CFG=readerdetaillog=off TMPDIR=/diablo/test/ovtmp \
    DIABLO_BIN=$PWD/../../dbin go test -count=1 -run TestOvPerfMany -v .
  ```
  Environment variables: `OVPERF_E` (dexpire.ctl `e`), `OVPERF_DEXP` (whole dexpire.ctl line),
  `OVPERF_STRACE=dir` (runs dreaderd under `strace -T -ff`), and `OVPERF_EXPIRE_LIVE=1` (runs
  dexpireover while dreaderd is still up).
* The site is `NewReaderSite` (readerforks 1, readerthreads 4, readerfeedforks 1, dreaderd `-d`).
  The feed is **MODE STREAM + pipelined TAKETHIS** from 127.0.0.2 (the feed access).
  Articles have realistic headers: 20 lines, about 1.1 KB, 0–3 References, a 5-line body.
* CPU time per process comes from `/proc/<pid>/stat` before and after each phase. Syscall
  sequences come from `strace -T -ff` in separate runs, so tracing overhead is not in the timings.
* Note: `-d` makes `logit` also write to stderr, an extra `write(2)` per log line that a
  production daemon does not do. The headline feed numbers use `readerdetaillog=off`, which
  removes those lines.

## 2. Baseline numbers (stock 9320a42)

### 2.1 Feed (write path)

| Scenario (ZFS unless noted) | art/s | CPU per article (user / sys) |
|---|---|---|
| 1 group, 100k articles, readerdetaillog **on** (default) | **7 449** | 39 µs / 92 µs |
| 1 group, 100k, readerdetaillog off | **9 749** | 25 µs / 72 µs |
| 1 group, 100k, readerdetaillog off, **tmpfs** | 22 126 | 21 µs / 24 µs |
| 1 group, 100k, off, `e4096` | 11 284 | 25 µs / 63 µs |
| 8 groups round-robin, 10k | 8 512 | 32 µs / 81 µs |
| **9** groups round-robin, 10k | 6 359 | 36 µs / 116 µs |
| 2000 groups, first article each (creates the index files) | 208 | most of the time is spent in fsync |
| **2000 groups round-robin, steady, 40k** | **1 128** | 65 µs / **347 µs** |
| crosspost ×5 random groups (of 2000), 8k | **377** (1 885 group-writes/s) | 235 µs / 1.19 ms |

The feed fork is CPU-bound in every case (wall time ≈ user+sys of the single feed fork).

### 2.2 Syscalls per fed article (strace, one group, steady state)

Order of syscalls for one article, with the code that issues each:

```
read(sock)                      ~0.36 per article (4 KB reads)
getpid,getpid,write(2),sendto(syslog)   x3   LogCmd (readerdetaillog) + -d stderr
pwrite(rtstatus, 70B)           StatusUpdate -> RTStatusUpdate (reader.c:1611)
fcntl(kpdb, F_WRLCK)            AssignArticleNo KPDBReadRecord(KP_LOCK)   group.c:504
[pwrite(kpdb, LMTS 8B)]         when the second changed                    group.c:600
pwrite(kpdb, NE 10B)            KPDBWrite(...KP_UNLOCK)                   group.c:609
fcntl(kpdb, F_UNLCK)
fcntl(data, F_WRLCK)                                                       group.c:715
lseek(data, 0, SEEK_END)                                                   group.c:726
[fallocate(data, KEEP_SIZE)]    every 16..256 KB                          group.c:797
writev(data, 3 iov)             headers + built Xref + NUL                 group.c:799
fcntl(over, F_WRLCK)                                                       group.c:808
pwrite(over, 32B)               the OverArt slot                           group.c:839
fcntl(over, F_UNLCK) ; fcntl(data, F_UNLCK)                                group.c:857,874
stat(".accesslock")             access-cache check per command             dns.c:4710
write(sock, "239 ...")          + pselect6 per command
```

That is about 17 syscalls, or 26 with the default logging and `-d`, and **five or six of them
are small writes** to four files. In the 20k-article trace these took: 120k fcntl, 60k pwrite64,
80.9k write, 60k sendto, 20k writev, 20k lseek and 20k newfstatat. On ZFS such a write costs
5–9 µs when the file is mmap'd elsewhere, against 0.6 µs on tmpfs (microbench
`overview-perf-bench/sysbench.c`):

| op | ZFS | ZFS, file mmap'd | tmpfs |
|---|---|---|---|
| fcntl lock+unlock pair | 1.3 µs | – | 1.3 µs |
| pwrite 32 B, sequential (index) | 6.5 µs | **9.2 µs** | 0.6 µs |
| pwrite 10 B, same offset (KPDB) | 5.4 µs | **7.4 µs** | 0.6 µs |
| lseek + writev 1120 B append | 7.6 µs | – | 1.6 µs |

**Multi-group feed (stock):** with more than 8 groups each article also runs the
`GetOverInfo()` miss path: newfstatat(dir), `openat(over, O_RDWR|O_CREAT)`, fcntl(RDLCK @4),
fstat, read(276), fstat, **mmap(index)**, madvise, `openat(data)`. Then, every 4 misses, the
eviction frees 4 entries, each with munmap, fcntl(UNLCK), close(over) and close(data)
(`group.c:933–954`, `:1829–1850`). The KPDB record is also read once more with a lock
(`group.c:969`). Microbench `mbench2.c` (ZFS, 2000 files, 20k appends + index writes):

| | reopen per article (stock behaviour) | everything kept open + mapped |
|---|---|---|
| 8 groups | 36–56 µs wall | 20 µs |
| **2000 groups** | **750–930 µs wall / 310 µs sys** | **45 µs / 43 µs sys** |
| 2000 groups, tmpfs | 15–24 µs | 7 µs |

On ZFS, closing and reopening files that were just written is the expensive part (not the
mmap itself). This matches the 347 µs sys per article measured in the 2000-group feed.

### 2.3 Read path (100k articles in one group, ZFS, warm page cache unless noted)

| Command | lines/s | MB/s | reader CPU (user / sys) |
|---|---|---|---|
| `OVER 1-100000`, first run after the feed | 40 395 | 13.3 | 1.30 s / 0.64 s |
| `OVER 1-100000`, warm | **67 444 – 73 766** | 22–24 | **1.16–1.24 s** / 0.11–0.21 s |
| `XHDR Subject 1-100000` | 345 546 – 623 774 | 30–55 | 0.14 s / 0.12 s |
| `HDR Message-ID 1-100000` | 387 341 – 574 681 | 27–40 | 0.14 s / 0.10 s |
| `XPAT Subject 1-100000 *thread 1234*` | 100k records scanned in 150–200 ms | – | 0.13 s / 0.06 s |
| `XHDR Injection-Info` (a late header) | 124k – 258k | 20–42 | 0.36 s / 0.16 s |
| `OVER n-999 – n` (typical client refresh), per command | 12.6 ms | – | – |
| `OVER <random single>` round trip, e256 / e4096 | 232 µs / 120 µs | – | – |
| GROUP, 2000 groups sweep, stock / cache 3000 | **478 µs** / **60 µs** per GROUP | – | 225 µs / 75 µs sys |
| GROUP on a never-fed group (creates the file) | **4.5–4.8 ms** per GROUP | – | – |
| GROUP+XOVER all, 2000 groups (~45 articles each) | 3.1–3.6 ms per group | – | – |

OVER uses **12 µs CPU per line**, XHDR of a single field 1.4 µs. Syscalls during OVER are few:
for 20k lines (6.6 MB) the trace shows 1796 writes, 897 pselect, and 79 openat/mmap/munmap/close
(one set per data file). Output goes out in **7 KB chunks**, because `MBUF_HIWAT` is
4096·2−1024 (`dreaderd/defs.h:617-618`, `BIG_MBUF` off by default), and each chunk costs one
pselect and two writes. XPAT returns to the select loop every 50 records (`xover.c:540`).

### 2.4 Expire (dexpireover, dreaderd stopped unless noted)

| Run | wall | CPU (user / sys) |
|---|---|---|
| `-a`, 1 group, 100k articles, nothing to expire | 36 ms | 0.01 / 0.03 |
| `-R` (rewrite data), 1 group, 113 MB in 391 data files | **1.76 s** | 0.06 / 1.70 |
| `-R`, same data in 25 data files (e4096) | 0.85 s | 0.05 / 0.80 |
| `-a` expiring all 100k articles (391 data files deleted) | 1.15 s | 0.04 / 1.05 |
| `-a`, 2500 groups (4576 files) | 2.53 s | 0.14 / 0.95–1.10 |
| `-R`, 2500 groups | 3.15–3.21 s | 0.21 / 2.74–2.82 |
| **`-a` with dreaderd live and a large OverInfo cache, 540 groups** | **72 s** | 71 groups × 1 s lock timeouts |

### 2.5 Storage

| | files | apparent | allocated (lz4) |
|---|---|---|---|
| 1 group, 100k articles, e256 | 1 over + **391 data** | 111.8 MB | 20.5 MB |
| 1 group, 100k, e4096 | 1 + 25 | 111.8 MB | 19.1 MB |
| 2000 groups, ~45 articles each | 2000 over + 2076 data | 149.7 MB | 26.9 MB |
| +500 never-fed groups after a GROUP on each | +500 over | +8.3 MB | +0.6 MB |

There is one level of 256 directories, keyed by the low byte of hhash h1 (`lib/hash.c:417`).
With the 42 785 real names of the i2pn2.org active file, all 256 directories are used, with at
most 595 groups in one (`hdist.c`). Synthetic names that share a long prefix (`ov.many.gNNNNN`)
all hashed into one directory. That is harmless for real data, but test results with such
names are skewed.

---

## 3. Write path analysis

**Per article and group:** the KPDB record is locked, read, and written (NE, sometimes LMTS)
in `AssignArticleNo` (`group.c:457–619`). Then `WriteOverview` (`group.c:622–880`) does:

* `GetOverInfo` → a cache hit or a full open (see 2.2),
* `MakeOverHFile(create)`, cached in `ov_HCache` (`group.c:1656`),
* a data lock (fcntl), `lseek(END)`, an occasional fallocate, `writev`,
* an index lock, `pwrite(32 B)`, then unlocking both.

There is **no fsync or msync** on the hot path. The two fsyncs are only in index-file creation
(`group.c:1087`, `:1096`); each costs about 2.2 ms on this pool (600 fsyncs = 1.30 s in the
300-group trace).

* **Files touched by a crossposted article:** for N groups, N KPDB records (one shared file),
  N data files and N index files. The headers are stored **N times** (write amplification N×,
  the same as INN tradindexed). The Xref string is rebuilt for every group
  (`group.c:735–757`, O(N²) for an N-group crosspost), and the start of the body is located by
  scanning the whole buffer again for each group (`group.c:660–677`). Both are negligible for
  N ≤ 10, and only matter for spam-sized crossposts.
* **Locks:** there are 4 fcntl lock/unlock pairs per article and group: KPDB record, data,
  index, and the data unlock. That is cheap (1.3 µs per pair), but it serializes multiple
  `readerfeedforks` writing the same group. Assigning each group to one feed fork would remove
  the data and index locks entirely. That only matters once the rest is fast.
* **Page cache:** index files are mmap'd `PROT_READ, MAP_SHARED` by every process
  (`group.c:1128`) with `MADV_SEQUENTIAL` on the whole index (`:1135`). That is wrong for
  random slot access and harmless for small files. Writers go through `pwrite`, which on ZFS
  must also update the mmap'd pages (9.2 µs against 6.5 µs unmapped). Data files are appended
  with writev and read through 1 MB mmap windows.
* **On ZFS** each small `pwrite` dirties a whole record (the file size up to 128 K). In a
  many-group feed, every txg rewrites one index record and one data tail record per touched
  group. Ops advice: use a dataset with `recordsize=16K`, `atime=off` for `spool/group`.
* **LMTS:** with many groups, almost every article also writes LMTS (`group.c:595–602`),
  because the group's timestamp is older than one second. LMTS is only read by
  `dexpireover -O` and `dgrpctl` (day granularity).

## 4. Read path analysis

* **OVER/XOVER**: `NNListOverviewRange` (`xover.c:495`) loops over the range, filling the output
  buffer up to `MBUF_HIWAT` (7 KB). Per record, `GetOverRecord` (`group.c:1172`) reads the slot
  from the mmap'd index and `MakeOverHFile` (cached) returns the data file. The window is
  remapped only when a record falls outside the current 1 MB window (`group.c:1231–1266`). With
  e256 a data file is about 280 KB, so there is one map per data file. That is fine.
  **The cost is `OutputOverview`** (`xover.c:630`):
  * For each of the 8 fields in `OVERVIEW_FMT` (`dreaderd/defs.h:39`), the loop at
    `xover.c:700` walks the stored header block byte by byte, finding each header's end
    including continuation lines, until the name matches. **Bytes: is never a stored header**,
    so that field always scans the whole record; Xref is near the end. That is about 8 × 1.1 KB
    of byte-wise scanning per line.
  * Single characters go through `MBPrintf` → `vsnprintf` (`xover.c:796` for the field
    separator, `:896` for an empty field), and the index goes through `MBPrintf("%lld")`
    (`:624`). That is about 9 vsnprintf calls per line.
  * **Prototype** (`prototype-cache-and-singlepass.diff`): index the headers once
    (name, length, lowercase first character), then look the fields up in that index, and
    replace the one-character `MBPrintf` calls with `MBWrite`. OVER CPU went 1.16–1.24 s →
    0.48–0.53 s per 100k lines, wall 1.42 s → 0.84 s, and `OVER last 1000` 12.6 → 5.6 ms. It
    must be applied only to multi-field modes: with the index built unconditionally,
    single-field XHDR Subject got slower (0.14 → 0.38 s), because the old code stops at the
    first match.
* **HDR/XHDR/XPAT** do scan the overview data. Each record is the full stored header block, so
  any header can be asked for, and the cost is proportional to the position of the header in
  the block (Injection-Info, late in the block: 0.36 s per 100k; Subject: 0.14 s). XPAT copies
  the header into a buffer and runs wildmat on every record (`xover.c` COM_XPAT case), and
  yields every 50 records. A message-id form scans up to 100 000 index slots newest-first
  (`READER_MSGID_SCAN`, `FindMsgInGroup`, `group.c:1559`). That reads only the 32-byte slots,
  so it is cheap.
* **GROUP** (`SelectGroup`, `nntp.c:378–393`) reads the KPDB record, **then calls
  `GetOverInfo()`** only to clamp NB to NE−maxArts. With the 8-entry cache, a client that walks
  its subscription list pays an open, lock, read and mmap per GROUP, and frees 4 other entries
  every 4 misses: 478 µs against 60 µs per GROUP. For a group with **no index file yet**, the
  same call creates one: `O_RDWR|O_CREAT` (`group.c:992`), `FileAllocSpace`, **two fsyncs**,
  and on ZFS 4.5–4.8 ms. NEWNEWS over `*` does the same for every group
  (`xover.c` NNListNewNews → NNListOverviewRange → GetOverInfo).
* **Descriptors and mmaps per process:** at most `OV_CACHE_MAX` (8) OverInfo entries. Each
  holds 1 index fd and mapping, plus up to `ReaderThreads`+2 data fds, each with up to 1 MB
  mapped (`group.c:1804`). For readerthreads 4 that is at most 8 × (1 + 6) = 56 fds and
  ≤ 8 × 6 MB of mappings. A reader with **more active groups than the cache** thrashes. A
  reader of 50 random groups with 40 threads thrashes all the time. Data files are opened
  `O_RDWR` even by readers (`group.c:1669`).
* **Thousands of groups:** every operation on a group outside the 8 cached ones (feed, GROUP,
  OVER, NEWNEWS, cancel) reopens that group's files. `FlushOverCache()` drops every
  unreferenced entry every 30 s in reader forks and every 300 s in feed forks
  (`reader.c:296–300`, `:339–342`). The reason is to release the shared locks dexpireover needs
  (see §5).

## 5. Expire analysis (`util/dexpireover.c`)

* **Algorithm:** a directory walk (`ScanDirectories`) calls `ProcessOverviewFile` per file
  (`:851`).
  * **For an index file:** open and read the header, take the KPDB record lock, then
    `GetOverExpire` (a linear wildmat over dexpire.ctl), then an optional resize, then
    `ExpireByDays` (`:1337`).
  * `ExpireByDays` mmaps the whole index and scans **all maxArts slots**: O(slots), not
    O(live articles). Each expired slot costs an **lseek+write of 32 bytes**
    (`:1395–1397`), without any lock against the feed writing the same slot.
  * **For a data file:** a KPDB record lock, then `getDataEntries()`, which opens, reads and
    closes the group's index **once per data file** (`:1128`, `:2614`). Then the file is deleted
    if it lies wholly outside NB..NE. Space is reclaimed **one data file (256 articles) at a
    time**, without a rewrite. That is the cheap part of the design.
  * Total cost is O(total slots + total data files). Measured: 2.5 s for 2500 groups and
    4576 files (≈ 1 ms per group). Extrapolated: about 40–60 s for 40k groups. Expiring 100k
    articles in one group took 1.15 s, mostly in sys (391 unlinks plus per-file KPDB and
    index reads).
* **`-R` (rewrite data)** rewrites every live record of every group into new data files and a
  new index, then renames (`:1689–2130`). Measured about 64–130 MB/s, sys-bound. For a 1 TB
  overview spool that is hours. Keep `-R` as an occasional repair tool, not routine.
* **Resize (`-s`, part of `-a`):**
  * **Hysteresis bug:** if count < maxArts/2 the ring shrinks to maxArts − P/3, and if count >
    2/3·maxArts it grows to maxArts + P/3, where P = nearestPower(maxArts) (`:1036–1041`).
    After a shrink from P to 2P/3, the grow threshold is 4P/9. So any group with a count in
    (0.444 P, 0.5 P) flips on **every** run: observed 4096 → 2731 → 4096 for 2000 articles.
    Each flip is a full index rewrite under an exclusive lock.
  * **Locking against live daemons:** the resize needs `XLOCK_EX` on offset 4. Every dreaderd
    process holds `XLOCK_SH` there for as long as it caches the group (`group.c:1009`). The
    lock attempt is blocking with `alarm(LockWaitTime=1)` (`:2245–2257`, `:1722–1726`), so
    **each busy group costs 1 s**. With stock caches that is ≤ 8 groups per fork, but with any
    larger cache the run blows up (measured: 72 s for 71 busy groups). The same applies to
    `-R`.
* **Impact on live readers and feeders:** expiry writes slots under nobody's lock (benign
  except in the ring-wrap race). Deleting data files is safe because readers keep their open
  fds. Resize and rewrite skip busy groups, so in practice **hot groups are rarely resized**,
  and they are the ones that need it most (see #6, data loss).

## 6. Storage layout

* **Per group:** 1 index file `over.<iter>.<hash>` (fixed 32-byte `OverArt` slots,
  `dreaderd/defs.h:732`, plus a 276-byte header), and **ceil(articles / e) data files**
  `data.<artBase>.<hash>`. With the default e=256 a busy group of 10M articles needs about
  39 000 data files, and a 1-billion-record spool about 4M files. Each data file is a list of
  NUL-terminated full header blocks, about 1.1–3 KB each in practice.
* **Fixed ring = data loss:** the slot is `artno % maxArts` (`group.c:1153`). WriteOverview
  writes without checking whether it overwrites a live slot. A new group starts with InitArts
  512. **Measured:** 2000 articles in a fresh group → `211 513 1488 2000`, and OVER returns 512
  lines. The other 1488 headers stay in the data files as dead bytes until their file expires.
  SelectGroup and dexpireover silently move NB up (`nntp.c:393`, `dexpireover.c:951–960`).
  Growth only happens in the next `dexpireover -s` run, and only if the group is not busy (§5).
* **Space waste:** the index is about 32 B × maxArts (negligible). In the data files, dead
  entries (ring overwrites, cancels, records of expired articles inside a file that still holds
  live ones) stay until the whole file expires or `-R` runs. On average that is half a data
  file per group plus the ring overwrites.
* **Small files:** a fresh index is 16.6 KB with fallocate. 40k groups → 660 MB of index if
  readers or NEWNEWS create them all (#5). On ext4, 391 data files of ~280 KB each for a 100k
  group is fine. The problem is the file *count* and the open/close churn (§2.2), not the
  per-file overhead.
* **Fragmentation:** `FilePreAllocSpace` uses `FALLOC_FL_KEEP_SIZE` in steps of 16–256 KB
  (`filealloc.c`). The rounding `& ~(preallocsz-1)` assumes a power of two, but `prealloc_ov`
  and `prealloc_oh` (`group.c:702–703`) are not powers of two. On ZFS, fallocate is a no-op or
  unsupported.
* **Full headers against overview lines:** 111.8 MB stored for 33 MB of OVER output (3.4×).
  OVER reads 3.4× more memory than it sends. In exchange, XHDR and XPAT work for any header.

## 7. Proposals

Ranked by gain/effort/risk. "v7" means a small, contained change that can ship in v7; the rest
goes after v7.

### 7.1 v7-safe

1. **Configurable, larger OverInfo cache with LRU** (`dreaderd/defs.h:42`, `group.c:933–954`).
   * Add a `readerovercache N` option. Default about 1024 for feed forks and about 256 for
     reader forks (fd budget: FD_SETSIZE is 4096, `lib/vendor.h:360`).
   * Evict least-recently-used entries with `ov_Refs == 0`, one at a time, instead of "free half
     of the buckets in round-robin order".
   * Add an idle timeout (e.g. drop entries unused for 10 s) to replace the blanket
     `FlushOverCache()` every 30 s/300 s.
   * **Gain measured** with OV_CACHE_MAX=3000: 2000-group feed 6.6×, crossposts 4.6×, GROUP 8×.
   * Effort: small. Risk: fd usage, and the dexpireover interaction, so it **must** come with 2.
2. **dexpireover: never block on busy groups** (`dexpireover.c:2245–2257`, `:1722–1726`). Use a
   non-blocking `F_SETLK` (or `XLOCK_NB`), put busy groups on a retry list, and retry them at the
   end of the run (or after one `-l` wait for all of them together). This turns 1 s per busy
   group into ~0. Effort: small. Risk: low.
3. **Feed logging:** make `readerdetaillog` default to off for feed connections, or log the
   whole article as one line (accepted, groups:artnos) instead of three (`reader.c:50`,
   `group.c:654`, `:858`). **Measured +31%** single-group feed throughput. It also takes a lot
   of load off the syslog daemon: 3 messages per article × 10k art/s. Effort: trivial. Risk:
   operators who parse these logs.
4. **Single-pass `OutputOverview` for multi-field modes** (`xover.c:630–929`): index the headers
   once, look fields up by length and first character, and replace the one-character
   `MBPrintf` calls with `MBWrite`. The prototype passes the reader e2e tests. **Measured:**
   OVER CPU 2.3× lower, wall 1.7× faster. Keep the early-exit scan for single-field
   XHDR/HDR/XPAT. Effort: small. Risk: low (output identical; the e2e tests cover it).
5. **Do not create overview files from read paths** (`group.c:992`, `:1085–1096`). Add a
   `create` flag to `GetOverInfo()`: feed and post pass 1, while GROUP, OVER, NEWNEWS and cancel
   lookup pass 0. When the file is missing, return NULL; SelectGroup already falls back to
   `DEFMAXARTSINGROUP`, `nntp.c:397`. Also drop the two fsyncs on creation: a fresh index
   carries no data that needs to be durable. **Measured:** 4.5–4.8 ms per GROUP on an empty
   group, and the file it creates. Effort: small. Risk: low.
6. **Fix the resize hysteresis** (`dexpireover.c:1036–1059`): grow only when count > 0.75·M and
   shrink only when count < 0.33·M, then choose a target that lands the count at about 50–60%.
   Or keep the formula and require `|new−old| > old/3` and a shrink target ≥ 1.5·count. This
   stops the full index rewrite on every run for groups in the (0.444 P, 0.5 P) band.
   Effort: trivial.
7. **Raise the default data entries per file** (`OD_HARTS` 256 → 4096, `lib/defs.h:493`, the
   `e` default in `lib/expire.c:38`). Measured: 16× fewer files, random OVER 232 → 120 µs,
   `-R` 2× faster, feed +16% (noisy). Existing groups keep their value from the index header
   (`oh_DataEntries`). The trade-off is coarser space reclaim: up to 4096 articles' headers per
   file, about 4.5–12 MB. Keep OVER_HMAPSIZE (1 MB) as it is. Effort: trivial. Risk: low. Ship
   it as a documented default change.
8. **Cheaper per-command bookkeeping:** update LMTS at most once per 60 s per group
   (`group.c:595–602`); it is only read at day granularity. Rate-limit the `.accesslock` stat to
   once per second (`dns.c:4710`). Write the rtstatus slot through a shared mmap instead of a
   70-byte `pwrite` per command (`rtstatus.c`). Each removes about 1 small ZFS write (5–9 µs)
   per article or command. Effort: small.
9. **Bigger output batches:** enable `BIG_MBUF` by default, or raise MBUF_HIWAT to about 64 KB
   (`dreaderd/defs.h:614–622`). Let XPAT yield every N bytes instead of every 50 records
   (`xover.c:540`). That means about 10× fewer select/write rounds on large OVER/XPAT. Effort:
   trivial. Risk: memory per connection (64 KB × readerthreads).
10. **Readers open data files `O_RDONLY`** (`group.c:1669`), and the whole index gets
    `MADV_RANDOM` (or no advice) instead of `MADV_SEQUENTIAL` (`group.c:1135`). The gain is
    small. It is mainly hygiene, and it makes a read-only spool possible.

### 7.2 After v7 (redesign)

11. **Index writes through a writable shared mmap** instead of `pwrite` (index slot, KPDB NE).
    This removes 2 of the 4–5 small ZFS writes per article (about 15 µs sys, roughly 20% of
    what remains). Ordering is "append the data, then store the slot", with release ordering
    in the store; readers already validate slot ↔ record. The OpenBSD mmap/write coherence
    question has to be solved first, so this cannot be the only code path.
12. **Pre-formatted overview records** (as INN tradindexed `.DAT` and ovdb/ovsqlite do): store
    the tab-separated OVER line (plus the :bytes/:lines metadata) at feed time. OVER becomes a
    memcpy from the mmap: expected ≥ 5× less CPU than now, ~2× better than proposal 4, and 3.4×
    less page cache. Keep full headers only for non-overview XHDR/XPAT, as an optional second
    file per data file or a fall back to the spool, as INN does. This is an on-disk format
    change, so it needs `OH_VERSION` 5 and a dexpireover `-R` migration.
13. **Growable index instead of a ring:** a base article number plus a linear array, like INN
    tradindexed's `.IDX`. Grow it online when NE − base reaches capacity (the writer does this
    under the index lock with an `iter++` rename; readers already reopen when `st_nlink == 0`,
    `group.c:1017`; add a cheap generation check on cache hits). That **removes the silent
    loss** (#6) and the dependence on dexpireover `-s`, and resize no longer competes with live
    readers.
14. **Fewer files for many-group servers:** either INN buffindexed-style shared block buffers
    (no per-group files at all, constant fd count), or a single ordered store keyed by
    (group-id, artnum) with batched commits, as in ovsqlite or an LSM/B-tree. ovsqlite gets its
    insert rate by committing many articles per transaction. Diablo's feed fork could likewise
    batch the index and KPDB updates of all articles in one select pass. Either removes the
    open/close churn (§2.2) by design, and makes expire O(expired) instead of O(slots + files).
15. **Group affinity for feed forks** (hash group → fork): no cross-process locks on data and
    index files, and each fork's cache only holds its own share of groups.
16. **64-bit fields:** `od_ArtBase` and `ov_endNo` are `int` but hold `artno_t`
    (`dreaderd/defs.h:748`, `:765`). `oa_SeekPos` is `int` (`:734`), which limits a data file to
    2 GB (fine at e≤4096). Fix this in the format change of 12/13.

## 8. Bugs noticed along the way (for the bug review)

* `od_ArtBase` is `int` (`dreaderd/defs.h:748`) and is compared with a 64-bit `artBase`
  (`group.c:1656–1659`), so it truncates above 2^31.
* `ExpireByDays` writes slots without the index lock, so it can lose a slot the feed rewrote
  after a ring wrap (`dexpireover.c:1395`).
* `FileAllocSpace`: `posix_fallocate` returns an error number, not −1, so the `e < 0` test never
  disables it (`filealloc.c`). `FilePreAllocSpace` rounds with a power-of-two mask on sizes that
  are not powers of two.
* GROUP reports a count of 513 when 512 records are visible after a ring wrap (`211 513 1488
  2000`). This is an off-by-one in the `artEnd − maxArts` clamp (`nntp.c:393`).
* A control message's whole body ends up in the `control.*` data file (the body is buffered for
  pgpverify and written with `artLen+1`, `group.c:731`).

## 9. Comparison notes (only where they suggest a concrete change)

* **INN tradindexed:** `.IDX` with fixed entries indexed by artnum − base, growing (no ring),
  plus `.DAT` with **pre-formatted overview lines**. The group index is shared and mmap'd (like
  KPDB). Expire rewrites the group. This is the model for 12 and 13.
* **INN buffindexed:** large preallocated buffers shared by all groups, so there are no
  per-group files. It is the answer to §2.2's file churn for servers with 50k+ groups (14).
* **INN ovdb / ovsqlite:** one keyed store with transactions. ovsqlite runs a server process
  that batches inserts per commit. This is the model for batching index updates per select pass
  (14), and its expire is O(expired rows).
