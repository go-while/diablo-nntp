# go-pugleaf SQLite storage: (A) the flat overview/article store, (B) the sharded history DB

Date: 2026-11-16. Source: go-pugleaf at commit 527ec49 (main),
`github.com/go-while/go-pugleaf` (paths below relative to it unless
prefixed `nntp-history:`, which means `github.com/go-while/nntp-history`
at 8c5321b, the library the history design came from).
Method: reading and git only. Nothing was built or run (guard G1). The only command that
touched data was a read-only `ls`/`du` of the local dev `data/` directory.

## Summary

go-pugleaf keeps two separate SQLite stores, as the owner said:

| | (A) overview/article store | (B) history hash DB |
|---|---|---|
| Layout | **flat**: one SQLite file per newsgroup, `<data>/db/<md5(group)>/<group>.db`, no sharding | 16 files `<data>/history/hashdb_0..f.sqlite3` x 256 tables `_00.._ff` |
| Key | `article_num INTEGER PRIMARY KEY` (rowid) + `message_id UNIQUE` | `message_id TEXT PRIMARY KEY ... WITHOUT ROWID` |
| Holds | overview columns, raw header block and full body in one `articles` row | `message_id -> "groupID:articleNum,..."` |
| Serves | GROUP, LISTGROUP, OVER/XOVER, HDR/XHDR, ARTICLE by number, by message-id inside a group | ARTICLE `<id>` outside the current group, IHAVE/TAKETHIS/POST duplicate checks, full `Xref:` |
| Truth? | yes, source of truth | index only, rebuildable (`internal/history/history_config.go:56-58`) |
| Relevance for Diablo | model for an overview store | lessons on sharding SQLite under concurrent writes |

The main DB `<data>/cfg/pugleaf.sq3` (config, newsgroups counters, users) is a third,
single file (`internal/database/db_init.go:332`).

---

# Part A: the overview/article store (flat, one DB per group)

## A1. File layout
- One SQLite file per newsgroup: directory `DataDir + "/db/" + GroupToHash(group)`, file
  `SanitizeGroupName(group) + ".db"` (`internal/database/db_groupdbs.go:218-227`).
  `GroupToHash` is the full 32-hex md5 of the group name
  (`internal/database/groups_hashmap.go:147-148`), so each group has its own directory; no
  bucketing, no shared file. `SanitizeGroupName` maps `[^a-zA-Z0-9_-]` to `_`
  (`internal/database/utils.go:131-153`).
- History of the layout: until Aug 2025 three DBs per group (articles, overview, threads;
  `internal/database/migrations/old/0001_overview_schema.sql:1-26`). `0001_single_db_schema.sql:1-5`
  "Replaces the 3-database-per-group architecture" (7ed619b, 2025-08-10); the overview table
  was folded into `articles` (`internal/database/migrations/README.md:88-91`).
- Schema migrations run on every open of a group file
  (`internal/database/migrations/README.md:50-53`, `internal/database/db_migrate.go:354-357`).

