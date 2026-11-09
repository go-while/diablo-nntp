From: go-while/cc-1 (opus-5.5)
To: jesse/cc-1, Jesse
Re: 20261107-221428Z-go-while-cc-1-overview-review.md

[Claude] Overview review part: bugs (main 9320a42).

# Overview subsystem: correctness review (upstream main 9320a42)

Reviewed 2026-11-07. Source: read-only `/diablo/test/ovrev`. Builds: `/diablo/test/ovbugs` (normal build)
and `/diablo/test/ovasan` (ASan, -O0 -g). Probe tests: `/diablo/test/ovbugs/tests/e2e/zz_ovbugs_probe_test.go`
(not for upstream as is). Run one with:

    cd /diablo/test/ovbugs/tests/e2e && DIABLO_BIN=/diablo/test/ovbugs/dbin \
      PROBE_DBG_BIN=/diablo/test/ovasan/dbin go test -count=1 -v -run TestProbe<Name> .

Baseline: the existing e2e set `-run 'Over|Xover|Hdr|Expire|Reader'` passes under ASan with **no ASan report**.
29 tests fail at first only because their LD_PRELOAD shims trip ASan's link-order check. With
`ASAN_OPTIONS=detect_leaks=0:verify_asan_link_order=0` all 29 pass and there is still no report.

Items marked "(howto agent)" came from section 9 of `/diablo/test/overview-howto.md`. I checked each one; the
verdict is given in the item.

Severity: High / Medium / Low / Nit. **REPRODUCED** means a probe or ASan run shows the failure.
**READ** means found by reading the code only.

---

## Findings, ranked

### 1. (moved)
One Medium finding about very large data files is handled separately (see the private board note).

### 2. Medium: dexpireover's expire passes rewrite index slots without the offset-0 record lock; the ring-wrap race destroys a new article's index entry (howto agent #1: CONFIRMED)
- `util/dexpireover.c:1386-1398` (ExpireByDays), `:1510-1519` (ExpireBySpool), `:1627-1636` (ExpireFromFile) do
  `copy = *oa; copy.oa_ArtNo = -2; lseek; WriteAll`. This is a read-modify-write of a mapped slot, with no
  `hflock(fd, 0, XLOCK_EX)`.
- dreaderd writes slots under that lock (`dreaderd/group.c:808`). CancelOverArt and dcancel/dgrpctl do too.
  Expire does not take the offset-4 lock either. That is correct, since readers hold SH@4.
- Failure scenario: a full ring. The next article NE+1 goes into the slot of article NE+1-maxarts, and that is
  exactly the oldest article, which expire is retiring. dexpireover reads the old record. dreaderd writes the new
  article into the slot. dexpireover then writes the old copy back marked -2. The new article disappears from
  OVER/XOVER, HEAD by number and LISTGROUP, while GROUP still counts it. Nothing repairs it.
- **REPRODUCED** deterministically with gdb (`TestProbeExpireRingWrapRace`):
  - Setup: `dexpire.ctl *:a4:x0.00002:l0`, articles 1-4 fed and left to expire. A breakpoint at
    `dexpireover.c:1395 if i == 1` holds the old slot-1 copy; article 5 is fed (slot 5%4 = 1); then dexpireover continues.
  - Before: `XOVER 5` = `["5\tnew 5\t..."]`.
  - After: `XOVER 5` = `[]`, GROUP = `211 5 1 5`.
  - The window is microseconds per expired record, so in production this is rare but real on busy groups.
- Fix: take `hflock(fd, 0, XLOCK_EX)` around each slot update (or per batch). Re-read the slot under the lock
  and write only if `oa_ArtNo` and the hash still match.

### 3. Medium: `dexpireover -R` keeps the old index over new data when the new over file cannot be written
- `util/dexpireover.c:1968-1995` removes the original data files first. `:1997-2058` writes the new over file to
  `.<name>`. If that fails (`ok = 0`: open error, short write, ENOSPC, EIO, rename error), it prints
  "Rewrite ... failed, removing", removes only the temp file and **leaves the old over file**. Then
  `:2063-2108` still renames the new, compacted data files into place.
