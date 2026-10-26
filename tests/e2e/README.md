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
| `TestReaderFeedAndRead` | dreaderd, dgrpctl | an article fed to the reader by IHAVE shows in GROUP and OVER; BODY and ARTICLE fetch it from the spool server (fake) |
| `TestReaderQuitAlone` | dreaderd | QUIT gets 205 and nothing follows it |
| `TestReaderQuitWhileConnecting` | dreaderd | a client that QUITs while another connects gets 205 and nothing after it. **Fails** on main before jrehmer/diablo-nntp#12 (dreaderd wrote "400 Server too busy" after the 205 when its only DNS worker was busy) |
| `TestReaderPost` | dreaderd | POST is passed to the outgoing ('o') server (fake peer) with IHAVE: body as posted, the vserver's `postpath` in the Path |
| `TestReaderBindHostPort` | dreaderd | `dreaderd -P 127.0.0.1:port` listens there. **Fails** on main before the -P fix: the whole "host:port" was passed to getaddrinfo |
| `TestHeaderFeedToReader` | diablo, dnewslink, dreaderd | diablo feeds dreaderd a header-only feed (`headfeed on`, `dnewslink -H`); the reader shows the article in GROUP and OVER and fetches BODY from diablo as its spool server |
| `TestDexpire` | diablo, dexpire | `dexpire -a` removes the spool directory of an article older than `keeptime`; STAT then gives 430, and offering the article again gets 435 (still in history) |
| `TestDiabloCtlFeeder`, `TestDiabloCtlReader` | samples/adm/diablo-ctl | status (3 when stopped), start, a second start ("already running"), restart, stop (port closed), stop again; skipped without the script (`DIABLO_CTL`, default: `samples/adm/diablo-ctl` next to `DIABLO_BIN`) |
| `TestDnewslinkConnlife` | diablo, dspoolout, dnewslink | realtime feed with `connlife 8` (skipped without `dnewslink -K`): one TCP session across a queue rotation; after connlife, retirement at the next rotation and a replacement at once (`dspoolout -J`); no descriptor growth over three replacements; every article reaches the peer once, in order. Linux only (reads /proc) |

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

Run the tests as an ordinary user or as `news`, not as root. As an ordinary
user, `diablo`'s switch to `news` fails and it keeps running as that user,
which owns the test sites.

The tests need a system where 127.0.0.2 can be used as a source address
(Linux routes all of 127.0.0.0/8 to the loopback interface; on FreeBSD and
macOS add the alias first, e.g. `ifconfig lo0 alias 127.0.0.2`).

`ci/build-on-host` installs the build packages (apt, dnf or apk), creates the
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
