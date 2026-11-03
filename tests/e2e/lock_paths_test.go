package e2e

// The error paths #260 added for lock calls that fail, in the daemons and the tools.
// Each test fails one lock with ENOLCK through the LD_PRELOAD shim of
// lock_results_test.go (LOCKFAULT_FILE picks the file, LOCKFAULT_ON switches the
// fault on and off, LOCKFAULT_SKIP lets the first locks through) and checks what the
// program does then. Linux and illumos only.

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-while/nntp-testkit/fakeserver"
)

// startReaderEnv starts dreaderd for a reader site (written with NoStart) with env added
// to its environment; the programs the test runs later do not get env.
func startReaderEnv(t *testing.T, s *Site, env ...string) {
	t.Helper()
	base := s.Env
	s.Env = append(slices.Clone(base), env...)
	_, port, _ := strings.Cut(s.Addr, ":")
	startReader(t, s, "[127.0.0.1]:"+port)
	s.Env = base
}

// feedForks counts the feed processes dreaderd -d has forked so far: one more than
// readerfeedforks means one died and was replaced.
func feedForks(s *Site) int {
	return strings.Count(s.Proc.Stderr(), "forked feed child process")
}

// runWith runs a Diablo program of the site with env added and stdin, and returns its
// output and error.
func runWith(t *testing.T, s *Site, stdin string, env []string, name string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(filepath.Join(s.Bin, name), args...)
	cmd.Dir = s.Dir
	cmd.Env = append(append(os.Environ(), s.Env...), env...)
	cmd.Stdin = strings.NewReader(stdin)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func touch(t *testing.T, p string) {
	t.Helper()
	if err := os.WriteFile(p, nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

func fileSize(t *testing.T, p string) int64 {
	t.Helper()
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	return st.Size()
}

// TestReaderOverviewLockFailure: a reader feed whose overview lock fails gets "400 Error
// writing data". The data lock fails before the append; the index lock fails after it,
// and the append is rolled back. Either way the data file keeps its size, the feed
// process stays, and the articles before and after are in XOVER. LOCKFAULT_ON makes only
// the middle article fail. Fails before #260: both results were ignored, the article was
// written without the lock and got 235.
func TestReaderOverviewLockFailure(t *testing.T) {
	so := lockFaultShim(t)
	for _, tc := range []struct{ name, file, want string }{
		{"data", "/d.0.e2e.test", "error locking overview data for e2e.test"},
		{"index", "/o.0.e2e.test", "error locking overview index for e2e.test"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st, spool := spoolStore(t)
			s := NewReaderSite(t, ReaderOptions{Spool: spool, NoStart: true, Config: map[string]string{
				"readergrouphash": "hierarchy",
				"path_generallog": "%s/general.log",
			}})
			home := strings.TrimSpace(s.Run(t, "dpath", "group"))
			on := filepath.Join(t.TempDir(), "on")
			startReaderEnv(t, s, lockFaultEnv(so), "LOCKFAULT=enolck", "LOCKFAULT_FILE="+tc.file, "LOCKFAULT_ON="+on)

			ids := []string{msgID(t), msgID(t), msgID(t)}
			feedReader(t, s, st, "e2e.test", newArticle(ids[0], "e2e.test", "before", "body one"))
			data := filepath.Join(home, "e2e", "d.0.e2e.test")
			size := fileSize(t, data)
			if size == 0 {
				t.Fatalf("%s is empty after the first article", data)
			}

			touch(t, on)
			a := newArticle(ids[1], "e2e.test", "lock fails", "body two")
			head, body := splitArticle(a)
			st.AddArticle("e2e.test", fakeserver.Article{MessageID: ids[1], Headers: head, Body: body})
			p := s.dialFrom(t, PeerIP)
			expect(t, p, 335, "IHAVE %s", ids[1])
			send(t, p, wire(a))
			status(t, p, 400, "IHAVE with the overview "+tc.name+" lock failing")
			if err := os.Remove(on); err != nil {
				t.Fatal(err)
			}
			if got := fileSize(t, data); got != size {
				t.Errorf("overview data %d bytes after the refused article, want %d (not rolled back)", got, size)
			}
			if l := generalLog(s); !strings.Contains(l, tc.want) || strings.Contains(l, "Error in GetOverInfo") {
				t.Errorf("want %q (and not a GetOverInfo failure) in the log:\n%s", tc.want, l)
			}

			feedReader(t, s, st, "e2e.test", newArticle(ids[2], "e2e.test", "after", "body three"))
			r := s.Reader(t)
			expect(t, r, 211, "GROUP e2e.test")
			expect(t, r, 224, "XOVER 1-10")
			over := strings.Join(r.ReadDotLines(), "\n")
			for i, want := range []bool{true, false, true} {
				if strings.Contains(over, ids[i]) != want {
					t.Errorf("XOVER: article %d (%s) listed %v, want %v:\n%s", i+1, ids[i], !want, want, over)
				}
			}
			expect(t, r, 205, "QUIT")
			if n := feedForks(s); n != 1 {
				t.Errorf("%d feed processes forked, want 1 (the feed process died)", n)
			}
		})
	}
}

// TestReaderControlLogLockFailure: when the lock of a control log (log= in dcontrol.ctl)
// fails, the control message is still carried out, nothing is written to that log, and
// the feed process goes on with the next control message. Fails before #260: the result
// was ignored and the log was written without the lock.
func TestReaderControlLogLockFailure(t *testing.T) {
	so := lockFaultShim(t)
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{
		Spool:   spool,
		NoStart: true,
		Config:  map[string]string{"path_generallog": "%s/general.log"},
		Files:   map[string]string{"dcontrol.ctl": "newgroup:*:*:doit,log=newgroup.log\n"},
	})
	startReaderEnv(t, s, lockFaultEnv(so), "LOCKFAULT=enolck", "LOCKFAULT_FILE=/newgroup.log")
	exists := func(g string) bool {
		c := s.Reader(t)
		defer expect(t, c, 205, "QUIT")
		expect(t, c, 215, "LIST ACTIVE %s", g)
		return slices.ContainsFunc(c.ReadDotLines(), func(l string) bool { return strings.HasPrefix(l, g+" ") })
	}
	for _, g := range []string{"e2e.lockctl1", "e2e.lockctl2"} {
		a := newArticle(msgID(t), "e2e.test", "cmsg newgroup "+g, "For your newsgroups file:", g+"\tA test group.")
		i := slices.Index(a, "")
		a = slices.Insert(a, i, "Control: newgroup "+g)
		feedReader(t, s, st, "e2e.test", a)
		waitFor(t, 10*time.Second, g+" created with the control log lock failing", func() bool { return exists(g) })
	}
	if b, err := os.ReadFile(filepath.Join(s.Dir, "log", "newgroup.log")); len(b) != 0 {
		t.Errorf("control log written without its lock (%v):\n%s", err, b)
	}
	if n := strings.Count(generalLog(s), "Unable to lock control log newgroup.log"); n != 2 {
		t.Errorf("%d lines about the control log lock, want 2:\n%s", n, generalLog(s))
	}
	if n := feedForks(s); n != 1 {
		t.Errorf("%d feed processes forked, want 1 (the feed process died)", n)
	}
}

// feedOne offers one article from the peer and fails the test unless it is taken and
// stored.
func feedOne(t *testing.T, s *Site) {
	t.Helper()
	p := s.Peer(t)
	id := msgID(t)
	expect(t, p, 335, "IHAVE %s", id)
	send(t, p, wire(newArticle(id, "e2e.test", "stats", "body")))
	status(t, p, 235, "IHAVE "+id)
	expect(t, p, 223, "STAT %s", id)
	expect(t, p, 205, "QUIT")
}

// statsLockLog matches the log lines of lib/stats.c about a failed lock: the slot lock
// (no count) and the counter locks (with a count).
var statsLockLog = regexp.MustCompile(`Unable to lock feeder stats file: \S+ \(([^,)]*)(?:, (\d+) times)?\)`)

// TestFeedStatsLockFailure: the feed stats (feederrtstats) when their locks fail.
//
//	slot      The slot lock fails: the incoming child keeps its stats in memory (no
//	          record for the label in the file), logs it, and takes the article.
//	counter   The slot lock works and the counter locks fail (LOCKFAULT_SKIP=1): the
//	          child counts anyway, logs it with a count, and takes the article.
//	snapshot  dfeedinfo -P whose second record lock fails reports it and removes the
//	          partial snapshot.
//
// Before #260 the results were ignored. slot and counter fail there only for the log
// line and slot also for the record that was allocated without the lock; the article
// was taken either way, so what they guard there is that a lock error does not refuse
// articles. snapshot fails there: the snapshot was written in full without its locks.
func TestFeedStatsLockFailure(t *testing.T) {
	so := lockFaultShim(t)
	fault := []string{lockFaultEnv(so), "LOCKFAULT=enolck", "LOCKFAULT_FILE=/feedstats"}
	site := func(t *testing.T, env ...string) *Site {
		return NewSite(t, SiteOptions{
			Config: map[string]string{"feederrtstats": "label", "path_generallog": "%s/general.log"},
			Env:    env,
		})
	}
	peerLine := regexp.MustCompile(`(?m)^\S+ peer .* con=(\d+) `)

	t.Run("slot", func(t *testing.T) {
		s := site(t, fault...)
		feedOne(t, s) // the child logs before its greeting
		if m := statsLockLog.FindStringSubmatch(generalLog(s)); m == nil || m[2] != "" {
			t.Errorf("first stats lock line %q, want the slot lock line:\n%s", m, generalLog(s))
		}
		if out := s.Run(t, "dfeedinfo", "-i", "-r", "peer"); peerLine.MatchString(out) {
			t.Errorf("the label got a record in the feedstats file without the slot lock:\n%s", out)
		}
	})

	t.Run("counter", func(t *testing.T) {
		s := site(t, append(fault, "LOCKFAULT_SKIP=1")...)
		feedOne(t, s)
		if m := statsLockLog.FindStringSubmatch(generalLog(s)); m == nil || m[2] != "1" {
			t.Errorf("first stats lock line %q, want a counter lock line with count 1:\n%s", m, generalLog(s))
		}
		m := peerLine.FindStringSubmatch(s.Run(t, "dfeedinfo", "-i", "-r", "peer"))
		if m == nil || m[1] != "1" {
			t.Errorf("connection not counted with the counter lock failing: %q", m)
		}
	})

	t.Run("snapshot", func(t *testing.T) {
		s := site(t)
		c := s.Peer(t)
		expect(t, c, 205, "QUIT")
		path := filepath.Join(s.Dir, "feedstats")
		waitFor(t, 10*time.Second, "the peer's feedstats record", func() bool {
			out, _ := s.TryRun("dfeedinfo", "-i", "-r", "peer")
			return peerLine.MatchString(out)
		})
		out, err := runWith(t, s, "", append(fault, "LOCKFAULT_SKIP=1"), "dfeedinfo", "-Pfail")
		if err != nil {
			t.Errorf("dfeedinfo -P: %v", err)
		}
		if !strings.Contains(out, "no snapshot written") || strings.Contains(out, "records written") {
			t.Errorf("dfeedinfo -P with its second record lock failing:\n%s", out)
		}
		if fileExists(path + ".fail") {
			t.Errorf("partial snapshot %s left behind", path+".fail")
		}
		// the same without the fault: two records (the dummy and peer)
		if out := s.Run(t, "dfeedinfo", "-Pok"); !strings.Contains(out, "2 records written") {
			t.Errorf("dfeedinfo -P without a fault:\n%s", out)
		}
	})
}

// TestSpamFilterLockFailure: with the lock of the spam filter's slots failing, the filter
// lets the articles through and logs it, on the 1st, 2nd, 4th and 8th of 10 failures with
// the count. Then (LOCKFAULT_ON removed) the filter works again and refuses repeats.
// Fails before #260: the result was ignored, the slot was counted without its lock, and
// the filter refused the repeats.
func TestSpamFilterLockFailure(t *testing.T) {
	so := lockFaultShim(t)
	on := filepath.Join(t.TempDir(), "on")
	s := NewSite(t, SiteOptions{
		Config:    map[string]string{"internalfilter": "B3", "path_generallog": "%s/general.log"},
		Newsfeeds: DefaultNewsfeeds + "label ISPAM\n    addgroup *\nend\n",
		Env:       []string{lockFaultEnv(so), "LOCKFAULT=enolck", "LOCKFAULT_FILE=spam.body.cache", "LOCKFAULT_ON=" + on},
	})
	const n = 10
	touch(t, on)
	if got := feedSameBody(t, s, n, "identical body, lock failing"); got != n {
		t.Errorf("%d of %d articles with the same body stored with the slot lock failing, want all", got, n)
	}
	re := regexp.MustCompile(`SpamFilter: unable to lock slot \d+ of \S+spam\.body\.cache: [^(]*\((\d+) times\)`)
	var counts []string
	for _, m := range re.FindAllStringSubmatch(generalLog(s), -1) {
		counts = append(counts, m[1])
	}
	if want := []string{"1", "2", "4", "8"}; !slices.Equal(counts, want) {
		t.Errorf("spam filter lock failures logged with counts %q, want %q:\n%s", counts, want, generalLog(s))
	}

	if err := os.Remove(on); err != nil {
		t.Fatal(err)
	}
	if got := feedSameBody(t, s, 8, "identical body, lock working"); got >= 8 {
		t.Errorf("all 8 articles with the same body stored with the lock working: the filter does not work in this setup")
	}
}

// overReaderSite is a reader site (hierarchy layout) with article 1 in e2e.test and in
// e2e.other; it returns the site, the message-ID in e2e.test and the over.* file of
// e2e.test.
func overReaderSite(t *testing.T) (s *Site, id, over string) {
	t.Helper()
	st, spool := spoolStore(t)
	st.AddGroup("e2e.other", "other")
	s = NewReaderSite(t, ReaderOptions{Spool: spool, Config: map[string]string{
		"readergrouphash": "hierarchy",
		"path_generallog": "%s/general.log",
	}})
	id = msgID(t)
	feedReader(t, s, st, "e2e.test", newArticle(id, "e2e.test", "cancel me", "body"))
	feedReader(t, s, st, "e2e.other", newArticle(msgID(t), "e2e.other", "cancel me too", "body"))
	home := strings.TrimSpace(s.Run(t, "dpath", "group"))
	return s, id, filepath.Join(home, "e2e", "o.0.e2e.test")
}

// TestDcancelOverviewLockFailure: dcancel -r skips an article whose over.* lock fails
// (the shared lock when it opens the file, or the exclusive one for the cancel with
// LOCKFAULT_SKIP=1), says so, cancels the next line's article and exits 0. The over.*
// file of the skipped group is unchanged. Fails before #260: the result was ignored and
// the article was cancelled without the lock.
func TestDcancelOverviewLockFailure(t *testing.T) {
	so := lockFaultShim(t)
	for _, tc := range []struct {
		name string
		skip string
	}{{"open", "0"}, {"cancel", "1"}} {
		t.Run(tc.name, func(t *testing.T) {
			s, _, over := overReaderSite(t)
			before, err := os.ReadFile(over)
			if err != nil {
				t.Fatal(err)
			}
			out, err := runWith(t, s, "e2e.test:1\ne2e.other:1\n",
				[]string{lockFaultEnv(so), "LOCKFAULT=enolck", "LOCKFAULT_FILE=/o.0.e2e.test", "LOCKFAULT_SKIP=" + tc.skip},
				"dcancel", "-r")
			if err != nil {
				t.Errorf("dcancel -r: %v", err)
			}
			for _, want := range []string{"e2e.test: overview lock failed", "Unable to cancel 1 in e2e.test", "1 in e2e.other cancelled"} {
				if !strings.Contains(out, want) {
					t.Errorf("no %q in the output of dcancel -r:\n%s", want, out)
				}
			}
			if after, _ := os.ReadFile(over); !bytes.Equal(before, after) {
				t.Errorf("dcancel changed %s without its lock:\n%s", over, out)
			}
		})
	}
}

// TestDgrpctlMasscancelLockFailure: dgrpctl masscancel skips the group when its over.*
// file cannot be locked for reading, and skips an article whose exclusive lock for the
// cancel fails (LOCKFAULT_SKIP=1); it says so, exits 0 and leaves the file unchanged.
// Fails before #260: the result was ignored and the article was cancelled without the
// lock.
func TestDgrpctlMasscancelLockFailure(t *testing.T) {
	so := lockFaultShim(t)
	for _, tc := range []struct {
		name, skip, want string
	}{
		{"open", "0", "e2e.test: overview lock failed"},
		{"cancel", "1", ">> masscancel: overview lock failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, id, over := overReaderSite(t)
			before, err := os.ReadFile(over)
			if err != nil {
				t.Fatal(err)
			}
			out, err := runWith(t, s, id+"\n",
				[]string{lockFaultEnv(so), "LOCKFAULT=enolck", "LOCKFAULT_FILE=/o.0.e2e.test", "LOCKFAULT_SKIP=" + tc.skip},
				"dgrpctl", "masscancel", "e2e.test")
			if err != nil {
				t.Errorf("dgrpctl masscancel: %v", err)
			}
			if !strings.Contains(out, tc.want) || strings.Contains(out, ">> cancelled") {
				t.Errorf("dgrpctl masscancel with the over.* lock failing, want %q and no cancel:\n%s", tc.want, out)
			}
			if after, _ := os.ReadFile(over); !bytes.Equal(before, after) {
				t.Errorf("dgrpctl changed %s without its lock:\n%s", over, out)
			}
			// the control: without the fault the article is cancelled
			if t.Failed() {
				return
			}
			if out, _ := runWith(t, s, id+"\n", nil, "dgrpctl", "masscancel", "e2e.test"); !strings.Contains(out, ">> cancelled e2e.test:1") {
				t.Errorf("dgrpctl masscancel without a fault did not cancel:\n%s", out)
			}
		})
	}
}