## A2. Schema and indexes
`articles` (`internal/database/migrations/0001_single_db_schema.sql:11-29`):
```
article_num INTEGER PRIMARY KEY      -- rowid = NNTP number
message_id  TEXT NOT NULL UNIQUE
subject, from_header TEXT; date_sent DATETIME; date_string TEXT; "references" TEXT
bytes INTEGER, lines INTEGER, reply_count INTEGER DEFAULT 0, path TEXT
headers_json TEXT                    -- raw header block joined with "\n" (nntp_overview.go:20-23)
body_text TEXT                       -- full body
downloaded INTEGER, imported_at DATETIME, spam INTEGER, hide INTEGER
```
- A rowid table (not `WITHOUT ROWID`): OVER/LISTGROUP ranges are rowid range scans.
- Indexes: automatic UNIQUE(message_id); `idx_articles_date_sent`
  (`0001_single_db_schema.sql:91`); `(hide, date_sent DESC)`, `(spam, hide)`,
  `(hide, article_num)`, `(article_num, spam)` (`0008_single_spam_hide_performance_indexes.sql:5-14`);
  redundant ones dropped by `0009_single_drop_redundant_indexes.sql:7-14`.
  No index on `imported_at`, so NEWNEWS is not implemented ("a NEWNEWS * would open every
  group DB", `BUGS.md:37-38`).
- Monotonic numbers: INTEGER PRIMARY KEY without AUTOINCREMENT reused numbers after
  expiry (INT-5); fixed with `article_num_seq(id=1,last_num)` and an AFTER INSERT trigger
  (`0010_single_article_num_seq.sql:1-30`); the insert computes
  `MAX(last_num, MAX(article_num)) + 1` in SQL (`internal/database/db_batch.go:1178-1187`).
- Column order (my reading, not measured by pugleaf): `downloaded`, `spam`, `hide` sit
  after `body_text`, and the OVER query selects `downloaded` and filters `hide = 0`
  (`internal/database/queries.go:1952-1956`). For an article whose body spills into
  overflow pages, SQLite walks the overflow chain to reach those columns. Overview data in
  the same record as the body is the one thing a Diablo overview table should not copy.
- Other tables (`threads`, `thread_cache`, `cached_trees`, `tree_stats` + trigger) feed the
  web view only (`0001_single_db_schema.sql:31-143`); `DisableThreading` skips their writes
  for a standalone NNTP server (`internal/database/db_init.go:90-94`).

## A3. Pragmas and connections
- Per connection, through a registered driver `sqlite3_pugleaf_group` whose `ConnectHook`
  runs the list (`internal/database/db_init.go:367-402,426-455`):
  `busy_timeout=30000`, `foreign_keys=ON`, `synchronous=NORMAL`, `cache_size=2000` (pages,
  ~8 MB at 4 KiB), `temp_store=MEMORY`, `journal_mode=WAL`, `wal_autocheckpoint=2000`
  (`internal/database/db_init.go:26-30,438-450`). mmap and page_size never set (defaults: 0, 4096).
- Per group `*sql.DB`: `SetMaxIdleConns(2)`, `SetConnMaxIdleTime(5 min)`, no
  `SetMaxOpenConns` (`internal/database/db_groupdbs.go:232-237`).
- Open-group cache with worker counts (`GetGroupDB`/`Return`, `db_groupdbs.go:101-190,322-334`).
  Soft cap `MaxOpenDatabases = 256` (`db_groupdbs.go:13`): a cron every 10 s closes groups
  idle > 1 h, and at >= 256 open force-closes the oldest idle ones down to 128
  (`internal/database/database.go:15,23-44,58-63,75,97`). The cron keeps running into
  shutdown because "a large backlog would exceed MaxOpenDatabases and the fd limit during the
  final flush" (`database.go:27-31`).
- Driver: `github.com/mattn/go-sqlite3 v1.14.32` (cgo, `go.mod:9`), SQLite 3.50.4
  (`docs/perf/main-db-pool.md:70-71`).

## A4. Write path
1. Intake hands articles to `SQ3batch`, which queues them per group
   (`internal/database/db_batch.go:221-240,2260-2342`).
2. A group is flushed at `MaxDBbatch` articles (100..1000, default 1000) or on a timer:
   `BatchInterval = 3 s`, shrinking with backlog to 125 ms (`db_batch.go:18-39,183-195`,
   defaults `db_init.go:123-125`).
3. At most `MaxDBthreads` (default 16) groups in parallel, one batch per group at a time
   (`db_batch.go:426-446,537-556`). Audit A-db-5 found a path where a batch slower than
   120 s let a second batch of the same group start (`docs/audit/audit-db.md:480-500`).
4. PHASE 1: one deferred transaction per chunk, one prepared `INSERT OR IGNORE` for all rows,
   `RowsAffected()==1` marks real inserts, commit, then one `SELECT message_id, article_num
   ... WHERE message_id IN (...)` to learn the numbers (`db_batch.go:1191-1300`). On error:
   close the group DB, sleep 1 s, retry unbounded while live, drop after a 2-minute grace at
   shutdown (`db_batch.go:858-918,659-700`).
5. PHASE 2 threading (web only; errors logged and swallowed by decision, `db_batch.go:933-966`).
6. PHASE 3: history adds for committed articles, then main-DB counters in a retried
   transaction (`db_batch.go:968-1145`).
- Consequence: an accepted article is invisible until its batch commits. pugleaf added a
  same-process `recentArrivals` map, a `local430` negative cache and `-post-commit-wait`
  (default 5 s) before the 240 reply (`internal/nntp/nntp-article-common.go:122-132`,
  `internal/nntp/nntp-server.go:123-129`, `cmd/nntp-server/main.go:84`).

## A5. Read paths
All NNTP reads filter `hide = 0` and bypass the article cache
(`internal/database/nntp_reads.go:9-13`). Busy/locked errors go through `Retryable*`:
linear backoff 10 ms..2.5 s with jitter, at most 5 min of waiting
(`internal/database/sqlite_retry.go:15-22,86-108`).

| Command | Query | Source |
|---|---|---|
| GROUP | `MIN/MAX(article_num) WHERE hide=0`; count = main-DB `message_count` clamped to the span | `nntp_reads.go:69-77`, `internal/nntp/nntp-cmd-group.go:115-158` |
| LISTGROUP | `SELECT article_num WHERE hide=0 AND article_num BETWEEN ? AND ? ORDER BY article_num LIMIT 10000`, paged | `nntp_reads.go:15-17,98-121`, `nntp-cmd-group.go:244-247` |
| OVER/XOVER | 11 columns, same range form, `LIMIT 1000`; first chunk read before the 224 line (empty range -> 423/420, DB fault -> 403), then keyset paging from `last+1`; a fault after the status line closes the connection | `internal/database/queries.go:1949-1993`, `internal/database/nntp_overview.go:8-11`, `internal/nntp/nntp-cmd-over-hdr.go:143-193` |
| OVER `<msgid>` | resolves like ARTICLE, builds the line from the row | `nntp-cmd-over-hdr.go:120-141` |
| HDR/XHDR | one column per overview field and `path`; Xref served empty; other headers from `headers_json` parsed per row in Go | `queries.go:2017-2060`, `nntp_overview.go:20-52,58-96` |
| ARTICLE/HEAD/BODY by number | one rowid lookup, whole record | `nntp_reads.go:22-41` |
| STAT by number | `message_id` only | `nntp_reads.go:57-65` |
| NEXT/LAST | `article_num > ? LIMIT 1` / `< ? DESC LIMIT 1` | `internal/database/nntp_nextlast.go:7-26` |
| ARTICLE `<msgid>` | current group via UNIQUE(message_id), then recent arrivals, `local430`, then history (Part B) and each listed group DB, re-read with `hide=0` | `internal/nntp/nntp-article-common.go:105-170`, `internal/processor/processor.go:295-321` |

## A6. Expiry and VACUUM
- Offline tool `cmd/expire-news` (days, per-group `expiry_days`, `-prune` to
  `max_articles`) (`cmd/expire-news/main.go:26-118`).
- Keyset scan over `article_num` reading `date_sent` (`cmd/expire-news/main.go:560-588`;
  an earlier OFFSET-while-deleting version skipped rows, `docs/history-index-implementation-patch.md:66`).
- One transaction per batch, chunks of 5000: `DELETE FROM articles WHERE article_num IN (...)`,
  thread rows and derived caches (`cmd/expire-news/main.go:624-735`).
- **No VACUUM, no auto_vacuum, no incremental_vacuum** anywhere in code, docs or scripts
  (grep, 0 hits): freed pages stay in the freelist, files never shrink.

## A7. Concurrency
- In one process: many reader goroutines; one writer per group, up to 16 groups at once.
- Across processes: fetcher, nntp-server and web may share a data directory. The rsync
  maintenance mode says "stop fetcher, nntp-server and web first" and refuses a group whose
  `-wal` survives its close, because "SQLite removes the WAL on the last close only"
  (`cmd/web/main_functions.go:327-354`, c4d6226).
- WAL: readers never block the writer; writers on one file serialize (deferred
  transactions, 30 s busy_timeout, `Retryable*` retries).

## A8. Recovery and numbers
- Crash recovery is SQLite's WAL recovery on open; `synchronous=NORMAL` (no corruption on
  power loss, last commits may be lost).