- Result: the old index (old offsets, and the old DataEntries) now points into the new data files. The comment
  says "we pretty much have to blow the file away", but the code does not. Entries come back as "corrupt
  overview entry" or simply missing. Where the offsets happen to line up on record boundaries, a reader could
  even see another article's headers. Exit status is 0 and the summary says "1/1 indexes resized successfully".
- **REPRODUCED** with `TestProbeRewriteOverFailure`. A directory at `.o.0.e2e.test` makes the temp create fail
  (it stands in for ENOSPC/EIO). Control run: `XOVER 2` gives the record. Injected run: dexpireover prints
  "Rewrite of e2e.test over. file failed, removing" and `XOVER 2` = `[]`.
- Related (READ): no fsync of the new data or over files before the renames, and the `write()`/`close()` return
  values at :2025-2028 are only partly checked. After a crash, a renamed file can be empty or partial on
  filesystems without rename-over-existing flushing (XFS, for example). The crash window between "remove
  originals" and "rename temps" loses the group's overview by design (comment at :1969-1973).
- Fix: on `ok == 0`, keep the old data files (build the new ones fully and rename them only after the new index
  is in place), or remove the index too so the group re-initialises consistently. fsync before rename.

### 4. Medium: busy groups are effectively never resized; dexpireover waits 1 s while feed forks hold the group up to 300 s (howto agent "related": CONFIRMED, functional)
- `util/dexpireover.c:192` sets `LockWaitTime = 1`. ResizeGroup/RewriteData need `XLOCK_EX@4` (`:2246-2257`, `:1722-1734`).
- dreaderd keeps SH@4 on every cached group. A feed-only fork flushes its cache only every 300 s
  (`dreaderd/reader.c:336-343`). Reader forks flush every 30 s. A group is released by eviction once 8 groups are
  cached, and reopened by the next article.
- On a group that gets articles all the time, the over file is almost never unlocked for 1 s. The `-s`
  growth from `InitArts` (512) never happens, the ring wraps, and articles inside the expire window silently
  leave overview. The only trace is "resize ... failed, file in use" (with -v) and the "x/y indexes resized" count.
- **REPRODUCED** the lock hold (`TestProbeResizeLockWait`). With `*:a16:i64`, 40 articles were fed, and 2 s later
  `dexpireover -s -l1` gave `resize maxArts from 16 to 64 failed, file in use grp=e2e.test`, with GROUP =
  `211 17 24 40`. The "never" part on a busy production group is inferred from the timing and was not run.
- Fix shape:
  - Retry skipped groups at the end of the run with a longer wait.
  - Or make dreaderd drop SH@4 when dexpireover asks. For example, dexpireover could take a non-blocking
    "want" lock at another offset that dreaderd polls in its 30/300 s tick, or dreaderd could hold SH only while
    an OverInfo has refs > 0.
  - At least document `-l` and the feed-fork 300 s hold.

### 5. Low-Medium: WriteOverview leaks an OverInfo reference when MakeOverHFile fails, which blocks dexpireover -s/-R for that group until restart
- `dreaderd/group.c:705-713` returns without `PutOverInfo(ov)`. Every other error path in WriteOverview after
  GetOverInfo does call it (:723).
- `ov_Refs` stays above 0, so neither FlushOverCache nor eviction ever frees the OverInfo. The process keeps
  SH@4 on the over file, plus its mapping and fds, for its whole lifetime.
- Trigger: any failure to create the data file (EMFILE, ENOSPC on create, EACCES, a damaged directory).
- **REPRODUCED** with `TestProbeMakeOverHFileRefLeak`:

  | Run | IHAVE | then | `dexpireover -R -l1 -w e2e.test` | same for `e2e.g00` |
  |---|---|---|---|---|
  | inject (directory at `d.0.e2e.test`) | `400 Error writing data` | 12 other groups fed through the same feed fork | "file in use" | rewritten (evicted) |
  | control (no injection) | 235 | same | rewritten | rewritten |

