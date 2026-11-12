package e2e

import (
	"bufio"
	"fmt"
	"net"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
)

// TestReaderNewnewsWildmat: the newsgroups argument of NEWNEWS is a wildmat (RFC 3977 4.2,
// 7.4): the comma-separated patterns are tried in order, the last one that matches a group
// decides, and "!" before a pattern excludes the groups it matches. dreaderd matched each
// piece on its own: "!pattern" was taken as a group name, so the groups it should exclude
// were listed, and a group matched by two pieces was listed twice, with each message-id
// printed twice. Now each group is listed once, as the whole wildmat decides.
func TestReaderNewnewsWildmat(t *testing.T) {
	st, spool := spoolStore(t)
	access := strings.Replace(DefaultReaderAccess, "readerdef rp\n", "readerdef rp\n    allownewnews yes\n", 1)
	groups := []string{"comp.lang.c", "comp.lang.go", "comp.os.linux", "misc.test"}
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: access, Groups: groups})
	ids := map[string]string{}
	for _, g := range groups {
		id := msgID(t)
		ids[g] = id
		feedReader(t, s, st, g, newArticle(id, g, "wildmat "+g, "body"))
	}

	c := s.Reader(t)
	defer c.ExpectCmd(205, "QUIT")
	for _, g := range groups {
		testkit.WaitFor(t, 10*time.Second, "the article in "+g, func() bool {
			_, line := c.Cmd("GROUP %s", g)
			return strings.HasPrefix(line, "211 1 ")
		})
	}

	for _, tc := range []struct {
		wildmat string
		want    []string // groups whose article is listed, once each
	}{
		{"*", groups},
		{"comp.*,!comp.lang.go", []string{"comp.lang.c", "comp.os.linux"}},
		{"comp.*,comp.lang.c", []string{"comp.lang.c", "comp.lang.go", "comp.os.linux"}},
		{"comp.lang.c,comp.lang.c", []string{"comp.lang.c"}},
		{"comp.lang.*,!comp.lang.*,comp.lang.go", []string{"comp.lang.go"}},
		{"*,!comp.*", []string{"misc.test"}},
		{"*,!comp.lang.?o,!misc.*", []string{"comp.lang.c", "comp.os.linux"}},
		{"!comp.*", nil},
		// plain names among wild pieces: the place in the list decides, not the kind of piece
		{"!*,comp.lang.c", []string{"comp.lang.c"}},
		{"comp.lang.c,!comp.*", nil},
		{"*,misc.test,misc.test", groups},
		{"misc.test,!misc.*,misc.test", []string{"misc.test"}},
		{"misc.test,misc.*,!misc.test", nil},
		{"comp.*,!comp.lang.*,comp.lang.go,nosuch.group", []string{"comp.lang.go", "comp.os.linux"}},
		{"comp.lang.c,comp.lang.go,!comp.lang.?,comp.lang.c", []string{"comp.lang.c", "comp.lang.go"}},
	} {
		c.ExpectCmd(230, "NEWNEWS %s 000101 000000 GMT", tc.wildmat)
		got := c.ReadDotLines()
		var want []string
		for _, g := range tc.want {
			want = append(want, ids[g])
		}
		slices.Sort(want)
		sorted := slices.Clone(got)
		slices.Sort(sorted)
		if !slices.Equal(sorted, want) {
			t.Errorf("NEWNEWS %s: %q, want each of %q once (groups %q)", tc.wildmat, got, want, tc.want)
		}
	}
}