- `cmd/recover-db`: logical check per group (max numbers, counts, gaps, orphans), `-repair`,
  `-rebuild-threads`, `-parsedates/-rewritedates`, `-reorder-by-datesent` into a `.new`
  file; write modes need explicit `-data` and `-force` (`cmd/recover-db/main.go:150-170`,
  `internal/database/db_rescan.go:60-140`). No `PRAGMA integrity_check` anywhere.
- Numbers: none for large spools (file size per million articles, OVER throughput, WAL
  growth) in repo, docs or commits. The only SQLite measurement is the main DB pool: 0
  `SQLITE_BUSY` retries in 35 configurations, plateau above 8-16 connections
  (`docs/perf/main-db-pool.md:11-20`). Local dev data: 25 groups, 150 MB (du, not representative).

---

# Part B: the sharded history DB (16 files x 256 tables)

## B1. Mapping Message-ID -> file and table
- `md5(message-id)` as hex; 1st hex char -> file `hashdb_<c>.sqlite3` (16), chars 2-3 ->
  table `_<cc>` (256) (`internal/history/history.go:978-991`,
  `internal/history/history sqlite3_sharded.go:118,227-232`). Hard-coded, "unchangeable !"
  (`history sqlite3_sharded.go:53-55`, `history_config.go:185-190`).
