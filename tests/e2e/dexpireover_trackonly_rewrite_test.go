package e2e

// D-235: dexpireover -R on a group written with
// "readerxover trackonly" (util/dexpireover.c rewriteDataFile, RewriteData).

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
)

// trackonlyConfig is the diablo.config of a trackonly reader: on a build with Cancel-Lock
// support dreaderd refuses trackonly with the default readercanlock, a build without it does
// not know the directive.
func trackonlyConfig(tb testing.TB) map[string]string {
	tb.Helper()
	cfg := map[string]string{"readerxover": "trackonly"}
	if b, err := os.ReadFile(filepath.Join(bin(tb), "dreaderd")); err == nil && bytes.Contains(b, []byte("readercanlock")) {
		cfg["readercanlock"] = "none"
	}
	return cfg
}

// groupLine is the reply to GROUP e2e.test.
func groupLine(t *testing.T, s *Site) string {
	t.Helper()
	c := s.Reader(t)
	defer c.Close()
	_, line := c.Cmd("GROUP e2e.test")
	return line
}

func waitGroup(t *testing.T, s *Site, want string) {
	t.Helper()
	testkit.WaitFor(t, testkit.Deadline(), "GROUP e2e.test "+want, func() bool {
		return strings.HasPrefix(groupLine(t, s), want+" ")
	})
}

func restartReader(t *testing.T, s *Site) {
	t.Helper()
	startReader(t, s, "[127.0.0.1]:"+strings.TrimPrefix(s.Addr, "127.0.0.1:"))
}

// expireOver runs dexpireover with dreaderd stopped: dreaderd's feed process keeps a shared
// lock on the index of a group it wrote to until its next flush (every 5 minutes in a
// feed-only process), and -R needs the exclusive lock. The run must exit 0 and report no
// failure and no dropped record.
func expireOver(t *testing.T, s *Site, args ...string) string {
	t.Helper()
	out, err := s.TryRun("dexpireover", args...)
	t.Logf("dexpireover %s: err %v\n%s", strings.Join(args, " "), err, out)
	if err != nil {
		t.Errorf("dexpireover %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	for _, bad := range []string{"failed", "lost", "Bad zlen", "entry removed", "inflate error"} {
		if strings.Contains(out, bad) {
			t.Errorf("dexpireover %s reports %q:\n%s", strings.Join(args, " "), bad, out)
		}
	}
	return out
}

// overFilePath is the over file of e2e.test.
func overFilePath(t *testing.T, s *Site) string {
	t.Helper()
	home := strings.TrimSpace(s.Run(t, "dpath", "group"))
	f := strings.Fields(s.Run(t, "dgrpctl", "listgrouphashfile", "e2e.test"))
	if len(f) != 2 || !strings.HasPrefix(filepath.Base(f[0]), "over.0.") {
		t.Fatalf("dgrpctl listgrouphashfile e2e.test: %q", f)
	}
	return filepath.Join(home, f[0])
}

// checkSlots reads the index of e2e.test and checks the record of every article 1..n in its
// slot: the article number, a receive time, and no data offset (-1) for a trackonly record,
// an offset for a record with overview data.
func checkSlots(t *testing.T, s *Site, when string, n int, trackonly func(int) bool) {
	t.Helper()
	b, err := os.ReadFile(overFilePath(t, s))
	if err != nil || len(b) < 16 {
		t.Fatalf("%s: over file of e2e.test: %v (%d bytes)", when, err, len(b))
	}
	o := binary.NativeEndian
	head := int(o.Uint32(b[8:]))
	slots := (len(b) - head) / 32 // sizeof(OverArt)
	if head <= 0 || slots <= 0 {
		t.Fatalf("%s: over file of e2e.test: header %d, %d slots", when, head, slots)
	}
	bad := 0
	for a := 1; a <= n; a++ {
		p := head + (a%slots)*32
		artNo, seek, rcvd := int32(o.Uint32(b[p:])), int32(o.Uint32(b[p+4:])), int32(o.Uint32(b[p+24:]))
		ok := artNo == int32(a) && rcvd != 0
		if trackonly(a) {
			ok = ok && seek == -1
		} else {
			ok = ok && seek >= 0
		}
		if !ok {
			if bad < 5 {
				t.Errorf("%s: slot of article %d: oa_ArtNo %d oa_SeekPos %d oa_TimeRcvd %d", when, a, artNo, seek, rcvd)
			}
			bad++
		}
	}
	if bad > 0 {
		t.Errorf("%s: %d of %d index records of e2e.test are wrong", when, bad, n)
	}
}

func allTrackonly(int) bool { return true }

// TestDexpireoverRewriteTrackonly: with readerxover trackonly the index of a group holds the
// article numbers and no overview data (every record has no data offset). "dexpireover -a -R",
// which the shipped weekly.reader runs, must keep those records: every index record is still
// in its slot, GROUP still shows the twenty articles, the next "dexpireover -a" leaves the
// range alone, and a new article gets the next number. Fails before the fix: -R wrote an
// empty index, and the next run moved the low number past all twenty articles
// (GROUP 211 1 20 20).
func TestDexpireoverRewriteTrackonly(t *testing.T) {
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Config: trackonlyConfig(t)})
	for i := range 20 {
		feedReader(t, s, st, "e2e.test", newArticle(msgID(t), "e2e.test", "trackonly "+strconv.Itoa(i), "body"))
	}
	waitGroup(t, s, "211 20 1 20")
	checkSlots(t, s, "before dexpireover", 20, allTrackonly)

	s.Proc.Stop()
	expireOver(t, s, "-a", "-R", "-v")
	checkSlots(t, s, "after dexpireover -a -R", 20, allTrackonly)
	expireOver(t, s, "-a", "-v")
	restartReader(t, s)
	if g := groupLine(t, s); !strings.HasPrefix(g, "211 20 1 20 ") {
		t.Errorf("GROUP e2e.test after dexpireover -a -R and -a: %q, want 211 20 1 20", g)
	}

	// A new article goes on from 21, and a second -R keeps all of them.
	feedReader(t, s, st, "e2e.test", newArticle(msgID(t), "e2e.test", "trackonly 20", "body"))
	waitGroup(t, s, "211 21 1 21")
	s.Proc.Stop()
	expireOver(t, s, "-a", "-R", "-v")
	checkSlots(t, s, "after the second dexpireover -a -R", 21, allTrackonly)
	expireOver(t, s, "-a", "-v")
	restartReader(t, s)
	if g := groupLine(t, s); !strings.HasPrefix(g, "211 21 1 21 ") {
		t.Errorf("GROUP e2e.test after the second dexpireover -a -R and -a: %q, want 211 21 1 21", g)
	}
}