### 6. Low: a group name longer than MAXGNAME in Newsgroups drops the article from every group listed after it
- `dreaderd/group.c:349-350` (pass 1) and `:398-399` (pass 2): `if (ngroup - group > MAXGNAME) continue;`. This
  runs after `*ngroup = 0` and does not restore the comma or step past it. The next iteration starts at that NUL
  and the loop ends.
- The article is still answered 235, but it is missing from the later groups. Any feed peer, or a poster through
  the spool, can make an article invisible in overview with `Newsgroups: <257-char name>,real.group`.
- **REPRODUCED** with `TestProbeLongGroupThenGroup`. Three articles were fed: control, `long,e2e.test` and
  `e2e.test,long`. GROUP = `211 2 1 2`, and XOVER lists only "control" and "before a long name".
- Fix: restore `*ngroup = c` and advance before `continue` (or skip to the next comma).

### 7. Low: a duplicate group in Newsgroups gets two article numbers and two overview entries
- `dreaderd/group.c:328-370`: AssignArticleNo is called once per token with no de-duplication. Pass 2 matches
  tokens by pointer, so each copy writes its own record.
- **REPRODUCED** with `TestProbeDuplicateGroup` (`Newsgroups: e2e.test,e2e.test`):
  - GROUP = `211 2 1 2`.
  - Two XOVER lines carry the same Message-ID.
  - Xref = `e2e.test e2e.test:2 e2e.test:1`.
- INN de-duplicates.

### 8. Low: GROUP's low mark is one below the oldest article still in the ring after a wrap (howto agent #2: CONFIRMED)
- `dreaderd/nntp.c:393-394`: `artBeg = artEnd - oh_MaxArts`. The slots hold `artEnd-maxarts+1 .. artEnd`, and
  `artEnd-maxarts` shares artEnd's slot. The code also uses the header's `oh_MaxArts` instead of `ov_MaxArts`.
- **REPRODUCED** with `TestProbeGroupLowAfterWrap` (4 slots, 6 articles): GROUP = `211 5 2 6`, XOVER shows 3-6,
  `STAT 2` = 423.
- `TestProbeResizeLockWait` shows the same: `211 17 24 40` with 16 slots. Fix: `artEnd - maxarts + 1`.

### 9. Low: dexpireover never processes data files in the hierarchy layout
- `util/dexpireover.c:803-805` treats a name as a data file only if it starts with `data.` or contains `.d.`.
  Hierarchy data files are `d.<base>.<group>`, so they never reach ProcessOverviewFile type 2:
  - Stale data files are never deleted (disk grows without bound).
  - `-z` never compresses them.

  Over files (`o.`) are handled.
- **REPRODUCED** with `TestProbeHierGzOrphan`. After `dexpireover -a` set NB=301, the stale `d.0` (range 0-255)
  and `d.4096` were both still there. Output: "Scanned 1 files" (only the over file).

### 10. Low: hierarchy layout `.gz` data files are taken for a group named "<name>.gz" and deleted as orphans by `doverctl clean` (howto agent #5: CONFIRMED, for doverctl; dexpireover never sees them, see #9)
- `lib/hash.c:689`: `sscanf(name, "d.%lld.%[^/].gz", ...)`. The `%[^/]` takes `e2e.test.gz` whole, and sscanf
  still returns 2 when the trailing literal fails, so the hash becomes "e2e.test.gz".
- **REPRODUCED** with `TestProbeHierGzOrphan`. `doverctl clean` printed
  `Group for over file not found, removing .../e2e/d.0.e2e.test.gz`.
- In hierarchy layout, .gz files come from `doverctl convert` of a compressed md5/crc spool, or from a future
  #9 fix that makes `-z` work there. Fix: match `.gz` first with an exact suffix test, or use `%[^/]` with a
  length check and strip ".gz".