- Full message-ids are stored, so no collisions (`cmd/history-rebuild/README.md:31-32`).

## B2. Schema
```
CREATE TABLE _xx (message_id TEXT NOT NULL PRIMARY KEY, newsgroups TEXT) WITHOUT ROWID;
CREATE INDEX _xx_rej ON _xx(newsgroups) WHERE substr(newsgroups,1,1)='r';
```
(`internal/history/history sqlite3_sharded.go:205-218`, `internal/history/history_rejects.go:73`).
`newsgroups` is a CSV of `groupID:articleNum` tokens (legacy plain `groupID`), or one
remembered-reject token `r<unixday>` (`internal/history/history.go:84-97`,
`history_rejects.go:55-58`). Tables are created only when a file lacks them, in one
transaction per file (`history sqlite3_sharded.go:184-224`). An empty file is 1,089,536
bytes (local `data/history`, ls), so ~17 MB for an empty index.

## B3. Pragmas and pools
- DSN `?_journal_mode=WAL&_synchronous=OFF&_busy_timeout=30000&_txlock=immediate&_cache_size=-8000`
  plus ConnectHook `temp_store=MEMORY`, `mmap_size=16 MB`, `wal_autocheckpoint=2000`
  (`internal/history/history_config.go:210-244`).
- 16 pools, `MaxOpenConns = MaxIdleConns = 32` by default, idle timeout 300 s
  (`history sqlite3_sharded.go:98-123`, `history_config.go:155`).

## B4. Add path
- Callers only enqueue a value op on a channel of `2 x BatchSize` (blocking = backpressure)
  (`internal/history/history.go:221,240-296`); adds happen after the group DB commit
  (`internal/database/db_batch.go:968-1010`).
- One writer goroutine flushes every 1000 ms or 10000 ops (`history.go:34-36,633-692`).
  A flush splits ops by file and writes the **16 files in parallel**, one transaction per
  file, statements prepared lazily per (query, table) (`history.go:709-751,789-917`).
- The add is an UPSERT that merges tokens with `instr/replace/trim` in SQL
  (`history.go:98-111`); renumber/remove read-modify-write in Go (`history.go:919-976`).
- `BEGIN IMMEDIATE` (via `_txlock`) so lock contention shows up at BEGIN; retried with
  backoff 100 ms..5 s, 100 attempts, then `log.Fatalf` (`history.go:46-49,763-787`). Any
  error after BEGIN: rollback and `os.Exit(1)`, marked "DO NOT TOUCH" (`history.go:801-807`).
- On writer exit: `wal_checkpoint(TRUNCATE)` on all 16 files (`history.go:694-707`).

## B5. Lookup path
- Point lookups `Exists`/`LookupGroups`/`LookupLocations`: one PK query in one table
  (`history.go:298-377`).
- Batched lookup for Xref on OVER ranges: ids grouped per file, one `UNION ALL` statement
  per file with one `IN (...)` term per table, 500 ids per statement (`history.go:379-503`).

