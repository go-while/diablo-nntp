package e2e

import (
	"bufio"
	"fmt"
	"net"
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
// NEWNEWS took 0.5 s and the DATE under 1 ms.
//
// Bounds: the NEWNEWS must end within 10 s (20 times the 0.5 s of the linear code, for a
// slow or busy host; the quadratic check takes about 5 times as long) and the DATE must be
// answered within 5 s of being sent (at worst it waits for the whole 0.5 s NEWNEWS).
func TestReaderNewnewsManyPlainNames(t *testing.T) {
	const (
		nNames      = 110000
		newnewsMax  = 10 * time.Second
		dateMax     = 5 * time.Second
		readTimeout = 120 * time.Second // the quadratic check ends in about 50 s
	)
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
	if sb.Len() > 1<<20 {
		t.Fatalf("the NEWNEWS line is %d bytes, more than CLIENT_MAXLINE", sb.Len())
	}

	start := time.Now()
	nn.SetWriteDeadline(start.Add(readTimeout))
	if _, err := nn.Write([]byte(sb.String())); err != nil {
		t.Fatalf("writing the NEWNEWS line (%d bytes): %v", sb.Len(), err)
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
		t.Errorf("other client's DATE waited %v (more than %v) behind a NEWNEWS of %d plain names", dateTook, dateMax, nNames)
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
	t.Logf("NEWNEWS of %d names (%d bytes) took %v, the other client's DATE %v", nNames+5, sb.Len(), took, dateTook)
	if took > newnewsMax {
		t.Errorf("a NEWNEWS of %d plain names held the reader %v (more than %v)", nNames, took, newnewsMax)
	}
	sorted := slices.Clone(got)
	slices.Sort(sorted)
	if !slices.Equal(sorted, all) {
		t.Errorf("NEWNEWS of %d plain names: %q, want each of %q once", nNames, got, all)
	}
}