### 11. Low: junk temp files of an interrupted `dexpireover -R` are never removed in md5 layouts
- `util/dexpireover.c:795`: `sscanf(name, ".%23[^.]s.%d.d.", ...)`. The literal `s` after the scanset can never
  match (the scanset stops at '.'), so the result is never 2.
- md5 temp names (`.<hash>.<iter>.d.<b>.<base>` and `.o.` names) do not start with `.data.` or `.d.` either, and
  ExtractGroupHashInfo rejects them, so they stay forever.
- **REPRODUCED** with `TestProbeJunkTempMd5` (md5-32/1): both temp files remain after `dexpireover -a`, "Scanned 0 files".

### 12. Low: `dgrpctl masscancel` never matches above 2^31 and holds the active record write lock for the whole walk (howto agent #4: CONFIRMED)
- `util/dgrpctl.c:198` uses `oa->oa_ArtNo == artEnd`. It needs `OA_ARTNOEQ`, because `oa_ArtNo` holds 31 bits.
- `:182` takes `KP_LOCK` for the whole scan. That blocks dreaderd's AssignArticleNo for the group. The record is
  read before the overview lock is taken (copy, then lock), which is harmless only because KP_LOCK keeps
  writers out.
- **REPRODUCED** with `TestProbeMasscancelAbove2G`. Article 2147483651 was fed and masscancel given its id:
  no output, and XOVER still lists the article.

### 13. Low: `dexpireover -o -n` (dry run) still consumes dexpover.dat (howto agent #8: CONFIRMED)
- `util/dexpireover.c:2530`: `rename(list, list.bak)` runs unconditionally.
- **REPRODUCED** with `TestProbeExpireOverListDryRun`: after `dexpireover -o -n`, dexpover.dat is gone and .bak exists.
- Effect: if dexpire writes a new list between the dry run and the real run, the real run's rename replaces .bak
  and the hashes of the first list are lost. Those articles then expire only by the `x` days fallback.

### 14. Low (damaged-file robustness): GetOverRecord trusts a negative `oa_Bytes`/`oa_SeekPos`
- `dreaderd/group.c:1182` rejects only `oa_Bytes > OVER_HMAPSIZE/2`. At :1283, `r[oa->oa_Bytes]` with a negative
  value reads before the mapping. A negative seekpos other than -1 reaches the mmap failure path in GetOverRecord.
- **REPRODUCED** with `TestProbeNegativeOverBytes` (`oa_Bytes = -0x40000000`):
  - ASan: `SEGV on unknown address ... READ ... #0 GetOverRecord dreaderd/group.c:1283 / #1 NNListOverviewRange xover.c:549`.
  - Normal build: the reader child dies on `XOVER 1`.
- Needs a damaged index file (local or crash damage), not a remote client. Same fix as #1.
- The dead `OutputOverRange` (only with `USE_OVER_MADVISE`) has the same class of problem, plus an off-by-one:
  `seekpos+bytes-HMapPos > HMapBytes` (:1442) allows reading `r[oa_Bytes]` one byte past the map. A record whose
  NUL guard is missing can also loop there without advancing (`return` at :1443). That is not built by default.

### 15. Low: OVER/XOVER `:bytes` is one too high on a full feed
- `dreaderd/xover.c:888-890` prints `artSize + 1`. `oa_ArtSize` is `co_ByteCounter`, which already counts every
  line with its CRLF and without the dot terminator (feed.c:303-304).
- **REPRODUCED** by arithmetic on `TestProbeLongGroupThenGroup`. The "control" article is 228 octets on the wire
  (RFC 3977 8.1.1 `:bytes`), and XOVER says 229.
- I did not check the header-feed path, where dnewslink's `Bytes:` value is used. The +1 may have been meant for
  that path; fix it in one place for both.

### 16. Low (READ): the Supersedes header string leaks per article on feed connections
- `dreaderd/group.c:196/238` allocates `supers` from `co_MemPool`. The frees at :444-450 omit it.
- Feed connections live for days, so the pool grows with every article that has Supersedes.