// TestReaderNewnewsManyPlainNames: a NEWNEWS line of plain group names (no '*', '?' or '!')
// lists each group once, and a long one does not hold the reader process. A line may be up
// to 1 MiB (CLIENT_MAXLINE), some 110,000 names, and dreaderd serves all the clients of a
// reader process (readerforks 1 here) from one loop. bbabbf8 dropped the duplicates by
// comparing each name with all the names before it: one such NEWNEWS took about 48 s on the
// test VMs, and another client's DATE waited as long. Before bbabbf8 (no duplicate check) the
// whole NEWNEWS ended within the 0.5 s the test waits before it sends the DATE, and the DATE
// was answered in under 1 ms.
//
// Bounds (checkNewnewsLongLine()): the NEWNEWS time runs from sending the line to the final
// "." and includes that 0.5 s wait; it must be at most 3 s. The DATE must be answered within
// 2 s of being sent (at worst it waits for the work of the NEWNEWS line). With a linear check
// the NEWNEWS takes 0.50 s and the DATE under 10 ms; the quadratic check takes about 50 s for
// both, so a host 15 times faster than the test VMs still fails it.
func TestReaderNewnewsManyPlainNames(t *testing.T) {
	const nNames = 110000
	st, spool := spoolStore(t)
	access := strings.Replace(DefaultReaderAccess, "readerdef rp\n", "readerdef rp\n    allownewnews yes\n", 1)
	groups := []string{"comp.lang.c", "misc.test"}
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: access, Groups: groups})
	ids := map[string]string{}
	for _, g := range groups {
		id := msgID(t)
		ids[g] = id
		feedReader(t, s, st, g, newArticle(id, g, "plain names "+g, "body"))
	}
	all := []string{ids["comp.lang.c"], ids["misc.test"]}
	slices.Sort(all)

	c := s.Reader(t)
	defer c.ExpectCmd(205, "QUIT")
	for _, g := range groups {
		testkit.WaitFor(t, 10*time.Second, "the article in "+g, func() bool {
			_, line := c.Cmd("GROUP %s", g)
			return strings.HasPrefix(line, "211 1 ")
		})
	}
	for _, tc := range []struct {
		names string
		want  []string
	}{
		{"comp.lang.c,misc.test,comp.lang.c,misc.test,comp.lang.c", all},
		{"misc.test,misc.test,misc.test", []string{ids["misc.test"]}},
		{"nosuch.group,comp.lang.c,nosuch.group", []string{ids["comp.lang.c"]}},
	} {
		c.ExpectCmd(230, "NEWNEWS %s 000101 000000 GMT", tc.names)
		got := c.ReadDotLines()
		sorted := slices.Clone(got)
		slices.Sort(sorted)
		if !slices.Equal(sorted, tc.want) {
			t.Errorf("NEWNEWS %s: %q, want each of %q once", tc.names, got, tc.want)
		}
	}

	// x0000000 .. x0109999, with the two real groups spread over the line, three and two times
	var sb strings.Builder
	sb.WriteString("NEWNEWS comp.lang.c")
	for i := 0; i < nNames; i++ {
		switch i {
		case 30000, 90000:
			sb.WriteString(",misc.test")
		case 55000:
			sb.WriteString(",comp.lang.c")
		}
		fmt.Fprintf(&sb, ",x%07d", i)
	}
	sb.WriteString(",comp.lang.c 000101 000000 GMT\r\n")

	checkNewnewsLongLine(t, s, sb.String(), nNames+5, "plain names", all)
}

// TestReaderNewnewsManyPlainNamesAndWild: the same line with one wild piece in front and a '!'
// piece among the names. ListActiveGroups() matched every piece against every active group
// once a piece was wild, so the plain names, which 4e8828c took without a scan, cost groups
// times 110,000 calls of WildCmp() in the reader's loop; the site has nGroups more groups for
// that to show (a news server has far more). The result is the one of the whole wildmat: the
// last piece that matches a group decides, so "!misc.*" in the middle of the line does not
// exclude misc.test, which is a plain piece again after it, and each article is listed once.
//
// Bounds as in TestReaderNewnewsManyPlainNames: fixed code takes 0.50 s for the NEWNEWS and
// under 10 ms for the DATE, the old code 13 to 15 s for both on the test VMs, so a host 4
// times faster than they are still fails the 3 s and 2 s bounds.
func TestReaderNewnewsManyPlainNamesAndWild(t *testing.T) {
	const (
		nNames  = 110000
		nGroups = 8000
		chunk   = 1000 // groups per dgrpctl run: adding a group gets slower the more there are
	)
	st, spool := spoolStore(t)
	access := strings.Replace(DefaultReaderAccess, "readerdef rp\n", "readerdef rp\n    allownewnews yes\n", 1)
	groups := []string{"comp.lang.c", "misc.test"}
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: access, Groups: groups, NoStart: true})
	// x0000000.g .. x0007999.g: no piece of the line matches them, but each piece shares up to
	// eight characters with some of them. dgrpctl takes 1.5 to 4.5 ms for each group (more the
	// more groups there are), so the test adds them in runs of 1000: the longest run took 4.5 s
	// on the test VMs (8.8 s with runs of 2000), against the 60 s limit of a command, which a
	// host 13 times slower would reach.
	for from := 0; from < nGroups; from += chunk {
		var cg strings.Builder
		for i := from; i < from+chunk && i < nGroups; i++ {
			fmt.Fprintf(&cg, "x%07d.g bulk group\n", i)
		}
		s.WriteFile(t, "bulk.groups", cg.String())
		s.Run(t, "dgrpctl", "checkgroups", "exec", filepath.Join(s.Dir, "bulk.groups"))
	}
	startReader(t, s, "[127.0.0.1]:"+s.Addr[strings.LastIndex(s.Addr, ":")+1:])
	ids := map[string]string{}
	for _, g := range groups {
		id := msgID(t)
		ids[g] = id
		feedReader(t, s, st, g, newArticle(id, g, "plain names and wild "+g, "body"))
	}
	all := []string{ids["comp.lang.c"], ids["misc.test"]}
	slices.Sort(all)

	c := s.Reader(t)
	defer c.ExpectCmd(205, "QUIT")
	for _, g := range groups {
		testkit.WaitFor(t, 10*time.Second, "the article in "+g, func() bool {
			_, line := c.Cmd("GROUP %s", g)
			return strings.HasPrefix(line, "211 1 ")
		})
	}

	// comp.*, then x0000000 .. x0109999 with misc.test twice spread over the line and
	// "!misc.*" between the two; comp.lang.c comes from comp.*
	var sb strings.Builder
	sb.WriteString("NEWNEWS comp.*")
	for i := 0; i < nNames; i++ {
		switch i {
		case 30000, 90000:
			sb.WriteString(",misc.test")
		case 60000:
			sb.WriteString(",!misc.*")
		}
		fmt.Fprintf(&sb, ",x%07d", i)
	}
	sb.WriteString(" 000101 000000 GMT\r\n")

	checkNewnewsLongLine(t, s, sb.String(), nNames+4, "plain names and wild pieces", all)
}

