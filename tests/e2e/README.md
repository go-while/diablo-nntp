# End-to-end tests

These tests run Diablo's programs as separate processes and talk to them over
real TCP connections, the way a peer or an operator would.

Each test builds a throwaway feeder site in a temporary directory: a
`diablo.config` whose `path_home` points there, `dnewsfeeds`, `dspool.ctl`,
and `diablo -d` (in the foreground) listening on a free port of 127.0.0.1. The
test talks to it and stops it with `dicmd exit`. Nothing outside the temporary
directory is touched, and the sites of different tests share nothing.

The tests are written in Go and use the
[nntp-testkit](https://github.com/go-while/nntp-testkit) library (standard
library only): a strict NNTP client that fails on bare LF line ends, bad
dot-stuffing, a missing `.` terminator or stray bytes after a reply, fake NNTP
peers, and `proc`, which starts and stops `diablo` and prints its output when a
test fails.

## What is covered

| test | programs | what it checks |
|---|---|---|
| `TestPeerIHAVE` | diablo | IHAVE from the peer (127.0.0.2) is accepted (335, 235); a second offer gets 435 |
| `TestStrangerRefused` | diablo | a host that no `dnewsfeeds` label lists gets `502 ... Transfer permission denied` |
| `TestDicmdConfig` | diablo, dicmd | `dicmd config maxconnect 17` is applied and reported; `path_dumphist` is rejected (startup only) |
| `TestGeneralLogNotWritable` | diablo | with an unwritable `path_generallog`, diablo reports it and keeps accepting articles instead of crashing |
| `TestHostCacheLastPeerRemoved` | diablo, dprimehostcache | removing the last `inhost` empties `dhosts.cache`, and the former peer gets a permission denial, not "label missing" |
| `TestHostCacheRebuildAfterFreshStart` | diablo | after a start without `dhosts.cache`, a peer added to `dnewsfeeds` is let in once the cache is rebuilt |
| `TestDnewslinkStreaming` | diablo, dicmd, dnewslink | queued articles reach a streaming peer (fake) in order, with TAKETHIS and no IHAVE |
| `TestDnewslinkIHAVEFallback` | diablo, dicmd, dnewslink | a peer that refuses MODE STREAM gets the articles with IHAVE |
| `TestDnewslinkPeerHas` | diablo, dicmd, dnewslink | an article the peer already has is not sent (CHECK gets 438); the others are |
| `TestDnewslinkTermStalledPeer` | diablo, dnewslink | SIGTERM while dnewslink waits for a stalled peer (at the greeting, and on an unanswered CHECK/TAKETHIS): it exits within 5 s and both articles stay queued. **Fails** on main before this fix: dnewslink polled again for the full `-T` timeout (600 s) after the signal |
| `TestReaderFeedAndRead` | dreaderd, dgrpctl | an article fed to the reader by IHAVE shows in GROUP and OVER; BODY and ARTICLE fetch it from the spool server (fake) |
| `TestReaderQuitAlone` | dreaderd | QUIT gets 205 and nothing follows it |
| `TestReaderQuitWhileConnecting` | dreaderd | a client that QUITs while another connects gets 205 and nothing after it. **Fails** on main before jrehmer/diablo-nntp#12 (dreaderd wrote "400 Server too busy" after the 205 when its only DNS worker was busy) |
| `TestReaderPost` | dreaderd | POST is passed to the outgoing ('o') server (fake peer) with IHAVE: body as posted, the vserver's `postpath` in the Path |
| `TestReaderCapabilities` | dreaderd | CAPABILITIES (RFC 3977 §5.2): a reader connection lists VERSION 2, READER, OVER, LIST ACTIVE NEWSGROUPS OVERVIEW.FMT DISTRIB.PATS, POST and IMPLEMENTATION, and each of them works; a feed-only connection lists IHAVE and STREAMING; MODE STREAM is 501 in reader mode. Then nntp-testkit's conformance checks run against the reader (the capabilities, mode-reader and mode-stream-capability checks must pass; the rest is logged). **Fails** on main before this change: "500 What?" |
| `TestReaderBindHostPort` | dreaderd | `dreaderd -P 127.0.0.1:port` listens there. **Fails** on main before the -P fix: the whole "host:port" was passed to getaddrinfo |
| `TestReaderCacheHitsRepaired` | dreaderd | a damaged scoreboard (`cache.hits` whose header claims 1 MiB in a 20 KiB file) is replaced when dreaderd opens it with `readercache on` and `cache=scoring(...)`, and the replacement is logged. **Fails** on main before this fix (D-14): the file stays damaged and the scoreboard stays off. 64-bit little-endian only |
| `TestHeaderFeedToReader` | diablo, dnewslink, dreaderd | diablo feeds dreaderd a header-only feed (`headfeed on`, `dnewslink -H`); the reader shows the article in GROUP and OVER and fetches BODY from diablo as its spool server |
| `TestKPDBConcurrentUpdate` | dkp | two writers update one record with values of a new length while the test holds the record's lock, so the second waits inside its first lookup and finds a replaced record: exactly one live record must remain, with both changes. **Fails** on main before #33 (D-11): two live records for the key, one without the first writer's change |
| `TestDexpire` | diablo, dexpire | `dexpire -a` removes the spool directory of an article older than `keeptime`; STAT then gives 430, and offering the article again gets 435 (still in history) |
| `TestDexpireKeepsNewestDir` | diablo, dexpire | `dexpire -a` keeps a spool's newest directory even when it is older than `keeptime` (diablo may be writing into it): the article in it stays readable and the next one is stored. **Fails** on main before jrehmer/diablo-nntp#25 |
| `TestSpoolDirRemovedWhileWriting` | diablo | the spool directory a connection writes into is removed (as dexpire can do); the next article on the same connection is stored (235, then STAT 223) in a recreated directory. **Fails** on main before jrehmer/diablo-nntp#24 (235, then STAT 430: the article was lost) |
| `TestFeederChildSighupAfterQuit` | diablo | 400 rounds of QUIT, then SIGHUP to the feeder child (as `dicmd exit` and `dicmd pause` send) while it logs its session: no child may be left after 10 s. **Fails** on main before the D-40 fix: 85 to 169 of 1000 children hung in a futex wait inside the SIGHUP handler on glibc, 6 of 2000 on musl. Linux only (/proc); `E2E_SIGHUP_ROUNDS` sets the rounds |
| `TestDiabloCtlFeeder`, `TestDiabloCtlReader` | samples/adm/diablo-ctl | status (3 when stopped), start, a second start ("already running"), restart, stop (port closed), stop again; skipped without the script (`DIABLO_CTL`, default: `samples/adm/diablo-ctl` next to `DIABLO_BIN`) |
| `TestDiabloCtlOnlyItsOwnSite` | samples/adm/diablo-ctl, diablo | `diablo-ctl stop` acts on its own site only: another site of the same user, started first, keeps its diablo and its dnewslink worker (a stand-in in its dqueue); the site's own worker is stopped; diablo writes `log/diablo.pid` and removes it on exit. **Fails** on main before #31: diablo-ctl reported and killed the other site's diablo and dnewslink. The diablo-ctl tests skip an old diablo-ctl that still looks for "the oldest diablo of the user" |
| `TestCanLockVectors` | (Go only) | the Go Cancel-Lock helpers reproduce INN's test vectors (tests/lib/canlock-t.c) |
| `TestCancelLock`, `TestCancelLockSupersedes`, `TestCancelLockEarlyCancel`, `TestCancelLockTrackonlyRefused` | dreaderd | `readercanlock` (RFC 8315): cancels and supersedes against Cancel-Lock in each mode (require-auth, auth, none, all), matching, wrong and missing keys, sha1 in a folded lock with a comment, early cancels, and the trackonly refusal; skipped without readercanlock support |
| `TestDnewslinkConnlife` | diablo, dspoolout, dnewslink | realtime feed with `connlife 8` (skipped without `dnewslink -K`): one TCP session across a queue rotation; after connlife, retirement at the next rotation and a replacement at once (`dspoolout -J`); no descriptor growth over three replacements; every article reaches the peer once, in order. Linux only (reads /proc) |
| `TestDnewslinkConnlifeNearest` | diablo, dspoolout, dnewslink | with `connlife 12`, a replacement worker that carries traffic retires at the rotation 10 s after its start (the one nearest 12 s), not at the next one about 20 s in. **Fails** on main before #30: the worker is still serving 12 s after its start (connlife 60 gave about 120 s). Linux only (reads /proc) |
| `TestInstallConfigFiles` | install-portable.sh | `make install` into a temporary prefix: config files are real 0640 copies of the samples, `dactive.kp` is empty, adm scripts in `<prefix>/adm` keep the execute bit; a reinstall keeps a changed config file and puts a changed adm script's new version next to it (`.new`); uninstall leaves the configuration. Skipped without `install-portable.sh` next to `DIABLO_BIN` |
| `TestInstallConvertsConfigSymlinks` | install-portable.sh | a config symlink into `samples/` left by an earlier install becomes a real file with the content in use, before `samples/` is replaced; with install and with bininstall |
| `TestAdmHourlyExpire` | adm/hourly.expire, dexpire | the script expires an article older than `keeptime` |
| `TestAdmDailyAtrim` | adm/daily.atrim, adm/rot | the script rotates the logs |
| `TestAdmBiweeklyAtrim` | adm/biweekly.atrim, dhisexpire | the script trims the history while diablo runs; the stored article stays in history |
| `TestAdmReaderScripts` | adm/daily.reader, adm/weekly.reader | both scripts run on a reader site. The adm tests are skipped while `samples/adm` still holds the csh scripts |