// TestDspooloutSeqLockFailure: dspoolout skips a label whose .label.seq cannot be locked
// for this run: its queue file is not rotated and the sequence file is unchanged, and the
// other label is rotated. Fails before #260: the result was ignored and the queue file
// was rotated without the lock.
func TestDspooloutSeqLockFailure(t *testing.T) {
	so := lockFaultShim(t)
	feeds := GlobalLabel + `label peer
    hostname 127.0.0.1
    addgroup *
end
label out
    hostname 127.0.0.1
    addgroup *
end
`
	s := NewSite(t, SiteOptions{Newsfeeds: feeds, NoStart: true})
	s.WriteFile(t, "dqueue/peer", "D.00000000/B.0000 <a@b> 0,1\n")
	s.WriteFile(t, "dqueue/out", "D.00000000/B.0000 <c@d> 0,1\n")
	s.WriteFile(t, "dqueue/.peer.seq", "0 0 0\n")
	out, err := runWith(t, s, "", []string{lockFaultEnv(so), "LOCKFAULT=enolck", "LOCKFAULT_FILE=/.peer.seq"}, "dspoolout", "-n")
	if err != nil {
		t.Fatalf("dspoolout -n: %v\n%s", err, out)
	}
	if !strings.Contains(out, "dspoolout: unable to lock .peer.seq") {
		t.Errorf("dspoolout did not report the failed lock:\n%s", out)
	}
	if strings.Contains(out, "add peer.S") || !fileExists(filepath.Join(s.Dir, "dqueue", "peer")) ||
		fileExists(filepath.Join(s.Dir, "dqueue", "peer.S00000")) {
		t.Errorf("the queue of label peer was rotated without the lock:\n%s", out)
	}
	if got := s.ReadFile(t, "dqueue/.peer.seq"); got != "0 0 0\n" {
		t.Errorf(".peer.seq %q after the skipped label, want it unchanged", got)
	}
	if !strings.Contains(out, "add out.S00000") {
		t.Errorf("the queue of label out was not rotated:\n%s", out)
	}
}