### 17. Low (READ): GetOverInfo loops without end, creating files, for a 256-character group name (howto agent #7: CONFIRMED by reading)
- `group.c:1093` copies at most 255 characters into `oh_Gname`. Then the `strcmp` at :1111 never matches, so it
  takes `iter++; goto again`, and each new iteration creates `over.<iter>`.
- MAXGNAME is 256. NNFeedOverview lets exactly 256 through (`> MAXGNAME`), but the group must also be in
  dactive.kp, and dgrpctl/AutoAdd refuse 256 (ValidGroupName `>= MAXGNAME`). Reachable only with a legacy or
  hand-made dactive entry.
- Fix: refuse `strlen(group) >= sizeof(oh_Gname)` in GetOverInfo (and in its dcancel/dgrpctl copies), and make
  NNFeedOverview use `>= MAXGNAME`.

### 18. Low (READ): FindCanceledMsg holds the group's active record lock (KP_LOCK_RO) while it walks up to maxarts slots
- `dreaderd/group.c:1515-1536`. D-193 fixed this for FindMsgInGroup only.
- With a big maxarts and a cancel flood from a peer, each cancel costs O(maxarts) per group named, and AssignArticleNo
  in other feed forks waits on the record. The fix is the same as FindMsgInGroup: read NB/NE, unlock, then walk.

### 19. Low (READ): an old data file range that was compressed gets a fresh plain file on write
- `MakeOverHFile(create=1)` (`group.c:1669`) opens or creates the plain name even when only `.gz` exists. Readers
  then open the plain file first, so every earlier record of that range points into the wrong (new, small) file.
- Reachable only when an article lands in a range dexpireover `-z` compressed. dexpireover protects the last
  `oe_StoreGZMinFiles` (default 2) ranges and files modified in the last 24 h, so this needs xref-slave
  out-of-order numbering or `:F0`. Fix: refuse to create when a `.gz` exists.

### 20. Low (READ): dreadover and `dgrpctl listgrouphashfile` ignore ITER (howto agent #3: CONFIRMED by reading)
- `util/dreadover.c:213, :285, :393` and `util/dgrpctl.c:451, :464` always pass iter 0 to GFName.
- For a group with a hash collision (ITER > 0), they show the other group's files. Admin tools only.

### 21. Low (READ): doverctl clean skips one-character top directories (howto agent #6: CONFIRMED)
- `util/doverctl.c:475-476` requires two alphanumeric characters.
- `md5-32/1` (and `/2`) put files under one-character directories: `dgrpctl listgrouphashfile` printed
  `0/0l2ugpamddj3jkuobfpmh.0.o.32.0`. So `doverctl clean` never cleans those layouts. dexpireover only checks
  `d_name[0]`.

---

## Nits