## Running

You need a Diablo build and Go 1.26 or later. A user and group called `news`
must exist: `diablo` looks them up at startup and exits without them.

```sh
make                      # with RADIUS_ENABLED off in lib/vendor.h on Linux
cd tests/e2e
DIABLO_BIN=$PWD/../../dbin go test ./...
```

Without `DIABLO_BIN` every test is skipped. `NNTP_TEST_DEADLINE` (default
`60s`) bounds every blocking step, so a hung server fails the test instead of
hanging it.

Prefer a test machine. Each test starts its own daemons in a temporary
directory on free ports, and the tests only signal processes of their own
sites. But they run as `news`, like a live site, and a bug there can reach the
live daemons: before jrehmer/diablo-nntp#31, `TestDiabloCtl*` (through
diablo-ctl) stopped the live diablo, dreaderd and dnewslink of the same user.

Run the tests as an ordinary user or as `news`, not as root. As an ordinary
user, `diablo`'s switch to `news` fails and it keeps running as that user,
which owns the test sites.

The tests need a system where 127.0.0.2 can be used as a source address.
Linux routes all of 127.0.0.0/8 to the loopback interface. Elsewhere add the
address first:

- FreeBSD and macOS: `ifconfig lo0 alias 127.0.0.2/32`
- illumos (SmartOS, OmniOS): `ipadm create-addr -T static -a 127.0.0.2/32 lo0/e2e`

