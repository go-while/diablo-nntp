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