// TestDexpireoverRewriteMixedRecords: a group written with overview data (articles 1-6) and
// then, after a switch to readerxover trackonly, without (7-12); with four articles per data
// file one file holds both kinds (4-7). "dexpireover -a -R" keeps every record, the ones with
// data still point at their (rewritten) data, and OVER gives the same lines as before. With
// zlib: an empty .gz in place of a data file whose records have data is still a failure of
// -R (group left as it was, exit 1), not an empty range.
func TestDexpireoverRewriteMixedRecords(t *testing.T) {
	cfg := trackonlyConfig(t)
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool,
		Files: map[string]string{"dexpire.ctl": "e2e.*:x30:e4\n"}})
	for i := 1; i <= 6; i++ {
		feedReader(t, s, st, "e2e.test", newArticle(msgID(t), "e2e.test", "full "+strconv.Itoa(i), "body"))
	}
	waitGroup(t, s, "211 6 1 6")

	s.Proc.Stop()
	lines := strings.Split(strings.TrimSuffix(s.ReadFile(t, "diablo.config"), "\n"), "\n")
	var conf []string
	for _, l := range lines {
		if k, _, _ := strings.Cut(l, "\t"); cfg[k] == "" {
			conf = append(conf, l)
		}
	}
	for _, k := range sortedKeys(cfg) {
		conf = append(conf, k+"\t"+cfg[k])
	}
	s.WriteFile(t, "diablo.config", strings.Join(conf, "\n")+"\n")
	restartReader(t, s)
	for i := 7; i <= 12; i++ {
		feedReader(t, s, st, "e2e.test", newArticle(msgID(t), "e2e.test", "trackonly "+strconv.Itoa(i), "body"))
	}
	waitGroup(t, s, "211 12 1 12")
	mixed := func(a int) bool { return a > 6 }
	checkSlots(t, s, "before dexpireover", 12, mixed)
	over := func() []string {
		c := s.Reader(t)
		defer c.Close()
		c.ExpectCmd(211, "GROUP e2e.test")
		c.ExpectCmd(224, "OVER 1-12")
		return c.ReadDotLines()
	}
	before := over()
	if len(before) != 6 {
		t.Fatalf("OVER 1-12 before dexpireover: %d lines, want the 6 with overview data: %q", len(before), before)
	}

	s.Proc.Stop()
	expireOver(t, s, "-a", "-R", "-v")
	checkSlots(t, s, "after dexpireover -a -R", 12, mixed)
	expireOver(t, s, "-a", "-v")
	restartReader(t, s)
	if g := groupLine(t, s); !strings.HasPrefix(g, "211 12 1 12 ") {
		t.Errorf("GROUP e2e.test after dexpireover -a -R and -a: %q, want 211 12 1 12", g)
	}
	if after := over(); strings.Join(after, "\n") != strings.Join(before, "\n") {
		t.Errorf("OVER 1-12 after dexpireover -a -R differs:\nbefore %q\nafter  %q", before, after)
	}
	if !builtWithZlib(t) {
		return
	}

	// An empty .gz in place of the data file of articles 1-3, whose records have data: that
	// is damage, not a trackonly range. -R must leave the group as it is and fail, as it
	// does for any unreadable data file, and not drop the three records.
	s.Proc.Stop()
	overPath := overFilePath(t, s)
	plain := filepath.Join(filepath.Dir(overPath), "data.0."+strings.TrimPrefix(filepath.Base(overPath), "over.0."))
	if err := os.Remove(plain); err != nil {
		t.Fatalf("data file of articles 1-3: %v", err)
	}
	if err := os.WriteFile(plain+".gz", emptyGzip(t), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := s.TryRun("dexpireover", "-a", "-R", "-v")
	t.Logf("dexpireover -a -R -v with an empty .gz for records with data: err %v\n%s", err, out)
	if err == nil || !strings.Contains(out, "Rewrite of e2e.test data files failed, group left as it was") {
		t.Errorf("dexpireover -a -R with an empty .gz for records with data: err %v, want exit 1 and the group left as it was:\n%s", err, out)
	}
	checkSlots(t, s, "after dexpireover -a -R with an empty .gz for records with data", 12, mixed)
	if _, err := os.Stat(plain + ".gz"); err != nil {
		t.Errorf("the .gz of articles 1-3 is gone after the failed -R: %v", err)
	}
}