// checkNewnewsLongLine sends the one NEWNEWS line (nNames pieces, what says which kind) on a
// connection to the site's reader (readerforks 1), asks for the DATE on a second connection
// 0.5 s later and checks: the DATE is answered within 2 s, the NEWNEWS ends within 3 s of
// being sent (this includes the 0.5 s wait) and lists the message-ids in want once each.
func checkNewnewsLongLine(t *testing.T, s *Site, line string, nNames int, what string, want []string) {
	t.Helper()
	const (
		newnewsMax  = 3 * time.Second
		dateMax     = 2 * time.Second
		readTimeout = 120 * time.Second // a quadratic check ends in about 50 s
	)
	if len(line) > 1<<20 {
		t.Fatalf("the NEWNEWS line is %d bytes, more than CLIENT_MAXLINE", len(line))
	}
	dial := func() (net.Conn, *bufio.Reader) {
		nc, err := net.DialTimeout("tcp", s.Addr, 10*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { nc.Close() })
		r := bufio.NewReaderSize(nc, 1<<16)
		nc.SetReadDeadline(time.Now().Add(20 * time.Second))
		if l, err := r.ReadString('\n'); err != nil || !strings.HasPrefix(l, "20") {
			t.Fatalf("greeting %q %v", l, err)
		}
		return nc, r
	}
	other, otherR := dial()
	nn, nnR := dial()

	start := time.Now()
	nn.SetWriteDeadline(start.Add(readTimeout))
	if _, err := nn.Write([]byte(line)); err != nil {
		t.Fatalf("writing the NEWNEWS line (%d bytes): %v", len(line), err)
	}
	// give the reader the line, then ask the other client for the date
	time.Sleep(500 * time.Millisecond)
	other.SetReadDeadline(time.Now().Add(readTimeout))
	dateSent := time.Now()
	fmt.Fprintf(other, "DATE\r\n")
	l, err := otherR.ReadString('\n')
	dateTook := time.Since(dateSent)
	if err != nil || !strings.HasPrefix(l, "111 ") {
		t.Errorf("other client's DATE: %q %v after %v", l, err, dateTook)
	} else if dateTook > dateMax {
		t.Errorf("other client's DATE waited %v (more than %v) behind a NEWNEWS of %d %s", dateTook, dateMax, nNames, what)
	}

	nn.SetReadDeadline(start.Add(readTimeout))
	l, err = nnR.ReadString('\n')
	if err != nil || !strings.HasPrefix(l, "230 ") {
		t.Fatalf("NEWNEWS status %q %v after %v", l, err, time.Since(start))
	}
	var got []string
	for {
		l, err = nnR.ReadString('\n')
		if err != nil {
			t.Fatalf("NEWNEWS body: %v after %v (%d lines)", err, time.Since(start), len(got))
		}
		if l == ".\r\n" {
			break
		}
		got = append(got, strings.TrimRight(l, "\r\n"))
	}
	took := time.Since(start)
	t.Logf("NEWNEWS of %d names (%d bytes) took %v, the other client's DATE %v", nNames, len(line), took, dateTook)
	if took > newnewsMax {
		t.Errorf("a NEWNEWS of %d %s held the reader %v (more than %v)", nNames, what, took, newnewsMax)
	}
	sorted := slices.Clone(got)
	slices.Sort(sorted)
	if !slices.Equal(sorted, want) {
		t.Errorf("NEWNEWS of %d %s: %q, want each of %q once", nNames, what, got, want)
	}
}
