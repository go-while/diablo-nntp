# Diablo reader overview: design proposal

For go-while first, then Jesse. Date 2026-11-16. Inputs: `overview-design-today.md` (today's design,
cites at upstream/main **051b4157**), `overview-pugleaf-sqlite.md` (go-pugleaf's stores),
ISSUES F-08/F-09, and the code at 051b4157 (read with `git show`). Nothing was built or run
(guard G1). Every number marked *(estimate)* or *(guess)* is mine and has to be measured (section 8).
**Status: analysis for a later decision; no overview change is planned now (owner, 2026-11-16).**
"today §n" means a section of `overview-design-today.md`; "pugleaf §An" one of the pugleaf report.

---

## 0. Summary

- **Data model for every option (the owner's requirement):** store each article **once**
  (an article record keyed by an article id, found by Message-ID hash plus a compare of the
  stored Message-ID, D-211), and per group only `article number -> article id`. The article record
  keeps the **overview fields as columns**. The rest of the header block is either a
  **compressed blob in the same record** (default) or is **not stored** and comes from the spool
  server on demand (opt-in, INN-like). Estimated size per article: today `c x 1.23 KB`
  (c = groups per article), new `~1.05 KB + c x 25 B` with the blob, `~0.6 KB + c x 25 B` without it.
- **Store options:** A per-group SQLite files; **A' one SQLite file written only by a dedicated
  overview writer process**; B SQLite sharded into N files, feed forks write directly; C a custom
  append-only log plus dense per-group index; D MariaDB/InnoDB.
- **Concurrency options** (section 3): **E a separate overview server process** that owns the
  store and answers requests over a Unix socket, which dreaderd's event loop waits on like any
  other socket (no threads in dreaderd); or a worker-thread pool inside each reader process.
- **Recommendation: A' served by E**: one SQLite file; the overview server is its only writer
  (batched transactions) and serves the reads. It is the only SQLite layout where an article and
  all its group entries commit in one transaction (store-once with exact reference counts, numbers
  without holes, batching across all groups, no lock contention, three files), and E keeps dreaderd
  single-threaded (and, once phase 4 is done, free of SQLite). Final only after the benchmark in
  section 8.
- **Phases** (section 7, the owner's order): (1) measure; (2) one internal overview API in front of
  today's code, no behaviour change; (3) the new store behind it, with a verify tool, written
  synchronously as today; (4) concurrency: the overview server (or threads). Each step small and
  shippable on its own.
- **Threads** (section 10): moving dreaderd from forks to threads is possible but large
  (~150 file-scope and ~56 function-static variables in dreaderd, KPDB's per-process fcntl locks,
  `select()`/`FD_SETSIZE`, ~175 `strerror` and ~68 `strtok` calls; *guess* 4-6 months, phased). It
  is independent of the store choice; a threaded dreaderd could host the A' writer as a thread.

---

## 1. Goals and non-goals

### 1.1 Goals (measurable; thresholds are proposals for the owner to change)

| # | Goal | Measure | Today |
|---|---|---|---|
| G1 | **Crash safety.** No acknowledged (235/239) article loses its overview after a process crash; no number ever shows a partial or foreign record; no number is reused. After an OS crash/power loss: same, except commits newer than the last sync when `sync=normal` (they become missing numbers, never corrupt ones). | kill -9 test (section 8, S4): 0 lost acked, 0 partial, 0 reused | NE advanced first, no fsync, slot may point past data (today §4.4) |
| G2 | **Hole-free numbering** for the store-once options with one commit point (A', C, D-hole-free): a number is published in dactive.kp only after its record committed. | S4: 0 numbers in NB..NE without a record, except cancels/expiry | crash or write error leaves holes (today §2.2) |
| G3 | **Write cost per article** | CPU µs/article, bytes written to the device per (article,group) entry, fsyncs/s | ~6 fcntl, 2 KPDB writes, writev 1.2 KB + pwrite 32 B per group; on ZFS 23-58 KB written per entry (F-09) |
| G4 | **Read latency** for OVER 1000, HDR (overview field and other header), XPAT, LISTGROUP, HEAD by number, OVER by Message-ID | p50/p99/max per command, warm and cold | one record per number, holes included; by-ID scan up to 100,000 slots (today §3) |
| G5 | **No reader stalls.** The longest single store call made from a reader's event loop | ≤ 20 ms at p99.9, ≤ 100 ms max under ingest + expiry (*proposal*) | `F_SETLKW` wait on dexpireover resize/-R can block a fork for a whole group (today §2.3) |
| G6 | **Expiry cost and space reclaim** | time and max writer stall to expire one day; space reused by new articles; shrink possible | visits every slot; frees space per data file or weekly `-R` (today §4.1) |
| G7 | **Retention not bound by a fixed index size** | burst ≫ old index size loses nothing | circular slot table is the retention (today §5.5) |
| G8 | **Store-once / size** | bytes per article on disk vs estimate (section 2.4) | full header per group (F-08: 244 GB logical for 276 GB spool) |
| G9 | **Files and fds** | files on disk; fds per process (readers `select()`, so the process limit is capped at `FD_SETSIZE`, `dreaderd/main.c:2207-2226`) | ~830,000 files on the archive (today §1.5) |
| G10 | **Verifiability** | a verify tool that checks physical and logical consistency | `dreadover` dump only |

### 1.2 Non-goals

- No change to the NNTP behaviour that today §6 lists (GROUP count = NE−NB+1, OVER field order,
  `LIST OVERVIEW.FMT`, reply codes, XNUMBERING, D-193/#331 by-ID behaviour, D-195 checks).
- dactive.kp stays the place of NB/NE (shared with feeder, dsyncgroups, dgrpctl). No KPDB redesign.
- No change to the feeder, the spool or the reader article cache (`cache.c`), except that
  HEAD-by-number may read the spool in the "no blob" mode.
- No full-text search, no threading tables (pugleaf's web tables are not needed, pugleaf §A2).
- No renumbering of existing groups; a conversion keeps every number.
- OpenBSD support is not a goal; if a store fixes D-171 as a side effect, fine.

---

## 2. What all options share

### 2.1 Store once: article record + group references

```
article  : artid (arrival order) | mhash | refcount | rcvd | bytes | lines | msgid
           | Subject | From | Date | References | Xref | extra headers | rest-of-header (blob or none)
groupref : (group id, article number) -> artid
group    : group id <-> group name, high-water mark (hwm)
```

- `mhash`: an internal, fixed 63-bit hash (first 8 bytes of MD5 of the Message-ID, top bit
  cleared). It is **not** the history's `hash` setting: the store keeps the Message-ID text, so every
  lookup compares it (D-211) and a later history-hash switch (D-210) needs no overview rebuild.
  `dexpireover -e/-o` compute the history's hash from the stored Message-ID at expiry time.
- `Xref` is built at write time from the numbers of all groups (today dreaderd builds it too,
  `group.c:758-782`). It doubles as the reverse map article -> (group, number), which cancels and
  `-e` expiry need to find every group entry of an article.
- Group ids replace the group-name hash in file names. `ITER` in dactive.kp is left alone, so the
  reader cache scoreboard key (`SetCacheHash`, today §6) does not change.
- Cancels and supersedes delete the article's group entries (the number stays unused because NE
  never goes back) and drop the article when its refcount reaches 0. Cancel-Lock reads the target's
  `Cancel-Lock` from the stored headers (`control.c:790`); in the no-blob mode `Cancel-Lock` is
  stored as an extra column when `readercanlock` is on.
- Control messages (`control.<type>` pseudo-groups) keep their body inside the blob, as today
  (`feed.c:338-367`).
- `readerxover trackonly` = article record with Message-ID, sizes and time only.

### 2.2 Overview columns and the rest of the header block

Columns: Subject, From, Date, Message-ID, References, `:bytes`, `:lines` (D-174), Xref, arrival
time, plus `readeroverextra` (a configured list of extra headers, like INN's extraoverview; LIST
OVERVIEW.FMT and LIST HEADERS follow it). Values are stored unfolded and cleaned the way
`OutputOverview()` does it today (`xover.c:860-917`), so OVER becomes a copy, not a header scan.

Rest of the header block, one setting `readeroverheaders blob|spool`:

| | `blob` (default) | `spool` |
|---|---|---|
| Stored | zlib of the header block with the column values elided (header names kept as placeholders so HEAD returns the original order). Optionally a preset zlib dictionary. | nothing |
| HEAD / ARTICLE header by number | one row read + inflate; vserverdef Path/Xref rewrite as today (`DumpOVHeaders`, `nntp.c:728-792`) | async `HEAD <msgid>` from the spool server, the path #331 already has for HDR by ID; fails once the spool has expired the article (BODY already fails then) |
| HDR on a non-overview header, range | one inflate per row, chunk of ~100 | one spool HEAD per article: a 1000-range is ~1000 round trips unless pipelined (*guess: seconds*); propose a range limit (e.g. 200) above which 503 |
| XPAT on a non-overview header | as HDR, wildmat per row | as HDR; same limit |
| HDR/XPAT on overview fields | columns only | columns only |
| Cancel-Lock check | from blob | from the extra column |
| Size (2.4) | ~1.05 KB + c x 25 B | ~0.6 KB + c x 25 B |

### 2.3 Article-number assignment: two ways

- **Reserve first (today's order).** Pass 1 takes `NE+1` under the dactive.kp record lock in every
  group (`group.c:479-640`), then the records are written. Numbers are never reused and Xref is
  always right, but a crash or write error between the two leaves numbers without records (missing
  articles, legal in NNTP, never acknowledged). Fits writers without a single commit point (A, B).
- **Publish after commit.** The one writer keeps per-group counters
  `next = max(NE from dactive.kp, group.hwm) + 1`, inserts the article and all its refs and
  `group.hwm` in **one transaction**, commits, then writes `NE = max(NE, hwm)` in dactive.kp and only
  then acknowledges. A crash before the commit loses nothing acknowledged and burns no number; a
  crash after the commit is repaired at writer start (`NE = max(NE, hwm)` for every group). Readers
  never see an NE beyond a committed record. Needs one commit domain for all groups of an article:
  A' (one file), C (one log), D (one InnoDB transaction). It also cuts dactive.kp writes from one
  per (article, group) (today §5.3) to one per group per batch.
- xref-slave mode (numbers from the feed's Xref, `group.c:571-581`) works with both; a number that
  already holds another article is kept and logged instead of overwritten (today the slot is
  overwritten, today §5.6).

### 2.4 Size per article (all *estimates*)

Assumptions: header block 1.2 KB raw (F-08: 244 GB / ~2x10^8 entries); overview columns ~0.45 KB
(*guess*: Subject 60, From 40, Date 31, Message-ID 45, References 200, Xref ~55, numbers 20); rest
~0.75 KB, zlib per record ~0.40 KB (*guess*, small records compress worse than ZFS on big records);
article-row overhead + Message-ID index ~40 B; B-tree fill ~85 % (x1.15); one group ref ~25 B in
SQLite, ~40 B in InnoDB. Today = `c x (1.2 KB + 32 B slot)`.

| per article, raw bytes | c = 1 | c = 2 | c = 5 |
|---|---|---|---|
| Today | 1.23 KB | 2.46 KB | 6.15 KB |
| SQLite store-once + blob (A', B, A) | ~1.05 KB | ~1.07 KB | ~1.15 KB |
| SQLite store-once, `spool` | ~0.59 KB | ~0.61 KB | ~0.69 KB |
| D (InnoDB) + blob | ~1.15 KB | ~1.2 KB | ~1.35 KB |
| C (log, records compressed in 64 KB blocks, *guess* 2.5x on the whole header) | ~0.55 KB | ~0.58 KB | ~0.65 KB |

- On a ZFS dataset with compression, today's text compresses ~2x (122 GB on disk for 244 GB,
  F-08), SQLite pages of text columns maybe ~1.6x (*guess*), a zlib blob not at all. So for
  **articles in one group** the blob variant is about **as big as today on disk** (~0.75 vs
  ~0.62 KB); the gain comes from crossposts and from the `spool` mode. A preset zlib dictionary
  (*guess* 0.3 KB per rest) narrows it; the benchmark must use real header samples.
- The archive zone (2x10^8 entries, c ≈ 2 *guess*: 244 GB / 1.2 KB entries against a 276 GB spool of
  ~10^8 articles at a guessed ~2.7 KB each): today 244 GB raw / 122 GB on ZFS; blob ~107 GB raw /
  ~78 GB on ZFS; spool ~61 GB raw / ~38 GB on ZFS. INN's overview for a 5+ TB spool is 202 GB (F-08);
  not comparable per article without its article count.
- **c is unknown.** The first measurement (section 8, M0) counts it from the archive's dactive.kp
  NE totals versus the history size, or from a sample of Xref lines.

### 2.5 Read locality: the cost of store-once

Group refs are clustered by (group, number); article records by arrival. An OVER range of 1000 is
one range scan of refs plus 1000 point reads of article records spread over the arrival order of all
groups. Recent ranges (the usual newsreader case) are in the page cache; a cold range of an old,
quiet group is up to one random page read per article (*guess*: ~0.1 ms on SSD, ~10 ms on disk).
Today's per-group data files are contiguous per group. This hits A', B, D alike (C keeps per-group
index locality, but the records live in the shared log too). Consequences: overview on SSD for big
archives (as F-09 already advises); readers use a time budget per pass (2.6); if S2-cold fails G5,
the fallback variant copies the overview columns into the group ref (c copies of ~0.45 KB, blob
still once) and is measured alongside.

### 2.6 Rules for readers (today's process model)

- Connections opened after `fork()` in each child (`task()`), never inherited.
- One statement per pass, keyset paging (`artno > last`), `sqlite3_reset` at the end of every pass:
  no read transaction stays open across event-loop iterations (long snapshots block checkpoints).
- A time budget per pass (stop after ~5 ms or 1000 rows, continue on the next pass, the way
  `OVERVIEW_PASS_RECORDS` already bounds a pass, `xover.c:575-582`); `sqlite3_step` returns per row,
  so the stall is bounded by one row fetch.
- `busy_timeout` ≤ 20 ms in readers; on SQLITE_BUSY retry on a later pass, after ~1 s answer
  `403` instead of hanging. Readers never checkpoint (`wal_autocheckpoint=0`).
- GROUP never creates anything (today it creates an index, today §1.5).

---

## 3. Concurrency: an overview server process (E) or worker threads

Today: `readerforks` (default 10) x `readerthreads` (default 40) sessions, each fork one
single-threaded `select()` loop (`dreaderd/reader.c:263`; defaults `lib/config.c:132-134`). Any
blocking store call holds every session of that fork. That model may change; the store must not
depend on it. Both alternatives below share one piece of dreaderd work: the overview commands
become **request + continuation**. The connection waits in a `co_Func` state, the way it already
waits for a spool server (`server.c`), and the loop resumes it when the answer arrives. What they
differ in is where the store call runs.

### 3.1 Option E: a separate overview server process (recommended)

A new program (working name `doverd`), forked and respawned by dreaderd's main process like the
feed and reader forks (`main.c:1097-1109,1186`), or started on its own. It alone opens the store.
Similar in spirit to INN's ovsqlite server (general knowledge, not checked here).

**Protocol** (binary, over a Unix stream socket, `path_run/doverd.sock`; one connection per
dreaderd process, multiplexed):
- Framing: `u32 length | u32 request id | u16 type | u16 flags | payload`. Length-prefixed on
  `SOCK_STREAM`, because illumos/Solaris do not implement `SOCK_SEQPACKET`
  (`dreaderd/main.c:143-148`); nonblocking reads reassemble partial frames (the dnsq lesson).
- Request ids: many sessions of one dreaderd process share its connection; replies may come out of
  order; the id finds the waiting connection.
- Requests: `GROUPINFO`, `OVER range fields`, `HDR range header`, `XPAT range header pattern`,
  `LISTGROUP range`, `STAT/HEAD n`, `NEXT/LAST n`, `BYMSGID msgid [group]`, `STORE article`,
  `CANCEL msgid groups`, `EXPIRE ...` (later), `PING`.
- Big ranges in chunks: a reply is a series of frames of at most ~64 KB with a "more" flag; the
  server sends the next chunk only when dreaderd asks for it (`NEXT id`), after the client's output
  buffer drained. So a slow newsreader never makes the server buffer a whole group, and no read
  transaction is held between chunks (keyset paging from the last number).
- Results are pre-formatted rows (number, fields), so dreaderd only adds the tab/CRLF framing and
  the vserverdef rewrites it does today.

**dreaderd changes.** The overview call sites go behind one internal API first (phase 2; it also
removes the three copies of the index code in `group.c`, `util/dcancel.c:719-960`,
`util/dgrpctl.c:946-1160`, today §5.12); then that API gets an asynchronous client for the server:
open/reconnect the socket, put it in the `select()` set, encode requests, dispatch replies by id,
resume the connection. No threads, no SQLite and no new library in dreaderd: one more socket per
process.

**Latency per request** *(guess, to measure in S6)*: a Unix-socket round trip ~10-30 µs plus one
copy of the reply, on top of the query. For OVER 1000 (~0.5 MB) the copy is small next to the query
and the client's own network time.

**When the server is slow or dies.**
- Per-request timeout (proposal: 2 s for reads, 10 s for stores): a read answers `403` (before the
  status line), a store answers `400` and closes the feed connection so dnewslink requeues.
- A failure in the middle of a multi-line reply, after the `224`/`225` line: close the client
  connection (a truncated list would be silently wrong; pugleaf does the same, pugleaf §A5).
- EOF on the socket: every pending request fails as above; dreaderd reconnects with backoff; the
  main process respawns the server. GROUP keeps working from dactive.kp; ARTICLE by Message-ID never
  needed overview.
- Stores that may have committed before the crash are re-offered by the feeder and recognised as
  duplicates (Message-ID index), so nothing is stored twice and nothing acknowledged is lost.

**Crash safety.** As A' (4.2): the server is the single writer, acknowledges a store only after
the commit, publishes NE after the commit, reconciles NE at start. dreaderd holds no store state, so
a dreaderd crash cannot damage the store.

**Inside the server.** Reads must not wait behind each other either. Choices, all inside a small
new program with no legacy shared state: one writer thread + K reader threads, each with its own
SQLite connection (`SQLITE_OPEN_NOMUTEX`); or, with no threads at all, one writer process + K
pre-forked read-server processes, dreaderd connecting to each and choosing the least busy; or
MariaDB connections behind it (D), so dreaderd never links libmariadb and DB outages are handled in
one place.

**Cost of E.** One more hop and copy per request; one more process to supervise; the protocol and
its tests; the server is a single point of failure for overview (respawned; readers get errors
meanwhile instead of stalls).

### 3.2 Alternative: a worker-thread pool per reader process

The loop keeps every socket; a store request (group id, range, field list, limits) goes to a queue;
2-4 worker threads, each with its **own** SQLite connection (built `SQLITE_THREADSAFE=2`,
`SQLITE_OPEN_NOMUTEX`) or a blocking MariaDB connection, run the query and fill a request-owned
malloc'd buffer; completion is signalled through a pipe whose read end is in the `select()` set
(portable; eventfd is Linux-only); the loop formats the reply into `co_TMBuf`.
- What it touches: the event loop (completion fd, a mutex-protected done queue), the
  OVER/HDR/XPAT/LISTGROUP/HEAD/STAT/NEXT/LAST/by-ID paths in `xover.c`, `list.c`, `nntp.c`
  (request + continuation, as for E), connection teardown while a request is in flight (the request
  owns its buffers; a cancelled request is freed on completion).
- Must stay out of workers (not thread-safe or per-process): `zalloc` pools (`SysMemPool`,
  `co_MemPool`), `logit` (workers return error text, the loop logs), **KPDB** (fcntl record locks
  are per process, so they do not exclude threads of the same process: NB/NE are read in the loop
  before the request), the old backend's overview cache, the reader cache (`cache.c`), spool server
  connections, statistics. `DOpts` is read-only after start (fine). Workers block all signals
  (`pthread_sigmask`) so handlers run in the loop thread. To confirm: reader children never fork or
  exec after start (a fork with live threads copies only the caller).
- New build dependency: `-pthread` on every target; SQLite (or libmariadb) linked into dreaderd.
- The writes still need a single writer for A' (a writer process with its own IPC, 4.2), so the
  pool covers reads only.

### 3.3 E versus the worker pool

| | E: overview server | Worker-thread pool |
|---|---|---|
| dreaderd work | request/continuation + async socket client (one API) | request/continuation + pool, queue, completion pipe |
| Thread safety in dreaderd | none needed | a discipline over 40 years of single-threaded code (pools, logit, KPDB, caches) |
| Store library in dreaderd | none | SQLite or libmariadb |
| Writes | the same server batches them (A' writer built in) | still need a writer process + IPC (A') or direct writes (B, D) |
| Per-request cost | one socket round trip + copy (*guess* 10-30 µs) | a queue hop and a pipe wake-up (*guess* a few µs) |
| fds per dreaderd process | 1 socket | 3 per worker per DB file (A' 12 with 4 workers; B 3N per worker) |
| Failure | server down: overview errors in every fork until respawn | a bad query or crash in a worker takes down its fork |
| Review risk | protocol parsing (council: input parsing) | locks and memory (council `#high`) |
| Effort *(guess)* | protocol + client 2-3 weeks, server 2-3 weeks (it contains the A' writer anyway) | pool 1 week + continuation 1-2 weeks, plus the separate A' writer |

With A' as the store, E costs about the same as the pool plus the writer the pool needs anyway,
and it carries no thread risk in dreaderd. The pool wins only if S6 shows the extra hop matters.

### 3.4 How each store option changes with E or the pool

| Option | With E | With a worker pool |
|---|---|---|
| A | the server holds the open-group cache, fds only in one process; still F-09-scattered writes | worse: open-group cache per worker, 3 fds per open DB per worker, under `FD_SETSIZE` |
| A' | natural fit: the server is the writer and serves reads (threads or read processes) | reads in workers; the writer remains a separate process |
| B | the server serialises writes per shard itself, so B loses its point (direct writers) | N shards x workers x 3 fds (N=16, 4 workers: 192 per process) |
| C | the server is the log owner and the reader (`pread`) | `pread` in workers; the log still needs a single appender process |
| D | the server holds the MariaDB connections; dreaderd needs no libmariadb | blocking Connector/C in workers instead of `_start/_cont` |

---

## 4. Options

### 4.1 Option A: SQLite, one database per group

**Layout.** `path_overview/<xx>/<gid>.db` (+ `-wal`, `-shm` while open). Under the store-once
requirement a per-group file can hold only the refs; the article records must live in a shared
store: `path_overview/art/<mhash % 16>.db` (16 files, like pugleaf's history, pugleaf §B1).

**Schema.** Group file: `CREATE TABLE ref(artno INTEGER PRIMARY KEY, mhash INTEGER, artid INTEGER);`
Article files: `art` as in A' (4.2) without `refs` exactness (see below).

**Write path.** Each feed fork writes directly: article into its art shard (one transaction), then
one transaction per group file. Numbering: reserve first (2.3), so a crash leaves missing numbers.
Visibility before 235/239: all commits done before the reply, as today. Per (article, group) one WAL
commit in a different file: the scattered-file pattern of F-09 again (each commit appends ≥ 1 page
to that group's WAL).

**Reads.** OVER: rowid range in the group file + point reads in up to 16 art files. LISTGROUP and
STAT: the group file alone. By Message-ID: index on `ref.mhash` per group, compare in the art row.
NEWNEWS: scan per group. HEAD by number: ref -> art.

**Expiry/reclaim.** Range delete in the group file; refcount decrement in another file (not atomic:
crash leaks article rows, the verify tool collects them). Group removal = unlink (A's best point).
Per-file `VACUUM` is quick for small groups.

**Crash/verify.** SQLite recovery per file; verify = `quick_check` per file + art/ref cross check.

**Concurrency.** Feed forks contend only per group file and per art shard. Readers: open a group
file per GROUP (open + WAL index + schema, *guess* 0.1-0.5 ms) unless cached.

**fds/files.** 30,000 group files (x3 while open) + 48. Per process an LRU of open groups x 3 fds,
all under `FD_SETSIZE` (1024) with client sockets: a 256-group cache uses 768 fds. Feed forks with a
broad feed reopen constantly (today's 8-group cache problem, today §5.4, moved to SQLite).

**Weak.** Store-once breaks the reason for per-group files (self-contained groups, pugleaf §A1);
F-09 is not fixed; fd budget; numbering holes on crash; non-atomic refcounts. **Dominated by A' and B.**

### 4.2 Option A': SQLite, one file, one dedicated writer process (recommended)

Two modes over the same file and schema, shipped in this order:
- **Direct mode (phase 3):** every feed fork writes, one `BEGIN IMMEDIATE` transaction per
  article, synchronously before 235/239 as today; readers read in-process with bounded passes
  (2.6). The file's write lock serialises the feed forks, so numbers can already be assigned
  inside the transaction and published after the commit (2.3, no holes), and the article and all
  its refs commit together. Cost: no batching, and a feed fork can wait up to its busy timeout
  (~100 ms) on the other feed fork's transaction. Measured in S1 as variant "A' direct".
- **Server mode (phase 4):** the overview server (E, 3.1) is the only writer and batches; feed
  forks hand articles over and get the 235/239 on its acknowledgement; reads go to the server too.
  The write path below describes this mode; the writer process is the server's write half.

**Layout.** `path_overview/overview.db` (+ `-wal`, `-shm`) and `path_overview/FORMAT`. One file by
default; the format allows `N` shards later (a `meta` row) if one file ever proves too big to
check or checkpoint, but then crossposts lose atomicity (see B).

**Schema** (sketch; types are SQLite affinities):
```
CREATE TABLE meta(k TEXT PRIMARY KEY, v TEXT);   -- format 'diablo-overview-1', mhash method, created
CREATE TABLE grp(gid INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE, hwm INTEGER NOT NULL DEFAULT 0);
CREATE TABLE art(
  artid INTEGER PRIMARY KEY,           -- arrival order: inserts go to the right edge
  mhash INTEGER NOT NULL, refs INTEGER NOT NULL, rcvd INTEGER NOT NULL,
  bytes INTEGER, lines INTEGER,
  msgid TEXT NOT NULL, subject TEXT, from_ TEXT, date TEXT, refhdr TEXT, xref TEXT,
  extra TEXT,                          -- readeroverextra values
  rest BLOB);                          -- zlib rest-of-header, NULL in spool mode; large columns last
CREATE INDEX art_mh ON art(mhash);
CREATE TABLE ref(gid INTEGER NOT NULL, artno INTEGER NOT NULL, artid INTEGER NOT NULL,
  PRIMARY KEY(gid, artno)) WITHOUT ROWID;
```
Pragmas: `journal_mode=WAL`, `synchronous=NORMAL` (`readeroversync full` for fsync per batch),
page size to be measured (4-16 KiB), `auto_vacuum` decided by the benchmark.

**Write path.**
1. Feed fork, at the final ".": header checks as today (`group.c:208-306`), then sends one request
   over a socket to the writer: Message-ID, Newsgroups list, Approved flag, the feed's Xref
   (slave mode), header block, bytes, lines, arrival time, control body. Socket: `SOCK_SEQPACKET`
   where it exists, `SOCK_STREAM` with length framing on illumos/Solaris, which do not implement
   SEQPACKET (`dreaderd/main.c:143-148`; the dnsq framing lesson applies: nonblocking partial reads).
   The server listens on `path_run/doverd.sock`; feed forks connect lazily and reconnect.
2. The connection waits (`co_Func` state); the fork keeps serving its other connections. For a
   streaming peer the fork keeps reading the next TAKETHIS of the same connection and sends replies
   in order as acks arrive; dnewslink streams (`util/dnewslink.c:2190`). An IHAVE peer is lock-step,
   so its rate per connection is at most 1 / ack delay.
3. Writer: collects requests for up to W ms (*proposal* 5-10 ms) or B articles (~500), then one
   `BEGIN IMMEDIATE`; per article: duplicate check (`art_mh` + Message-ID compare; a duplicate is
   acknowledged as stored, which makes re-offers after a writer crash idempotent); moderation and
   `readerautoaddtoactive` as in `AssignArticleNo` (`group.c:494-560`); numbers by publish-after-commit
   (2.3); Xref; insert `art` once, `ref` per group, update `grp.hwm`; `COMMIT`; then dactive.kp
   `NE = max(NE, hwm)` (+ LMTS) once per group of the batch; then the acks.
4. Feed fork: on ack 235/239; then supersedes and control messages as today, their overview changes
   (cancel) going to the writer as requests.
- **Visibility before 235/239:** the record is committed and visible before the reply, as today.
  The cost is the ack delay: W + commit (*guess* p99 < 20 ms), not the visibility.
- **Writer dies:** dreaderd main respawns it (as it respawns forks, `main.c:1097-1109,1186`). Feed
  forks see EOF: every unacked request answers `400` and the connection closes, so dnewslink
  requeues (to confirm: dnewslink's 400 handling, `ERR_GOODBYE`, `dnewslink.c:185`); re-offered
  articles that did commit are recognised as duplicates and acknowledged. Readers are unaffected
  (they read the file directly). While no writer runs, feed connections get `400` at once.
- **Writer errors:** SQLITE_FULL/IOERR: roll back the batch, NAK it (400 + requeue), retry with
  backoff; SQLITE_CORRUPT: stop writing, log CRIT, readers continue.
- Checkpoints: done by the writer (it serves no clients, so its pauses only delay acks), `PASSIVE`
  periodically, `TRUNCATE` when the WAL passes a size limit.

**Read paths.**

| Command | Query (per pass, keyset) |
|---|---|
| GROUP | dactive.kp NB/NE as today; no store access |
| OVER/XOVER/XZVER range | `SELECT r.artno, a.subject, a.from_, a.date, a.msgid, a.refhdr, a.bytes, a.lines, a.xref, a.rcvd FROM ref r JOIN art a ON a.artid=r.artid WHERE r.gid=? AND r.artno BETWEEN ? AND ? ORDER BY r.artno LIMIT ?`; `l` age limit on `rcvd` |
| HDR overview field | same, one column |
| HDR other header / XPAT | same with `a.rest` (inflate per row) or the spool (2.2); smaller passes |
| LISTGROUP | `SELECT artno FROM ref WHERE gid=? AND artno BETWEEN ...`: refs only (today reads every data record, `list.c:697`) |
| STAT by number | ref + `a.msgid` |
| HEAD / ARTICLE header by number | ref -> art, inflate `rest`, rebuild the block, `DumpOVHeaders` rewrite; body as today |
| NEXT/LAST | `artno > ? ORDER BY artno LIMIT 1` (today walks number by number, `nntp.c:449-529`) |
| OVER/HDR by Message-ID | `SELECT ... FROM art WHERE mhash=?` + Message-ID compare (D-211), then the ref of the current group; no 100,000 scan; #331's spool fallback unchanged |
| ARTICLE by Message-ID | unchanged (cache/spool, today §3) |
| NEWNEWS | per matching group, refs ⋈ art filtered on `rcvd`, in passes (an index `(gid, rcvd)` is not added unless S2 shows it is needed) |
| Cancel / supersede / `dcancel -r` / masscancel | `art_mh` + compare; refs from `xref`; delete refs, refcount, art (in the writer) |

**Expiry and reclaim.** `dexpireover` is a second writer with short transactions (≤ 2000 refs, its
busy timeout 5 s; the writer's 1 s): per group delete refs below the new NB or older than `x` days
(numbers follow arrival except in slave mode, so a forward scan stops at the first young row and
slave-mode rows are checked one by one); decrement `art.refs`, delete articles at 0. `-e`/`-o`:
per article, hash the stored Message-ID with the history's method. Freed pages are reused by new
articles (steady state: the file does not grow); shrink after a retention cut with
`doverctl vacuum` (`VACUUM INTO` + rename, offline, 2x space) or `incremental_vacuum` if the file was
created with `auto_vacuum=INCREMENTAL`. Later phase: expiry requests handled by the writer itself.

**Crash recovery, verify.** WAL recovery on the first open; writer start reconciles NE (2.3).
`doverctl verify`: `PRAGMA quick_check` (offline, or online with a WAL-size guard because it is one
long read), then logical checks: every ref has its article; `art.refs` = number of refs (by a sorted
pass); `xref` agrees with refs; `grp.hwm ≤ NE`; `mhash` = hash(msgid); per group the missing
numbers in NB..NE (information only). `-repair` fixes refcounts and orphans.

**Concurrency.** One writer, so no writer-writer contention and no busy waits in feed forks; WAL
readers never block the writer and vice versa (pugleaf §A7). dexpireover and tools are the only
other writers.

**fds/files.** 3 files. 3 fds per process (writer, each reader fork, each feed fork's read
connection for Cancel-Lock), 3 per worker with the pool.

**Tools.** See 4.6.

**Weak.**
- One writer is a throughput ceiling (*guess*: SQLite batched inserts reach 10^4-10^5 rows/s;
  pugleaf's history measured ~330,000/s in batches, pugleaf §B6) and a single point of failure for
  ingest (respawned, readers unaffected).
- The feed path becomes asynchronous: the largest code change of this option (`feed.c`,
  `group.c:180-466`).
- One file of ~100 GB on the archive: corruption reaches everything (mitigation: verify tool,
  `sqlite3_backup` copies), a full `integrity_check` takes long (*guess* hours).
- The `art_mh` index takes one random page per insert; once it is larger than RAM every insert
  reads a random page: SSD needed at archive scale. Measured with and without the index.
- Cold OVER reads (2.5).
- WAL needs a local file system (no NFS for `path_overview`).

### 4.3 Option B: SQLite sharded into N files, feed forks write directly

**Layout.** `path_overview/shard.<i>.db`, i = 0..N−1, N fixed at creation (default 16, from the
benchmark). Refs of a group live in shard `hhash(group) % N` (the group-name hash stays `hhash`,
D-210); the article record in shard `mhash % N`. Same tables as A'.

**Write path.** Each feed fork writes directly, synchronously, before 235/239 (no async change):
numbers reserved first in dactive.kp (pass 1 as today), then the article record in its shard (one
transaction), then one transaction per shard holding the article's groups. Order gives the
invariant "never a ref without its article"; a crash can leave an article without refs (orphan,
collected by verify) and missing numbers. `BEGIN IMMEDIATE`, busy timeout ~100 ms (a feed fork
serves up to 40 feed connections), on timeout `400` as today. Crossposts cost 1 + k commits.

**Reads.** As A', but OVER reads refs in one shard and articles in up to N shards (N connections
per process, prepared statements each).

**Expiry.** Per shard; refcount decrements cross files (not atomic: leaks on crash, never dangling).
Shards expire in parallel (`-pN` maps to shards).

**fds/files.** 3N files; 3N fds per process (48 at N=16; 12 processes ≈ 576 in all).

**Weak.** No single commit point: numbering holes on crash, non-atomic refcounts, more code in the
verify tool; each feed fork can wait on a shard lock (G5 for feed connections); N WALs, each commit
of a crosspost touches several files (less batching than A'). **Strong:** no IPC and no async feed
path; parallel writers; smaller files and checkpoints. B is the fallback if A''s writer process is
judged too invasive or too slow.

### 4.4 Option C: custom append-only log plus dense per-group index (no SQLite)

**Layout.**
- `path_overview/log/<seq>.log`: append-only segments (e.g. 256 MB). Record:
  `magic u32 | len u32 | crc32 u32 | flags | rcvd i64 | bytes | lines | mhash u64 | ngroups |
  (gid u32, artno u64) x ngroups | msgid | overview fields | rest (zlib)`. Optionally records
  packed in 64 KB compressed blocks (smaller, but a read inflates a block).
- `path_overview/idx/<xx>/<gid>.<seg>`: dense index segments of S numbers (e.g. 65536), entry
  16 B `{log seq u32, offset u32, len u32, check u32}`; zero = no article. No circular reuse.
- `path_overview/commit`: last committed log position (also a commit record in the log).

**Write path.** The log has one appender, so C needs the A' writer process (same IPC, same async
ack). Per batch: append records, optional `fdatasync`, commit record, then index entries per group
(small `pwrite`s, one per group per batch), then publish NE (hole-free, 2.3). Recovery: truncate the
log after the last commit record, re-apply index entries from the committed tail (idempotent);
index entries that point past the committed end are ignored.

**Reads.** Index entry by `(artno − segbase)`, then `pread` of the record (no shared maps: also
removes the mmap-coherence need behind D-171). By Message-ID: no index unless we build one (a
backward scan of the group's index comparing `mhash` then the Message-ID, bounded as today, or our
own hash table file). NEWNEWS: scan.

**Expiry.** Index segments below NB are unlinked. Log segments can go only when no group still
references any record in them: groups with different retention share segments (an article in a
7-day and a 365-day group pins its segment), so C needs a compactor that copies live records forward
and rewrites their index entries, plus per-segment live counts (rebuildable by a scan).

**fds/files.** Today's 830,000 files become ~30,000 index segments (S=65536) + a few hundred log
segments; per process a small cache of index and log fds.

**Weak.** We write and own a storage engine: commit protocol, recovery, compaction, verify,
by-ID index. That is the opposite of "stable" for v7 unless it is very small. **Strong:** big
sequential writes like the spool (F-09 measured the spool at ~1.1x on ZFS), smallest size with
block compression, no new library, per-group index locality.

### 4.5 Option D: MariaDB / InnoDB

**Layout.** A database `diablo_overview` on a local or remote MariaDB server; tables as A' in
InnoDB:
```
CREATE TABLE grp (gid INT UNSIGNED AUTO_INCREMENT PRIMARY KEY, name VARBINARY(255) NOT NULL UNIQUE,
  hwm BIGINT UNSIGNED NOT NULL DEFAULT 0) ENGINE=InnoDB;
CREATE TABLE art (artid BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, mhash BIGINT UNSIGNED NOT NULL,
  refs SMALLINT UNSIGNED NOT NULL, rcvd BIGINT NOT NULL, bytes INT UNSIGNED, lines INT UNSIGNED,
  msgid VARBINARY(250) NOT NULL, subject VARBINARY(1024), from_ VARBINARY(1024), date_ VARBINARY(128),
  refhdr BLOB, xref VARBINARY(2048), extra BLOB, rest MEDIUMBLOB, KEY (mhash)) ENGINE=InnoDB;
CREATE TABLE ref (gid INT UNSIGNED NOT NULL, artno BIGINT UNSIGNED NOT NULL,
  artid BIGINT UNSIGNED NOT NULL, PRIMARY KEY (gid, artno)) ENGINE=InnoDB;
```
`VARBINARY`/`BLOB` so header bytes are never charset-converted.

**Write path.** Row-level locks allow every feed fork to write at once. Hole-free numbering inside
one transaction: `SELECT gid, hwm FROM grp WHERE gid IN (...) ORDER BY gid FOR UPDATE` (sorted to
avoid deadlocks; InnoDB still reports deadlocks, so retry), insert, update `hwm`, `COMMIT`, then
publish NE in dactive.kp (2.3); a hot group serialises its writers on its `grp` row. Alternatively
reserve first in dactive.kp as today. Batching: multi-row INSERTs in one transaction per feed fork
pass; `innodb_flush_log_at_trx_commit=1` (fsync per group commit) or `2`. The feed fork is an event
loop too, so its queries must be asynchronous (Connector/C `_start/_cont`) or in a writer process
or worker pool; otherwise every round trip (*guess* 20-100 µs on a Unix socket) holds its 40 feed
connections.

**Reads.** Same SQL as A'. In the event loop, MariaDB Connector/C's non-blocking API
(`mysql_options(MYSQL_OPT_NONBLOCK)`, `mysql_real_connect_start/_cont`,
`mysql_real_query_start/_cont`, `mysql_store_result_start/_cont`, `mysql_get_socket`): the loop
waits on the connection's socket like any other fd, so even a cold query does not hold the fork.
One connection runs one query at a time: a few connections per reader process with a request
queue. Oracle's libmysqlclient lacks this API; libmariadb is required. With the worker pool
(section 3) the plain blocking API is enough.

**Expiry/reclaim.** `DELETE ... LIMIT` in chunks; InnoDB reuses freed pages; shrink with
`OPTIMIZE TABLE` (online rebuild, 2x space). Partitions by time do not fit per-group retention.

**Crash/verify.** InnoDB redo log; `CHECK TABLE` + the logical checks of A' in SQL.

**Failure handling.** Server unreachable: readers answer `403` for overview commands, GROUP still
works from dactive.kp, ARTICLE by Message-ID still works (it never used overview); feed connections
get `400` and close, so dnewslink queues back up without loss; reconnect with backoff, non-blocking.
DB down means a degraded reader.

**fds/files.** Sockets only: connections per process (readers few each, feed forks one each).

**Weak.** A second server to install, tune (buffer pool), back up, upgrade and monitor; Diablo has
been self-contained. Every query is a round trip. Memory competes with dreaderd on the same host.
The non-blocking API is a coroutine layer inside Connector/C (*guess*: availability per CPU
architecture to check). **Strong:** many concurrent writers, no one-writer-per-file limit,
asynchronous reads in the event loop without threads, mature recovery and tooling, hole-free
numbering without a writer process.

### 4.6 Tools and config (all options; differences noted)

New code: `lib/overstore.[ch]`, one API used by dreaderd and every tool, with the old format as
backend 1 (removes the three copies of `GetOverInfo`/`GetOverArt`, today §5.12).

| Tool | Change |
|---|---|
| `dexpireover` | same options and daily/weekly scripts; `-y -x -e -o -NB -U -O -w -v -n -pN` map to the store (`-pN`: shards in B, groups split in A'/D); `-s -R -z` and dexpire.ctl `a i j e Z F` are accepted, logged as obsolete, no-ops (or `-R` = vacuum) |
| `doverctl` | `clean` (orphan groups and articles), `convert` (old -> new, new -> old), `verify [-repair]`, `vacuum`, `stats`; hash-method convert obsolete |
| `dreadover` | same CLI, dumps refs/articles from the store; `-H` prints the rebuilt header block |
| `dgrpctl` | `groupinfo` shows gid/hwm/ref count; `listgrouphashfile` shows the store/shard; `masscancel` by Message-ID through the index (fixes the unmasked 31-bit compare, `dgrpctl.c:198`) |
| `dcancel -r group:number` | through the API (writer request in A'/C) |
| `diloadfromspool` | unchanged (history only); new `doverbuild` rebuilds overview from the spool (section 9) |
| `dexpire -o` | unchanged; `dexpireover -o` hashes stored Message-IDs with the history's method |

Config (diablo.config): `readeroverstore old|sqlite|mariadb|log` (default `old` until the switch),
`path_overview`, `readeroverheaders blob|spool`, `readeroverextra <headers>`, `readeroversync
normal|full`, `readeroverbatch <ms>/<n>` (A'/C), `readerovershards <N>` (B), `readeroverdb <dsn>`
(D). Unchanged: `readerxover`, `readerxrefslavehost`, `readerautoaddtoactive`, forks/threads.
`readergrouphash` only names old-format files.

---

## 5. Comparison

| | A per-group | **A' one file + writer** | B N shards | C custom log | D MariaDB |
|---|---|---|---|---|---|
| Store once, exact refcounts | no (cross-file) | **yes** (one txn) | no (cross-file, leaks) | yes (one log) | yes |
| Numbers without holes (G2) | no | **yes** | no | yes | yes |
| Writer contention | per group | **none** | per shard | none | row locks |
| Batching across groups | no | **yes** | per shard | yes | yes |
| Feed path async change | no | **yes** | no | yes | yes (or stall) |
| Reader stall risk in-process | cold reads + opens | cold reads | cold reads | cold reads | none (async API) |
| fds per process | LRU x 3 (hundreds) | **3** | 3N | small cache | sockets |
| Files on disk | ~90k | **3** | 3N | ~30k | server's |
| F-09 write pattern | scattered per group | WAL sequential + checkpoint | N WALs | sequential log + small index | InnoDB redo + pages |
| Size, c=2, blob (2.4) | ~1.1 KB | ~1.07 KB | ~1.07 KB | ~0.58 KB | ~1.2 KB |
| Engine we must maintain | no | no | no | **yes** | no (but a server to run) |
| Extra moving part | no | overview server (E) | no | overview server (E) | DB server (+ E optional) |

E or the worker pool (section 3) applies to every column; with E, dreaderd's reader-stall row
becomes "none" for all of them, because every store call runs outside dreaderd.

---

## 6. Recommendation

**A' served by E: one SQLite file; in phase 3 written directly by the feed forks (one transaction
per article, numbers published after commit); in phase 4 owned by the overview server, which is the
only writer (batched) and serves the reads. Store-once, overview columns, compressed
rest-of-header blob (`readeroverheaders spool` as the opt-in for big archives).**

Reasons, by goal:
- G1/G2/G8: the only SQLite layout with one commit point, so an article and all its refs commit
  together, refcounts are exact and numbers are published only after commit (no holes, no reuse),
  already in direct mode.
- G3: in server mode one writer batches all groups into one transaction; dactive.kp is written
  once per group per batch instead of per (article, group); WAL writes are sequential in one file.
- G5: with E no store call runs in dreaderd at all; in direct mode reads are bounded per pass.
- G7, G9, G10: no slot table; 3 files; 3 fds per process in direct mode, 1 socket with E; SQLite's
  recovery plus a verify tool.
- Stability: SQLite is a widely used, heavily tested engine; we write only the schema, the server
  and the API. dreaderd stays single-threaded (E instead of threads). The precedent is INN's
  ovsqlite (one SQLite file behind a server process; general knowledge, not checked here).
- Small steps: phases 2 and 3 change no process model; the asynchronous parts come only in phase 4.

What would change it:
- S6 shows the server hop costs more than the reads it saves → the worker pool (3.2) with a
  separate writer process.
- The single writer cannot sustain 3x the target ingest (R3) while B or D can → B (if fds and
  cross-file refcounts are acceptable) or D (if the owner accepts a DB server, which E can front).
- Cold reads break G5 inside the server too (its readers queue up) → more server readers, the
  "columns in the ref" variant (2.5), or D.
- Bytes written per entry on ZFS not ≤ 1/3 of today's emulation (R4) while C reaches it → C, with
  its engine risk stated to Jesse.
- Direct mode is enough on the measured load (feed-fork waits small, ingest within R3) → phase 4
  can wait; E is then only for read stalls.
- `spool` vs `blob` default: the owner's size priority and the measured c (M0).

---

## 7. Phases, effort, risks

The owner's order. Efforts are *guesses* for agent work plus review rounds. Every step ships on
its own and is tested on the test hosts.

| Step | Content | Effort | Shippable / testable |
|---|---|---|---|
| **1 Measure** | | | |
| 1a | `ovbench` with `ovb_today`, `ovb_sqlite` (A, A' direct, A' writer, B), real header sample (M0) | 1-2 weeks | report; decision on store and blob/spool |
| 1b | `ovb_log` (C prototype), `ovb_maria` (D), S6 server round trip | 1-2 weeks | report; only if 1a leaves the choice open |
| **2 One internal overview API, no behaviour change** | | | |
| 2a | Forward-compat guard in today's code: `GetOverInfo` refuses `oh_Version > OH_VERSION` (today it logs CRIT and continues, `group.c:1060-1064`; the re-check under the exclusive lock tests byte order and size only, `:1088-1091`); dreaderd refuses an unknown `path_group/FORMAT` | 2-3 days | v7; e2e tests for both refusals |
| 2b | `lib/overstore.[ch]` with the old format as backend 1; dreaderd's read paths on it | 1-2 weeks | full e2e suite unchanged |
| 2c | dreaderd's write path, cancels, and the tools (`dcancel -r`, `dgrpctl`, `dreadover`, `dexpireover`, `doverctl`) on it; the three index-code copies removed; masscancel's 31-bit compare fixed | 1-2 weeks | e2e unchanged + masscancel test above 2^31 |
| **3 New store behind the API, with a verify tool** | | | |
| 3a | SQLite backend (A' schema), `doverctl verify` and `convert` (old -> new, offline), `dreadover` on it; dreaderd untouched | 2-3 weeks | convert a test spool; verify; byte-for-byte compare of OVER/HDR/HEAD old vs new through the API |
| 3b | dreaderd on the new store in direct mode, opt-in `readeroverstore sqlite`: reads with bounded passes, feed forks write one transaction per article, numbers published after commit | 2-3 weeks | e2e suite with the new store; kill -9 crash test |
| 3c | Expiry, cancels/supersedes, `doverbuild`, `convert new old` (rollback), config, man pages | 2-3 weeks | e2e expiry/cancel tests; rollback test |
| 3d | Trial on a test reader (Jesse's box, with the user's and his OK), F-09 measurement again | 2-4 weeks calendar | before/after numbers |
| **4 Concurrency** | | | |
| 4a | Request/continuation for the overview commands in dreaderd, still calling the API synchronously underneath | 1-2 weeks | e2e unchanged |
| 4b | Overview server (E) with the read requests; dreaderd's async client; reconnect/timeout/restart | 3-4 weeks | e2e + stall test + server-kill test |
| 4c | Stores through the server: batching, asynchronous 235/239 in feed forks, duplicate re-offers | 2-3 weeks | crash tests (kill -9 server and forks), ingest benchmark |
| 4d | Default switch decision; old backend kept for conversion and rollback | | release note |

Alternative for 4b/4c: the worker pool (3.2) plus a writer process, if S6 says so. Steps 3 and 4
can overlap after 4a. Total *(guess)* 4-6 months.

Risks:
- Cold-read latency of store-once (2.5); mitigated by bounded passes, the server, SSD, the fallback
  variant.
- SQLite behaviour on illumos ZFS is not measured anywhere (no numbers in pugleaf either, pugleaf
  "Not checked"); the F-09 platform is where it matters.
- Feed-fork waits on the write lock in direct mode (3b), until 4c.
- The asynchronous feed path (4c) is the riskiest change: reply order, connection teardown with
  pending acks, server restart.
- The server protocol parses input from another process: review council scope.
- Migration time and space on the archive (section 9).
- New build dependency (libsqlite3 or a vendored amalgamation) across 20 CI jobs; with E only the
  server and the tools link it after step 4.
- Memory: SQLite page caches per connection x processes; test containers have 1280 MiB + 1 GiB swap.
- `art_mh` random inserts at archive scale (needs SSD).

---

## 8. Measurement plan (test hosts only, through jobq; nothing on this machine)

**Programs** (one C tree, built on the host): `ovbench` driver + backends `ovb_today.c` (today's
pattern: data append + slot `pwrite` + fcntl locks, the baseline), `ovb_sqlite.c` (variants A, A' direct
(P writer processes on one file, one transaction per article), A' writer (one writer, feeders over a
socket, batches), B with N ∈ {4,16,64}; flags blob/spool, `art_mh` on/off, page size, sync
normal/full, columns-in-ref), `ovb_log.c` (C prototype, ~800 lines *guess*), `ovb_maria.c` (D:
blocking and `_start/_cont`), `ovb_server.c` (a minimal E: the framing of 3.1 over a Unix stream
socket in front of `ovb_sqlite`, K reader threads or processes).

**M0, data shape.** Real header blocks are needed for compression numbers (synthetic text
compresses wrongly): a sample of ~100,000 header blocks and the crosspost factor c, from a test
spool or, with the user's and Jesse's OK, from the archive (`dreadover -H` dump).

**Shapes.** Groups G = 30,000 (the F-09 box), Zipf popularity; c from M0 (default 70 % 1, 20 % 2-3,
10 % 4-10, *guess*); dataset larger than RAM (the 1280 MiB container makes a few GB cold).
- S1 ingest: unthrottled and at 200/1000/3000 articles/s; writers P = 1, 2, 4 (`readerfeedforks`
  default 2).
- S2 mixed: ingest at a fixed rate + 10 reader processes each simulating 40 sessions: OVER 1000
  recent 60 %, OVER 1000 old 20 %, HDR Subject 5 %, HDR non-overview 5 %, XPAT 3 %, LISTGROUP 3 %,
  HEAD by number 3 %, OVER by Message-ID 1 %; reads in-process, through `ovb_server` (E), and in a
  worker pool.
- S3 expiry: preload 8 synthetic days, expire one under S2 load; then ingest one more day (space reuse).
- S4 crash: 50 x kill -9 of writer/feeders at random; after each, verify: every acknowledged article
  present (the generator logs acks), no partial record, `quick_check` ok, no number reused, missing
  numbers counted.
- S6 server cost: the S2 read mix through `ovb_server` vs in-process: added latency per request
  (p50/p99), server CPU, behaviour with K = 1, 2, 4 server readers when one client asks for cold
  ranges; kill -9 of the server under load (clients must see errors within the timeout, then
  recover).

**Metrics.** Latency p50/p99/max per command; longest single store call in a reader loop; ingest
articles/s and CPU µs per article (`getrusage`); bytes written per entry (`/proc/<pid>/io`
`write_bytes`; `zpool iostat` logical vs physical where ZFS); fsyncs; max WAL size and checkpoint
times; ack delay (A', C, D-writer); bytes per article on disk; files; fds per process
(`/proc/<pid>/fd`); RSS.

**Decision rules** (thresholds are proposals):
- R1 integrity (S4): 0 lost acknowledged, 0 partial, 0 reused, or the option is out.
- R2 stall: longest reader-loop store call ≤ 20 ms p99.9 and ≤ 100 ms max (S2+S3), in-process. An
  option that passes only through E or the pool makes step 4 a prerequisite of its default switch.
- R6 server: E adds ≤ 0.1 ms p99 per request and ≤ 10 % CPU over in-process reads, else the worker
  pool for 4b.
- R3 ingest: ≥ 3x the target peak (Jesse's archive peak rate, to ask; placeholder 1000 articles/s,
  c≈2), CPU ≤ 1 core, ack delay p99 ≤ 50 ms.
- R4 write cost: physical bytes per entry on ZFS ≤ 1/3 of `ovb_today` on the same pool; elsewhere
  compared relatively.
- R5 size: within 20 % of 2.4, else revisit blob vs spool.
- A' direct (step 3b) is acceptable as an interim if its feed-fork lock waits stay ≤ 100 ms at
  P = 2 and 2x the target rate; else step 4c moves up.
- Pick A' if it passes R1-R4. B instead only if A' fails R3 and B passes. D if the SQLite variants
  fail R2 or R3 and D passes, and the owner accepts a DB server. C only if every SQLite variant fails
  R4 clearly and C passes R1. A only if it beats B on R2/R3 with ≤ 256 fds per process. N for B: the
  smallest passing value.

**Hosts and limits.** Linux pool hosts through jobq.sh (e.g. 113 Debian/glibc, 105 Alpine/musl), one
run per host, every job under the 30-minute cap (split the shapes), container memory 1280 MiB +
1 GiB swap, free disk checked first (datasets of a few GB per backend). ZFS amplification (R4) is
decisive on illumos, the F-09 platform: the arc1-ca BSD/illumos VMs are on hold until the user lifts
it, so either that, or Jesse runs `ovbench` on his box with the user's OK. Package installs only by
the lead (project rules); a sibling's build-on-host reports missing packages instead.

**Libraries per OS** (package names from general knowledge, *to verify* with `pkg-config` on each
host before the run):

| OS | libsqlite3 | libmariadb (+ server for D) |
|---|---|---|
| Debian / Ubuntu / Hurd | `libsqlite3-dev` | `libmariadb-dev`, `mariadb-server` |
| RHEL / Alma / Rocky / Fedora | `sqlite-devel` | `mariadb-connector-c-devel`, `mariadb-server` |
| Alpine (musl) | `sqlite-dev` | `mariadb-connector-c-dev`, `mariadb` |
| Arch / openSUSE / Void | `sqlite` / `sqlite3-devel` / `sqlite-devel` | `mariadb-libs` / `libmariadb-devel` / `libmariadbclient-devel` |
| FreeBSD / DragonFly | `databases/sqlite3` | `databases/mariadb-connector-c`, `mariadb*-server` |
| NetBSD / SmartOS (pkgsrc) | `databases/sqlite3` (NetBSD also has it in base) | `databases/mariadb-connector-c` or `mariadb*-client` |
| OpenBSD | `sqlite3` package | `mariadb-client` / `mariadb-server` |
| OmniOS | `library/sqlite-3` | `ooce/database/mariadb-*` |

Minimum SQLite 3.24 (UPSERT; WAL, `prepare_v3` are older), 3.27 for `VACUUM INTO`; old LTS releases
may ship older versions (*guess*: RHEL 8). Alternative: vendor the public-domain amalgamation in
`lib/` behind a Makefile/vendor.h switch, one tested version on every target, at the cost of compile
time in the 20 CI jobs. For D, a local MariaDB on the host with a small buffer pool (256-384 MB) to fit
the container; `_start/_cont` needs Connector/C built with non-blocking support (check per host).

---

## 9. Migration

**Version marker.** HMAGIC3 (PR #412, commit 485b5352, not on upstream/main 051b4157) works because
old history code refuses any other magic. Old overview code does not refuse a newer version
(step 2a in section 7), so:
- Step 2a ships the refusal: v7+ binaries refuse `oh_Version > OH_VERSION` and an unknown
  `path_group/FORMAT`.
- Cutover writes `path_group/FORMAT` ("moved: diablo-overview-1 at <path_overview>"), and the new
  store's `FORMAT` file + `meta` row, which a new binary checks against `readeroverstore` (mismatch:
  refuse to start).
- For pre-v7 binaries: rename `path_group` to `path_group.v4` and leave a regular file at
  `path_group`. An old dreaderd then fails `GetOverInfo` for every group: feed articles get
  `400 Error writing data` and the connection closes (`group.c:715-722`); because pass 1 already
  advanced NE, each retry burns numbers (missing numbers, no corruption); GROUP still answers from
  dactive.kp with the `DEFMAXARTSINGROUP` clamp (`nntp.c:392-399`); OVER is empty. Release note.

**Convert (keeps every number).** `doverctl convert old new`: per group read slots NB..NE and their
data records through the old backend, de-duplicate articles across groups by Message-ID text (one
article, many refs), split columns and blob, skip and report corrupt entries. Offline. Time on the
archive *(guess)*: reading 244 GB plus ~10^8 article inserts, a few hours; measured on a copy first.
If the downtime is too long: start dreaderd on the new store and convert behind it, with readers
falling back to the old store below each group's conversion mark (more code; only if needed).

**Rebuild from the spool.** `doverbuild` reads the spool (like `dspoolrefeed`) and inserts
directly. Numbers are kept only when the articles' Xref carries this reader's numbering
(`readerxrefslavehost` setups); otherwise groups are renumbered, as a refeed does today (today §4.4).
With D-210: the new store no longer needs a rebuild on a history-hash switch (2.1); if D-210's switch
comes first, its overview rebuild and this conversion can be one step.

**Cutover.** Stop dreaderd, convert, `doverctl verify`, set `readeroverstore`, start, compare
OVER/HDR/HEAD of sample groups with the old store. **Rollback:** `readeroverstore old`, move
`path_group.v4` back, remove the marker; articles received since cutover are missing in the old
store unless `doverctl convert new old` is run for them (the API has both backends) or an optional
dual-write period (`readeroverstore both`: write both, read new) was used.

---

## 10. dreaderd: from forks to threads

The owner leans toward replacing the fork-per-N-sessions model with threads and mutexes. This
section is the code evidence for that move, at 051b4157. Counts come from a pattern scan of the
sources (file-scope declarations and `static` inside functions); they are approximate (multi-line
declarations can be missed) and are meant to size the work, not as an exact list.

### 10.1 Process-global and static state

**dreaderd/** (25,725 lines of `.c`): ~152 file-scope variables and ~56 function-local statics
(13 of the lines are `const` tables, harmless). By where they live:

| Where | State | Why it matters for threads |
|---|---|---|
| `main.c:116-257` (68 file-scope) | accept/DNS-queue state, listener and handback fds, totals (`TotalClient*`), fork counts, `ThisReaderFork` | main process only; stays single-threaded if the main loop stays one thread |
| `dns.c` (29 file-scope, ~20 statics) | `accessmap` (`:147-148`), Perl interpreter `my_perl` (`:1499`), PAM conversation statics `PAM_user/PAM_password/PAM_error` (`:1699-1701`), `alarm()`-based auth timeouts (`:404-412`), `static char result[]` per auth backend (`:638,815,871,984,1038,1372,1768`) | DNS/auth workers are separate forks; Perl, PAM and `alarm()` are per-process tools; **keep these as processes** |
| `thread.c:34-42` | `SFds/RFds/WFds`, `MaxFds`, `CurTime`, `TimerBase`, `FDes[MAXFDS]`, `TMemPool` | the event loop itself: becomes one struct per loop thread |
| `reader.c:42-48` | `KDBActive`, `TFd`, `ReaderSlotAry`, `TerminatePending`, `CanTerminate` | per fork; KPDB handle and graceful-exit flags |
| `group.c:176-177`, `:910`, `:959` | overview cache `OvHash[OVHSIZE]`, `NumOverInfo`, `static int OI` | shared cache: mutex or per thread |
| `list.c:80-85` | `activeCache` with `activeCache_ReadLock/WriteLock` **ints** (not locks) | shared cache needs real locks |
| `cache.c:31-34` | `CacheHits` mmap, `CacheHitsFD`, `CacheHitsEnd` | shared map, written in place |
| `cancel.c:23` | `CCacheAry` (pre-cancel cache) | shared |
| `server.c:47-55`, `:661`, `:722` | spool-server request lists `SReadBase/SWriteBase`, counts, `static int Randex1/Randex2` | per-process server connections shared by its sessions |
| `rtstatus.c:76-97` | status-slot file and slot base per fork | slot numbering by `ThisReaderFork` (`reader.c:154`) |
| `post.c:100,122,170,621,737,1635,1744` | `static char res[256]`, `res[512]`, `msg[96]` (`PostDateCheck`), `name[]`, `refusedMsg[]`, `err[]` returned to callers | static result buffers: caller buffers or thread-local |
| `control.c:360,733,778` | `CtlLogName[256]`, `SupersedeBy`, `whyBuf[96]` | same |
| `reader.c:838`, `mbuf.c:323`, `dreaderd/spool.c:248`, `crypto.c:108` | `static char r[512]`, `compoutbuf[65536]`, `filepath[PATH_MAX]`, SHA-256 result | same |
| `dreaderd/subs.c:285-286` | `GenerateMessageID`: `static LastT`, `MsgIdCounter`; the id is time + counter + **`getpid()`** + host | two threads of one process could generate the **same Message-ID** in one second: needs an atomic counter and a thread id |
| `filealloc.c:44,74-75` | `fallocate_ok` flags | benign races |

**lib/ code dreaderd calls** (by function-name match: `kpdb`, `log`, `hash`, `subs`, `zalloc`,
`expire`, `parsedate`, `canlock`, `iplist`, config; the feeder modules `history`, `newsfeed`,
`arttype`, `spamfilter`, `precommit`, `spool`, `hostauth`, `include`, `active`, `stats` are not called
directly): ~113 declarations, 56 of them the path/pattern globals of `global.c` set at start.

| Where | State | Why it matters |
|---|---|---|
| `config.c:30` | `struct DiabloOpts DOpts` | read-only after start, except on config reload: reload needs a quiesce point or a new copy swapped in |
| `kpdb.c:196-200` (comment), `:188`, `:528-529` | KPDB uses **fcntl record locks**, which are per process ("If you fork() after calling KPDBOpen() you must call KPDBReOpen()"); `static char *ResDC` result buffer | fcntl locks do not exclude threads of one process: KPDB needs a mutex (per record or per database) and per-thread result buffers |
| `log.c:24-28`, `:271` | `LogInfo` structs, `static char logLine[8192]` | `logit` from several threads: lock or per-thread buffer |
| `zalloc.c:51-53`, `:367-368`, `:536` | debug pool state, `mempool_id`; pools themselves are unlocked (`SysMemPool`) | `co_MemPool` per connection is fine while a connection stays on one thread; `SysMemPool` needs a lock or per-thread pools |
| `hash.c:56-60`, `:207`, `:344`, `:386` | `CrcXor` table + `DidCrcInit`, **global `HMD5ctx`/`BMD5ctx`**, `md5hash()` returns `static digest`, `GFName()` returns `static GFBuf[PATH_MAX]` | `md5hash` and `GFName` are not reentrant (the D-210 MD5 hash would use them) |
| `lib/subs.c:172-173,271,354-355,731-732,858,889,895,932,1094` | `PatPath1/2`, `static char st[]` results of the address formatters (`NetAddrToSt` and others), `seen[32][64]`, `FBuf[8][32]` rotating buffers, `buffer[32768]` | static result buffers |
| `expire.c:15-19` | `OvExBase`, `EXMemPool` (dexpire.ctl, reloaded by `LoadExpireCtl`, `reader.c:188,309`) | reload while other threads read it |
| `logtime.c:22`, `strerror.c:19`, `msgid.c:114`, `xopen.c:26,172` | static result buffers, `DirCache` | same |

### 10.2 Non-reentrant libc calls

| Call | dreaderd/ | lib/ | Replacement |
|---|---|---|---|
| `strtok` | 49 (control.c 18, dns.c 11, main.c 5, post.c 5, spool.c 5, server.c 4, list.c 1) | 19 | `strtok_r` |
| `strerror` | 87 | 88 | `strerror_r` (XSI/GNU variants differ: one wrapper) or a thread-local buffer |
| `localtime` / `gmtime` | 4 / 3 (`control.c:371`, `dreaderd/subs.c:178,266-267`, `post.c:1301,1304`, `misc.c:41`) | 3 (`logtime.c:27`, `stats.c:657`, `newsfeed.c:2826`) | `localtime_r` / `gmtime_r` |
| `inet_ntoa` | 3 (`main.c:1577,1579`, `dns.c:1232`) | 3 (`hostauth.c:275,462`, `lib/subs.c:935`) | `inet_ntop` |
| `gethostbyname` / `gethostbyaddr` | 1 at startup (`main.c:740`; `getservbyname` `:724`) | `lib/subs.c:529,601`, `hostauth.c:272,467` | `getaddrinfo` / `getnameinfo` |
| `getpwnam` / `getgrnam` | `main.c:870-871`, startup only | | fine (before threads) |
| `random` | `mbuf.c:76` (rate limit), `dns.c:1429` (DNS fork) | `lib/spool.c:309,366` (feeder) | per-thread state or `arc4random` where available |
| `system` | `reader.c:1756` (crash handler, after `fork()`) | `popen` in `include.c` (feeder config) | fine where it is |

### 10.3 Process assumptions beyond variables

- **`select()` and `FD_SETSIZE`.** Readers wait in `select(MaxFds, ...)` (`reader.c:263`), and
  `main.c:2207-2226` caps `RLIMIT_NOFILE` at `FD_SETSIZE` for that reason. One process holding the
  sessions of all forks (400 by default) plus spool connections and files passes 1024: the loop has
  to move to `poll()` (or epoll/kqueue/event ports) first.
- **Signals are per process.** Reader forks install `SIGHUP` -> `TerminatePending` (graceful
  recycle), `SIGUSR1` -> `DumpThreads`, and with a crash handler `SIGSEGV/BUS/FPE/ILL` ->
  `sigSegVReader`, which closes **every fd of the fork** and runs `readercrashhandler` in a forked
  child (`reader.c:161-168,1725-1760`). With threads, one signal reaches one arbitrary thread and a
  fault handler cannot isolate a session; graceful recycle becomes a per-thread flag.
- **Fork-time initialisation per reader** (`ReaderTask`, `reader.c:138-200`): status slots by
  `ThisReaderFork` (`:154`), its own `KPDBOpen` of dactive.kp (`:182`), `LoadExpireCtl` (`:188`),
  spool server connections (`CheckServerConfig`, `:194`), the access cache (`ReadAccessCache`,
  `:541,1203`). Each becomes shared (one copy, locked) or per thread.
- **Client handoff.** The main process accepts, waits for DNS/auth, then passes the socket to the
  least-used fork over a socketpair (`main.c:3191-3200`). With threads that becomes an in-process
  queue; the main loop and DNS forks can stay as they are.
- **Privileges and limits.** `setgroups/setgid/setuid` (`main.c:870-899`) and `RLIMIT_CORE`
  (`:911`) happen once in the parent before the forks; they work the same with threads (all before
  the first thread is created).
- **Memory pools** assume one thread per pool (no locks in `zalloc`).

### 10.4 What is lost: fault isolation

Today a crash (or a stuck loop) in a reader fork takes down at most `readerthreads` sessions
(default 40) and the main process starts a new fork (`main.c:1097-1109,1186`). In one threaded
process every crash ends every session, and a memory corruption in one session can damage the others.
Ways to limit it:
- **Hybrid:** P processes x T threads (the same knobs as today: `readerforks` processes, threads per
  process). T = 1 is today's model; a crash loses P's share of sessions.
- Keep code that runs foreign libraries in processes: DNS/auth (Perl, PAM, NETREMOTE), the crash
  handler, and the overview server (E).
- Sanitizers on the test hosts: ThreadSanitizer (clang/gcc `-fsanitize=thread`, Linux) over the e2e
  suite, ASan as today.

### 10.5 A phased path (each step shippable; efforts are *guesses*)

| Step | Content | Effort |
|---|---|---|
| T1 | `poll()` instead of `select()` in the reader and main loops; drop the `FD_SETSIZE` cap | 1-2 weeks |
| T2 | libc: `strtok_r`, `localtime_r/gmtime_r`, `inet_ntop`, `getaddrinfo/getnameinfo`, one `strerror` wrapper, per-thread random (10.2); no behaviour change | 1-2 weeks |
| T3 | lib/: caller-provided buffers for `GFName`, `md5hash`, the `subs.c` formatters, `logtime`, `msgid`; `logit` locked; KPDB mutex + per-thread result buffer; `SysMemPool` locked or per thread | 2-4 weeks |
| T4 | dreaderd: event-loop state into a per-loop struct (`thread.c`); static result buffers in `post.c`, `control.c`, `reader.c`, `mbuf.c`, `spool.c` to caller buffers; `GenerateMessageID` with an atomic counter and thread id; statistics atomic | 3-5 weeks |
| T5 | Shared caches with real locks or per thread: overview cache, `activeCache`, `CCacheAry`, `CacheHits` map, spool-server lists, access cache, expire ctl reload | 3-5 weeks |
| T6 | Hybrid P x T: sessions pinned to one loop thread (so per-connection state stays single-threaded), handoff queue, per-thread graceful recycle; start with T = 1, raise on the test hosts under TSan | 2-4 weeks |

In all *(guess)* **4-6 months** of agent work plus review (every step touches locks or memory:
council at `#high`). DNS/auth stays in forks. Main risks: races that tests do not hit (TSan helps
only on what runs), the crash blast radius, lock contention on the shared caches, behaviour
differences of threads on illumos/BSD, and the review load against the v7 goal "bug-free".

### 10.6 Relation to the store options

- One process with threads can hold the single SQLite writer as a **thread** (A' without IPC),
  batch writes from every feed session, and share one page cache and one overview cache instead of
  one per fork; MariaDB (D) gets one connection pool.
- With the hybrid (P > 1), there are still P writers: A' then needs E (or one designated writer
  process) as before; B or D tolerate P writers.
- E and the thread move are independent: E works with today's forks, and later the server can become
  a module inside a threaded dreaderd. The worker pool (3.2) is a small, contained first use of
  threads in dreaderd (workers touch no dreaderd state), so it can serve as the pilot for T1-T6.
- Steps T1-T3 help any option (poll, reentrant lib); the store phases 1-3 (section 7) do not wait
  for them.

---

## 11. To confirm (not shown here)

- dnewslink requeues on `400` after TAKETHIS (`ERR_GOODBYE`, `util/dnewslink.c:185`).
- Reader children fork only in the crash handler (`reader.c:1744`), after the fault; nothing else forks or execs after start (needed for the worker pool and threads).
- Jesse's peak ingest rate and the crosspost factor c of the archive.
- SQLite and MariaDB package versions on each test host; Connector/C non-blocking support.
- Every number marked *estimate* or *guess*, by section 8.