`TestDnewslinkConnlife` reads `/proc` and is skipped off Linux.

`ci/build-on-host` installs the build packages (apt, dnf, zypper, pacman, apk or emerge), creates the
`news` user, turns RADIUS off and builds; run it as root from the top of a
throwaway copy of the source tree. The test binary can be built elsewhere
with `CGO_ENABLED=0 go test -c` and copied to a machine without Go.
`.github/workflows/e2e.yml` does the same on GitHub's Ubuntu runners.

## Writing a test

```go
func TestSomething(t *testing.T) {
	s := NewSite(t, SiteOptions{})      // diablo with DefaultNewsfeeds: 127.0.0.2 may feed
	p := s.Peer(t)                      // dial from 127.0.0.2, read the greeting
	id := msgID(t)
	expect(t, p, 335, "IHAVE %s", id)
	send(t, p, wire(newArticle(id, "e2e.test", "subject", "body")))
	status(t, p, 235, "IHAVE article")
	s.Dicmd(t, "status")                // dicmd with the site's DIABLO_CONFIG_PATH
}
```

`SiteOptions` adds or replaces `diablo.config` directives, replaces
`dnewsfeeds` or `dspool.ctl`, writes further files, and runs a `Setup`
function before `diablo` starts (e.g. `PrimeHostCache`).