## B6. Why 16 x 256, and what it solved
Lineage, from git:
1. 2023: nntp-history used **BoltDB with 16 DB files** (`intBoltDBs = 0x10 // do NOT
   change this!`, nntp-history `BBOLTDB.go:18` at 0a61281), keyed by the first hex char
   (`HEXCHARS`, `nntp-history.go:66,244` at c903341^), with 4096 buckets from a 3-hex-char
   prefix. bbolt allows one writer per file, so 16 files meant 16 parallel writers
   (my inference from bbolt's design; no commit says it in words).
2. 2024: bbolt removed (nntp-history 058d680, c903341).
3. 2025-06-13/14: SQLite backend in nntp-history; "4096 Table Sharding: Uses 3-character
   hex prefixes" (nntp-history `SQLITE3_README.md:10,26-29`); six modes 1x4096, 4096x1,
   16x256, 64x64, 128x32, 512x8 implemented (nntp-history
   `sqlite-sharded-integration-log.md:32-39`). Their fd analysis: 1 DB = 3-5 fds, 4096 DBs
   "could hit OS limits", 16-512 manageable (`:112-115`).
4. Measured 2025-06-14, 1M inserts per mode (`sqlite-sharded-integration-log.md:323-333`):
   | Mode | Files | Init | Insert 1M | Rate |
   |---|---|---|---|---|
   | 1 x 4096 tables | 1 | 8.55 s | 3.00 s | 333,474/s |
   | 16 x 256 | 16 | 2.42 s | 3.01 s | 331,937/s |
   | 64 x 64 | 64 | 3.05 s | 3.10 s | 322,424/s |
   16x256 was picked as "Write-Heavy / Balanced ... Fast setup ... good concurrency"
   (`:371`). Cache per file was cut to 2000 pages because 16 x the single-file cache would
   be 6.4 GB (`:230-234`, recommendation `cache_size=2000` at `:272`; pugleaf's
   first port kept `getAdaptiveCacheSize` = 2000, `internal/history/history_config.go` at 61312d3).
5. 2025-08-03 (61312d3) the layout entered pugleaf with `history.dat` + short hashes;
   2025-11-06 (fb0c9a7) history.dat and short hashes dropped, rows became full message-ids;
   the 2026-09 rewrite kept the 16x256 layout as a design rule
   (`docs/history-index-implementation-patch.md:34-42,83`).

What the measurements show:
- Insert throughput was **the same** for 1 file and 16 files (0.5 % apart). The 16-file
  win measured was startup time (creating 4096 tables in one file: 8.6 s vs 2.4 s). No
  lock-contention or concurrent-writer measurement exists in either repo.
- The 256-table split inside a file costs on batched reads: ids of a 1000-line range spread
  over ~900 tables; one query per table was 1.2x slower than 1000 point lookups, one
  statement per file 1.7x faster (21.3 ms -> 12.5 ms per 1000 ids) (db6a8db message;
  `.claude/plans/done/xref-batched-lookup.md:15,25,118`).
- Point lookup cost through `database/sql`: ~21 us.
- What the 16 files do buy in pugleaf's code: 16 independent WAL write locks, so the
  history flush commits 16 transactions in parallel (`history.go:709-751`), and smaller
  files/WALs per checkpoint. One file per hex char also keeps the fd count fixed at 16 x pool.

Problems the history hit (from `docs/history-index-implementation-patch.md`):
1. Pragmas set with `db.Exec` reached one pooled connection only ("part of the IO issues")
   -> per-connection ConnectHook (`:53`).
2. 4096 `CREATE TABLE IF NOT EXISTS` and a TRUNCATE checkpoint at every start -> create only
   when missing (`:53`).
3. Writer stall: plain INSERT + infinite retry inside the transaction on
   `UNIQUE constraint failed` -> idempotent UPSERT (`:50`).
4. "history + hashdb eats memory and has IO issues" (BUGS.md 2025-08-04, `:37`) ->
   history.dat dropped.
5. Shutdown hang and duplicate cache items (`:49,51`) -> string API, sharded in-memory cache.
None of these were SQLite lock contention; all were application bugs around it.

## B7. Rebuild and trim tools
- `cmd/history-rebuild`: rebuild from the group DBs with a resume file, `-validate-only`,
  `-analyze-only`, `-trim` (`cmd/history-rebuild/README.md:1-60`, `cmd/history-rebuild/main.go:82-91`).
- `expire-news -trim-history` removes entries of deleted articles after the commit
  (`cmd/expire-news/main.go:44-47,708-716`); by default entries are kept (INN-style remember).
- Rejects are pruned by one process holding an advisory `flock`
  (`internal/history/history_rejects.go:46-48,494-500`).
- `synchronous=OFF`: OS crash or power loss can lose or corrupt history; accepted because it
  is rebuildable.

---

# What a C port to Diablo would need

Diablo today: per-group `over.*`/`data.*` files in hashed directories
(`lib/hash.c:608-627`, `OD_HARTS 256` in `lib/defs.h:443`); ReaderFeedForks (default 2)
write overview, ReaderForks (default 10, 40 connections each in a select loop) read it
(`lib/config.c:112-114`); article numbers from the KPDB active file
(`dreaderd/group.c:540`); overview written per article (`dreaderd/group.c:553`); XOVER by
message-id is a stub (`dreaderd/xover.c:466-471`).

For Part A (overview):
- **Linkage**: system `-lsqlite3` or the public-domain amalgamation vendored in `lib/`;
  `sqlite3_open_v2`, `sqlite3_prepare_v3(SQLITE_PREPARE_PERSISTENT)`, `sqlite3_step`,
  `sqlite3_busy_timeout`. No license conflict.
- **Forks**: open connections after `fork()`, never inherit them. Diablo forks its children at
  startup (`dreaderd/main.c:1721-1738`), so each child opens its own in `task()`. Build
  `SQLITE_THREADSAFE=0` or `2`. WAL across processes needs a local filesystem (`-shm` mmap).
- **Select loop**: `sqlite3_step` blocks the whole child (40 sessions). Bounded chunks with
  keyset paging (pugleaf: 1000 rows) fit Diablo's incremental `NNListOverviewRange`; use a short
  busy_timeout and retry on the next pass instead of pugleaf's 30 s waits.
- **Schema**: an overview-only table (Diablo keeps bodies on the spool):
  `article_num INTEGER PRIMARY KEY` set explicitly from KPDB (no `article_num_seq` needed),
  `message_id TEXT UNIQUE`, overview fields, xref, bytes, lines, received time; large or
  rarely read columns last; an index on received time if NEWNEWS should ever work.
- **Writers**: two feed forks on one group file serialize on its WAL lock: route a group to
  one feed fork (hash), or `BEGIN IMMEDIATE` + retry. Keep transactions small or batch below
  a second; pugleaf's 3 s window forced three visibility workarounds (A4).
- **Files and fds**: one file per group = simple per-group expiry, but an fd and an open per
  group in every reader child (x10 children); pugleaf caps at 256 open per process with idle close.
- **Expiry**: `auto_vacuum=INCREMENTAL` at creation + `PRAGMA incremental_vacuum` after
  deletes, or files only grow (pugleaf has neither). A periodic `PRAGMA quick_check` in a tool.

For Part B (sharding lessons):
- pugleaf's own numbers do not show that 16 files write faster than one: equal insert rate;
  the gain was startup time for 4096 tables. Sharding into files pays when several
  processes or threads write at once (one WAL write lock per file), which matches Diablo's
  multi-process writers better than pugleaf's single writer goroutine.
- Tables inside a file add nothing for B-tree lookups and cost on batched reads (1.2x slower
  per-table); one `WITHOUT ROWID` table per file is enough.
- Diablo already has its feeder history (dhistory); a reader-side msgid index is needed only
  for ARTICLE/XOVER `<msgid>` without a group or for full Xref.
- The fail-stop policy (`os.Exit` on any history write error, `log.Fatalf` after 100 busy
  retries) would kill 40 sessions in a Diablo child; a port needs a softer policy.

Go-specific, does not carry over: `database/sql` pools and `ConnectHook` (C: pragmas right
after open); goroutines/channels for the batch divider and the 16 parallel flushes (C: a
writer process, or one per shard); `Retryable*` wrappers (C: busy_timeout + retry in the
event loop); md5 routing (Diablo has its own hash code).

# Not checked
- No large-spool numbers exist in either repo (sizes, OVER throughput, WAL growth under load).
- The column-order overflow cost (A2) and the bbolt single-writer rationale (B6.1) are my
  reading, not stated or measured by the authors.
- Web-side readers and the fetcher's download path were not reviewed; Diablo facts are only
  those cited.