- (howto agent #9, CONFIRMED) `util/dexpireover.c:28-31` says dreaderd locks *data* files at offset 4.
  It does not: data files are locked only at offset 0 EX while appending (`group.c:715`).
- `dreaderd/list.c:389-407`: the comment says HDR by message-id is not implemented, and `LIST HEADERS MSGID`
  returns an empty list. Since #323/#331 it is implemented, so it should list ":". `:bytes` and `:lines` are not
  listed, although HDR `:lines` works. HDR `:bytes` returns an empty value (or "(none)" for XHDR) although
  `oa_ArtSize` is there.
- `NNStartListOverview` strict mode (`xover.c:309-313`) answers 423 only from the NB/NE bounds. A range inside the
  bounds but with no surviving article gives `224` and an empty list (RFC 3977 8.3.2 wants 423).
- XPAT takes the rest of the line as one pattern (`xover.c:210`). RFC 2980 allows several space-separated patterns.
- `group.c:1037-1041` vs `:1065-1068`: the version > OH_VERSION check is not repeated under the lock. A future
  version file then falls through and is used.
- `group.c:1107-1110` and dexpireover validate `oh_DataEntries` as a power of two. dcancel/dgrpctl GetOverInfo,
  dreadover (:285) and doverctl `getDataEntries` (:711) do not: DataEntries 0 gives a mask of -1.
- dcancel/dgrpctl GetOverInfo each `KPDBOpen` KDBActive again (dropping the global handle), use `KP_LOCK` rather
  than `KP_LOCK_RO`, and leak `ov` on the missing-record return (`util/dcancel.c:720-731`, `util/dgrpctl.c` same
  place). These are one-shot tools.
- `NNFeedOverview`: after a WriteOverview error (400 + NNTerminate) the loop goes on to the remaining groups, and
  `ExecuteControl` still runs (`group.c:408-432`).
- FileAllocSpace result unchecked in GetOverInfo (`group.c:1086`). `ov_Size` then comes from the computed size,
  not a fresh fstat. In practice the ftruncate fallback makes this harmless.
- `dreaderd/filealloc.c`:
  - The preallocation rounding `& ~(preallocsz-1)` assumes a power of two, but `maxarts*4` usually is not one.
  - `posix_fallocate` returns an error number, so its `e < 0` test never fires.
- `GetOverInfo` sets `ov_LimitSecs = LimitDays*86400.0` into an int with no range check (config).
- `lib/expire.c`: `:a`/`:i`/`:j` use `strtol` with no upper bound. `:a100000000` makes GetOverInfo fallocate
  3.2 GB per group.
- `dreadover`/`dgrpctl` print `oh_Gname` with `%s` from a disk header that may lack a NUL. The `strcmp(oh_Gname,
  group)` calls are fine: they stop at the group's NUL (correcting oc-7's C-note).
- `getDataEntries` (dexpireover :2614, doverctl :711) and ProcessOverviewFile (:883-885) read fields of an
  uninitialised `oh` after a short read (harmless, since r=0).
- Format nits: `group.c:1386,1392,1414,1418` print off_t with `%i`. `:1699` prints unsigned long `zlen` with `%d`.
- Several tools each define maxarts their own way: dreaderd from the file size, ResizeGroup/getFirstArtAge from
  `oh_MaxArts`, Expire* and RewriteData from the file size. They agree only while
  `size == headsize + oh_MaxArts*32`. ResizeGroup's shrink `ftruncate` failure is only logged (:2402-2406) and
  would break that.
- ResizeGroup rewrites in place with no fsync. A crash in the middle leaves records in old slot positions. They
  are rejected by `OA_ARTNOEQ`, so articles go missing rather than becoming wrong.
- `reader.c` flushes the overview cache only on every 10th select wakeup (`counter == 10`), so the 30/300 s
  periods are loose.

---

## Prior reviews: status on 9320a42