// emptyGzip is a gzip file of no bytes, as -z made of an empty data file.
func emptyGzip(t *testing.T) []byte {
	t.Helper()
	var gz bytes.Buffer
	if err := gzip.NewWriter(&gz).Close(); err != nil {
		t.Fatal(err)
	}
	return gz.Bytes()
}

// TestDexpireoverRewriteTrackonlyGzip: the data files of a trackonly group are empty (dreaderd
// and -R create them, nothing is written into them). "dexpireover -z" must not turn them into
// .gz files that a later -R cannot read, and an empty .gz that an earlier -z left behind is a
// range without data, not a failure: -R exits 0, keeps every record, and the next -a leaves
// the range alone. Fails before the fix: -z gzipped the empty files, and every later -R
// stopped on them ("Bad zlen 0", "Rewrite failed", exit 1).
func TestDexpireoverRewriteTrackonlyGzip(t *testing.T) {
	skipWithoutZlib(t)
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Config: trackonlyConfig(t),
		Files: map[string]string{"dexpire.ctl": "e2e.*:x30:e4:F1:Z1\n"}})
	for i := 1; i <= 12; i++ {
		feedReader(t, s, st, "e2e.test", newArticle(msgID(t), "e2e.test", "trackonly "+strconv.Itoa(i), "body"))
	}
	waitGroup(t, s, "211 12 1 12")
	s.Proc.Stop()

	overPath := overFilePath(t, s)
	dir, suffix := filepath.Dir(overPath), strings.TrimPrefix(filepath.Base(overPath), "over.0.")
	// -z compresses data files older than Z1 (one hour) outside the newest F1 file
	old := time.Now().Add(-2 * time.Hour)
	data, _ := filepath.Glob(filepath.Join(dir, "data.*."+suffix))
	for _, f := range data {
		if err := os.Chtimes(f, old, old); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("data files before -z: %q", data)
	// dreaderd made one empty data file per four articles; those of articles 1-3 and 4-7 are
	// old enough and outside the newest file, so -z looks at them
	for _, base := range []string{"0", "4"} {
		f := filepath.Join(dir, "data."+base+"."+suffix)
		if fi, err := os.Stat(f); err != nil || fi.Size() != 0 {
			t.Fatalf("empty data file of articles from %s before -z: %v %v", base, fi, err)
		}
	}
	zout := expireOver(t, s, "-y", "-z", "-v")
	// -z must leave the empty files as they are, not gzip them
	for _, f := range data {
		if fi, err := os.Stat(f); err != nil || fi.Size() != 0 {
			t.Errorf("empty data file %s after -z: %v %v, want it still there, empty", filepath.Base(f), fi, err)
		}
		if _, err := os.Stat(f + ".gz"); err == nil {
			t.Errorf("dexpireover -z gzipped the empty data file %s", filepath.Base(f))
		}
	}
	if n := strings.Count(zout, "not eligible for compression: empty\n"); n != 2 {
		t.Errorf("dexpireover -y -z -v: %d \"empty\" lines, want 2 (articles 1-3 and 4-7):\n%s", n, zout)
	}
	expireOver(t, s, "-a", "-R", "-v")
	checkSlots(t, s, "after dexpireover -z and -a -R", 12, allTrackonly)

	// An empty .gz from an earlier -z, in place of the plain file of articles 1-3.
	plain := filepath.Join(dir, "data.0."+suffix)
	if err := os.Remove(plain); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := os.WriteFile(plain+".gz", emptyGzip(t), 0o644); err != nil {
		t.Fatal(err)
	}
	expireOver(t, s, "-a", "-R", "-v")
	checkSlots(t, s, "after dexpireover -a -R with an empty .gz", 12, allTrackonly)
	expireOver(t, s, "-a", "-v")
	restartReader(t, s)
	if g := groupLine(t, s); !strings.HasPrefix(g, "211 12 1 12 ") {
		t.Errorf("GROUP e2e.test after the runs: %q, want 211 12 1 12", g)
	}
}