// TestDexpirescoringLockFailure: dexpirescoring exits 1 when it cannot lock cache.hits,
// and leaves the file unchanged. Fails before #260: the result was ignored, the counters
// were decayed without the lock and it exited 0.
func TestDexpirescoringLockFailure(t *testing.T) {
	so := lockFaultShim(t)
	s, feed := scoringReaderSite(t, nil)
	path := filepath.Join(s.Dir, "cache.hits")
	feed("e2e.test", newArticle(msgID(t), "e2e.test", "scoring", "body"))
	if ok, d := readArticleOnce(t, s, "e2e.test", 1); !ok {
		t.Fatalf("first read: %s", d)
	}
	waitFor(t, 10*time.Second, "a scoreboard entry", func() bool { return len(cacheHitsEntries(t, path)) > 0 })
	time.Sleep(1100 * time.Millisecond) // -H 1 changes the header a second later
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out, err := runWith(t, s, "", []string{lockFaultEnv(so), "LOCKFAULT=enolck", "LOCKFAULT_FILE=/cache.hits"},
		"dexpirescoring", "-H", "1", "-v")
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != 1 {
		t.Errorf("dexpirescoring with the cache.hits lock failing: %v, want exit status 1\n%s", err, out)
	}
	if !strings.Contains(out, "Can not lock cache hits file") {
		t.Errorf("dexpirescoring did not report the failed lock:\n%s", out)
	}
	if after, _ := os.ReadFile(path); !bytes.Equal(before, after) {
		t.Errorf("dexpirescoring changed cache.hits without its lock:\n%s", out)
	}
	if out, err := runWith(t, s, "", nil, "dexpirescoring", "-H", "1", "-v"); err != nil {
		t.Errorf("dexpirescoring without a fault: %v\n%s", err, out)
	}
}
