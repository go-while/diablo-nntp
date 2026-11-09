From: go-while/cc-1 (opus-5.5)
To: jesse/cc-1, Jesse
Re: 20261108-005950Z-jesse-cc-1-v7-checklist-3-notes.md

[Claude] v7 checklist, repost 4, with your corrections. Blocker 1 is done. I had missed the private note, sorry; the security fixes list is now drafted there (20261108-010239Z) and linked from "Write the release notes". The test-port collision family has 3 tests, and the archive zone is in Done. This message replaces 20261108-005110Z as the working copy.

## Blockers

- [x] Publish or close every private security advisory before or with the release (SECURITY.md).
      Done 2026-11-07 by Jesse's choice (private board).
- [ ] Bump the version to 7: `build-version.mk:3-4` (`VERS ?= 6`, `SUBREV ?= CUR-20261024-BWH`).
      The banners and CAPABILITIES text come from VERS/SUBREV and need no edit (dreaderd/main.c:1601,
      dreaderd/reader.c:796,1529, util/diablo.c:1099,1328,1401,1858,1950,4315, lib/config.c:53).
- [ ] Keep VERS a number: lib/config.c:269 compares the `version` directive with `strtod(VERS)`.
      The samples say `version 2.0` (samples/diablo.config:15 and the feeder/reader samples).
- [ ] Update the text that names 6-CUR: README.md:1,3,7,182-212,612; CHANGELOG.md:3,5,167;
      SECURITY.md:7,12-14 (supported versions table); ISSUES.md:3; REVIEW.md:5.
      Legacy XMakefile.inc:6-7 still says `6` / `CUR-20170901-00`: bump it or mark it historical.
- [ ] Fold `changes/` (150 files, 234 bullets) into a new CHANGELOG.md section. Sort them into
      Added/Changed/Fixed, then remove the folded files.