| Item | Status |
|---|---|
| oc-6 A7 (GHSA-hmh6 GetOverInfo path buffer) | fixed: `PATH_MAX`-sized `zalloc` plus `snprintf` in group.c; dcancel/dgrpctl use `PATH_MAX` and check truncation |
| oc-6 B1 (CtlCancel NULL walk) | fixed (`ptr != NULL`, control.c) |
| oc-6 B4 / DeepSeek B4 (WriteOverview Xref host overflow) | fixed: `xtmpLen` is sized from `xrefhost` plus 20 digits per group (`group.c:627-643`) |
| oc-6 B5 (GetOverInfo leaks) | fixed: FreeOverInfo on the KPDB miss, `path` freed on every return |
| oc-6 B8 | not overview core; not re-checked |
| oc-7 F1 (dexpireover `tbuf` stack smash) | fixed (`%23[^.]`). The same sscanf has the separate dead-pattern bug, #11 above |
| oc-7 C15 (dgrpctl maxarts 0) | fixed (`dgrpctl.c:341`) |
| oc-7 note on `strcmp(oh_Gname)` over-read | not a bug (bounded by the group's NUL) |
| oc-2 overmax 1 (dcancel/dreadover SIGFPE) | fixed (dcancel re-init path; dreadover `maxarts <= 0`) |
| oc-2 overmax 2 (stale fstat before the EX lock) | fixed: fstat again after the lock in group.c, dcancel and dgrpctl |
| oc-2 overmax caveat (negative `:a`) | fixed (`<= 0` becomes DEFARTSINGROUP) |
| oc-2 overmax nit (corrupt `oh_HeadSize`) | still live, see "checked" below and #14 |
| wildmat XPAT OOB | fixed (b2b6780) |
| ISSUES.md D-26, D-45, D-106, D-150 to D-155, D-174, D-176, D-193 | fixed as recorded; spot-checked in the code |
| ISSUES.md D-55 | the remaining OVER range part is bounded per pass (10000 records) |
| ISSUES.md open items | no open D-item about overview remains |

---

## Checked and fine (coverage)

**group.c**
- OverWriteAt/OverWriteV handle partial writes, EINTR and the return-0 case correctly.
- WriteOverview order is right: data append (under the data-file EX@0 lock), then index under EX@0; the data is
  rolled back with ftruncate if the index write fails. Readers never see an index entry before its data.
  `xtmp` sizing is exact.
- Lock protocol:
  - SH@4 is held by every cached OverInfo.
  - EX@4 is taken by ResizeGroup/RewriteData.
  - EX@0 is taken for record writes and initialisation.
  - The `nlink == 0` re-open after a rename-over is correct.
  - No ABBA: dreaderd takes data EX@0 then over EX@0, and nobody takes them in the other order (dexpireover
    never locks data files).
  - Re-initialisation truncates only a file that fails the check under EX@0, and any process holding a map of
    it would have failed the same check (the version check aside, see nits).
- GetOverRecord bounds when the file and records are sane: the map is always at most st_size (`xmap` refuses a
  map past EOF), `HMAPALIGN` 64K plus record 512K+2 fit in 1 MB, and the guard NULs are checked. No SIGBUS
  path: nothing truncates a data file below a referenced record while it is mapped (the rollback truncates
  only unreferenced bytes; dexpireover renames or unlinks).
- An embedded NUL in a fed header only shortens the output; NNRetrieveHead's msgid copy is bounded by MAXMSGIDLEN.
- MakeOverHFile `.gz` path: header length bounded (D-176), zlen limited to 256 B .. 64 MB, a temp file per open,
  every error path unmaps and closes, and the `count > ReaderThreads` eviction never frees the entry being returned.
- No pointer from NNRetrieveHead is used after a later GetOverInfo (nntp.c/cache.c/spool.c callers).
- CancelOverArt locks and re-reads.
- FindMsgInGroup: lock dropped before the walk, walk bounded.

**xover.c**
- OutputOverview parsing is bounded by the record's NUL.
- XPAT buffer (`hdr[128]` or zalloc of rlen+1) is correct.
- wildmat has ABORT, so no exponential blow-up.
- Range parsing: overflow guarded by `artno_input`; per-pass bound of 10000 records and 50 for XPAT.
- The msgid form resolves before "follows", and `co_ListMsgId` is freed.

**list.c**
- `NNTPListOverviewFmt`, `NNListGroup` (NULL ov handled).

**dexpireover**
- Parallel fork/wait accounting (D-154).
- getFirstArtAge, Expire* and ResizeGroup refuse `n < oh_MaxArts` or `maxarts <= 0`.
- ResizeGroup re-reads NE under its lock and copies only matching slots.
- RewriteData's NUL-guard checks in rewriteDataFile.
- Data-file deletion criteria and compression window.
- x64read/x64write loops.
- ReadDExpOverList hashing (chain walk is O(n) per insert, a performance issue only).

**Other files**
- `lib/expire.c` parsing is bounded (pattern length, line length).
- dcancel/dgrpctl/dreadover maxarts-0 guards.
- doverctl convert: over file moved last, .gz variant handled.

**Not covered:** `USE_OVER_MADVISE` and `READOA` builds beyond reading; OpenBSD (D-171, won't fix); performance.

## Cleanup
No daemons left (the harness stops them; `pgrep` shows none from ovbugs or ovasan). Test sites were Go TempDirs
and are removed. Worktrees `/diablo/test/ovbugs` and `/diablo/test/ovasan` are kept; the probe file is only in ovbugs.