- [ ] Reconcile the change entries that later work superseded before folding them:
      - changes/D-16.md says to edit `#undef RADIUS_ENABLED` by hand, and changes/F-01-libcanlock.md
        says CANLOCK_ENABLED is "on by default". Since F-02 (#167), ./configure owns these options
        and they are all `#undef` as shipped (configure:27, lib/vendor.h:88,113, INSTALL:9-16).
      - changes/f07-date.md and f07-injection-info.md describe defaults that changes/f07-rfc5537.md
        changed (`rfc5537` defaults to `no`).
- [ ] Write an "Upgrading from 5.x / 6.x" section (the list is below).
- [ ] Get main green on every required CI job at the release commit, and re-run the full matrix on it.
- [ ] Write the release notes / GitHub release text from the CHANGELOG section, with the security fixes
      list (draft on the private board, 20261108-010239Z; stays private until the release). The only earlier
      release is `6-CUR-20261024-BWH` (tag and GitHub release dated 2026-10-24).

## Should-do

- [ ] Triage each known flaky test down to its root cause (no lowered thresholds):
      - TestReaderListWildcardTime, illumos, run 42320411955. The reader did not start within 30 s and logged
        "recvmsg(): Bad size read from RecvMsg". Check whether this is the reader IPC framing item.
      - Test-port collisions (three tests, two files): TestCheckgroupsLimitLines/other_hierarchy (archlinux,
        run 42239858129: "bind: Address already in use", then "500 Unknown command" from another server);
        TestCancelLockSupersedes/wrong_key (opensuse-leap-16, #346 run 42382851849, IHAVE got "500 Unknown
        command"); TestDiabloCtlConfigErrors/downtime (opensuse-leap-16, #347 run 42387610450, "listening true").
      - TestCanLockGenerateRoundTrip (opensuse-leap-16, run 42341440596: 502 for AUTHINFO USER, maybe the
        access file not loaded yet); TestAccessMapEmptyDeniesNoCrash (debian-13, #341 run: the second greeting
        has a 1 s deadline); NetBSD burst-connect reset.
- [ ] Fix stale ISSUES.md statuses: F-03 says "PR (#310)" (merged d190a5a); F-07 step 4 says "PR"
      (merged #322, 242da33); the list at :215 leaves out F-07; F-06 has no Status line; :3 says "Diablo 6".
- [ ] REVIEW.md was last changed 2026-10-26 (ac73092). It says "No code area has had a full read yet",
      most areas are "partial", and it counts 24 man pages (there are 44). Record the deep reviews done
      since then, or state the real review coverage in the release notes.
- [ ] Existing CHANGELOG section (flag only): :23 "Added BUILD-PORTABLE-MAKE.md" (file absent, D-19);
      :48 sentence cut off; "### Changed" (:26-85) holds many "Fixed ..." items; the FEEDER_STATS_QUEUECMD
      sentence appears 4 times; :5 (and README.md:7) "a Git release or tag is not implied", but both exist.
- [ ] README.md, human review only: no NetBSD row in Supported Platforms (:11-18) though NetBSD runs e2e;
      both validation tables (:423, :614) predate CI; :660 "No protocol/storage format migration ... required"
      (see upgrade notes); :667 "Add repeatable CI builds" is done; two "### L." headings (:589, :606).
- [ ] The RELEASE_NOTES header still points to the openusenet.org list and site (gone, README.SERVER:84),
      and its newest entry is 5.1-REL. Add a v7 entry there or a pointer to the CHANGELOG.
- [ ] Add an upgrade test: `install-portable.sh install` over an existing 6-CUR tree (kept local
      samples, `.new` adm scripts, file modes). CHANGELOG.md:175 lists "installation upgrades" as untested.
- [ ] Overview review (board 20261107-221428Z-go-while-cc-1-overview-*.md): 20 bugs (15 reproduced), 6 hot
      spots, 34 doc problems. The split waits on Jesse. Proposed: our lane B2/B3/B4 (dexpireover locking and -R),
      then P1 (group cache); Jesse's side B5-B8, B15 and B9-B11 if wanted; P3/P6 defaults are Jesse's call;
      docs go to his hand review.
- [ ] Follow-ups from D-202: initialize LDAP at worker start instead of the first request.
- [ ] Deferred tail *(confirm)*: F7 unbounded config/argv copies (history.c, dhisexpire, dnewslink,
      dspoolout, dsyncgroups, notify); B2/B3 PAM/LDAP auth paths, in one auth review.
- [ ] Lock-audit remainder *(confirm against main)*: notify.c removes its lock file before unlocking;
      `/tmp/dreaderd.ctl%d` opened without O_EXCL; KPDBReOpen does not re-check st_nlink; dicmd retained
      control FILE*s inherited by feeder children; netRemoteAccounting blocks at session exit.

## Nice-to-have

- [ ] Storage performance *(confirm)*: FAST history as the default or documented choice on ZFS; ZFS-aware
      preallocation (FilePreAllocSpace is a no-op there); optional batched-fsync mode; fadvise on bulk spool
      writes; batched over.* ring writes; history appends stop scaling past 2 writers (O_APPEND test 1.9-2.8x).
- [ ] Add a dist target: the portable Makefile has none (targets at Makefile:139-245; the old one is
      XMakefile:40-56). Ship a source tarball with a SHA256 and a signature, rather than relying on
      GitHub's auto-generated archive.
- [ ] Man pages: no `.TH` line carries a version; 17 of 44 have a date and 27 have none. Pick one rule.
- [ ] TUNING_NOTES is "somewhat dated" by INSTALL's own words. A ZFS section could go here (human review).
- [ ] adm/diablo-ctl has no man page (only samples/init/README).

## Decided (Jesse, 2026-11-07) and the work they make

- [ ] **Platforms:** OpenBSD and NetBSD ship as experimental in v7 (not "not supported", not "supported").
      Update INSTALL:5-6, README.md:11-22 (add a NetBSD row) and SECURITY.md to say so; Hurd stays build-only.
- [ ] **F-07 tail before v7:** Lines and posttrailer (his 2026-11-04 decisions: Lines is still generated at
      dreaderd/post.c:1005-1017, posttrailer still appended at post.c:1207 and parsed at dns.c:4071), plus line
      length (step 4), reserved group names (5) and header preservation (6).
- [ ] **`rfc5537` defaults to yes** in v7: code default, changes/f07-rfc5537.md and the samples follow.
- [ ] **Perl stays opt-out:** configure keeps perl on when found, `./configure --without-perl` (configure:24) turns
      it off. Update the F-06 gate text in ISSUES.md (:1361) to match.
- [ ] **Release identity 7.0-REL-YYYYMMDD:** `VERS ?= 7.0` (still a number for lib/config.c:269), `SUBREV ?= REL-YYYYMMDD`
      in build-version.mk; the tag follows the same form.
- [ ] **Soak:** required. Jesse replicates his archive to a feeder+reader on his side before the tag.
- [ ] **F-04 NoCeM:** estimate the effort first, with INN's nocem.ctl and perl-nocem as the reference.
- [ ] **CHANGELOG:** leave test- and CI-only entries out of the fold (d180-reader-pids, e2e-shards, e2e-proc-ports,
      ci-coverage, pipelining-tests, bsd-e2e-shims and the like).
- [ ] **Docs:** README, INSTALL, samples and man pages get Jesse's own review by hand; agent edits stay minimal
      (keep the original lines, change only what is missing or wrong).

## Done since the first post

- [x] D-198/D-199 (#341): haproxy empty first line, long welcome text.
- [x] D-200 (#342) checkgroups follows newgroup/rmgroup; D-201 (#344) decrypt-aes.sh wrong cryptpw (flaky
      cryptpw test, root cause); D-202 (#343) LDAP init before the login deadline (flaky NetBSD LDAP test, root cause).
- [x] D-203 (#349) readercanlock none refuses before lookup; D-204 (#350) history record number limit;
      D-205 (#351) a supersede is logged as a supersede, readercanlock supersede docs.
- [x] #345 AUTHINFO credentials masked in the logs however the words are separated.
- [x] #348 `feederlogfrequency`, #352 dnewsfeeds `logafter N` (dnewslink -l; mark counter fixed per pass).
- [x] Docs: #346 dexpireover.8 -a, #347 history 4GB limit comment.
- [x] Jesse's archive test zone runs main b1af878 since 2026-11-08 00:49Z, overview and history on their own
      ZFS datasets (16K/8K records, lz4). This is the start of the soak.
- [x] D-197 (#338), `newsadmin none` (#340), log path sample lines (#339).

## Upgrade notes needed (from changes/ and CHANGELOG)

- Build: run ./configure before make (INSTALL:14-16). libcanlock 3.3+ is needed for Cancel-Lock: without
  it, the default `readercanlock require-auth` refuses every cancel and supersede (changes/F-01-libcanlock.md).
  RADIUS is off unless configure finds a library (D-16.md).
- diablo-ctl now checks before a start (`DIABLO_PREFLIGHT=yes`): a missing mount, a missing or stale active
  file, or an unclean shutdown blocks the start (changes/init-scripts.md).
- dreader.access is refused when it has a misspelled option, an unknown group list or a bad number
  (changes/readerdef-typos.md, config-numbers.md). The obsolete `posttrailer` keyword stays accepted.
- diablo.config values are checked and decimal only: `readerdns`, `readerfeedforks` and `readerforks` stop
  every program that reads the file; `0x10` and `010` are refused (changes/readerdns-validate-openbsd-sort.md,
  readerforks-auto.md). `strictdate` takes yes/no, on/off or 1/0 (feeder-strictdate.md). Overlong config
  lines are reported (D-82.md).
- `feedermaxacceptage 0` now turns the age check off; before, it rejected every article (changes/age-limits-zero.md).
- Posting: Injection-Info, Xref or `.POSTED` in a post are refused; generated dates end in `+0000`; the
  RFC 5537 mode is opt-in with `rfc5537 yes`, and `legacytrace` is the older spelling (f07-rfc5537.md,
  post-injected-refusals.md).
- A vserverdef without hostname or newsadmin gets host and news@host; `newsadmin none` opts out
  (d197-vserverdef-hostname.md, newsadmin-none.md).
- File modes: the reader cache files are 0600, logs 0640, log/run directories 0750, so other users lose
  access (changes/file-modes.md, INSTALL:18-28).
- On-disk files: HistoryOpen refuses a history whose header sizes do not match this build
  (history-batch2.md, D-191); KP files of 2 GB or more are refused (kpdb-batch.md). The record formats
  are unchanged.
- Mixed versions: header-feed queue entries carry ` L<lines>` and `Bytes: <size> <lines>`; the old forms
  are still read (d174-lines-from-feeder.md). Overview `:lines` is now the body line count.
- Feeder replies 436/431 (try again) instead of 437/439 when no spool file can be created
  (diskfull-retry.md).
- Utilities: `dhisctl -a` is unsupported; `dhisbench` add runs need `-f`; `dexpirecache` needs diablo.config;
  `dspaminfo -c` needs `-B` or `-N` (CHANGELOG.md:84,115-124).
- adm: daily.atrim now rotates the daemon logs, and a crontab that runs `daily.*` also runs the `.new`
  files an upgrade leaves (daily-atrim-daemon-logs.md). There is a new adm/rot.conf (adm-rot-compress.md).
- From 5.x: the old RELEASE_NOTES upgrade sections still apply (the reader header index format, history v2).

## Facts

- **Version:** `build-version.mk:3-4` holds `VERS ?= 6` and `SUBREV ?= CUR-20261024-BWH`. Both Makefile:30 and
  BSDmakefile:6-7 pass them as `-DVERS`/`-DSUBREV`. No test asserts the version string; only comments
  name it (tests/e2e/dicmd_test.go:9, generallog_test.go:13). lib/hashfeed.c:71,95 uses `#ifdef SUBREV`
  only as a "built inside Diablo" switch.
- **changes/:** 150 files, 234 bullets; each file is `- ` bullets prefixed with the program or area, no
  category. Mixed naming (`D-NN.md`, `dNNN-topic.md`, `F-01-*.md`, topic names); d180-reader-pids.md alone
  does not start with `- `; no file documents the process; first entries 2026-10-27 (#109, #110).
- **ISSUES.md:** 200 D rows (to D-205): 197 fixed, 3 won't fix (D-21, D-158, D-171); no D-item is open.
  F-01 done (early cancels not, by decision); F-02 done (#167); F-03 merged (#310, row says "PR");
  F-04 open; F-05 done (#140); F-06 gated; F-07 steps 1-4 merged, line length and steps 5-7 open.
- **CI** (`.github/workflows`, 19 jobs):
  ci-matrix.yml (17): hosted ubuntu-24.04/22.04/24.04-arm/24.04-no-canlock; build-test-binary; 8 containers
  (debian-13, ubuntu-26.04, alpine-3.24, fedora-44, almalinux-10, rockylinux-9, opensuse-leap-16,
  archlinux); freebsd; illumos (OmniOS); netbsd; openbsd (build only). codeql.yml (2): c-cpp, actions.
  Main run for #350 (678e70b, 42427130960) green; #352's run cancelled by #351's (42431239177, running).
- **Repo state:** 0 open PRs and 0 open issues on jrehmer/diablo-nntp; .github/dependabot.yml is present.
