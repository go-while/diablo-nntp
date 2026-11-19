package e2e

import (
	"bufio"
	"io"
	"net"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
)

// byteLimit is the session byte limit of the local reader in byteLimitAccess: more than the
// greeting, less than the first reply of the test.
const byteLimit = 300

var byteLimitAccess = accessWith("bytelimit " + strconv.Itoa(byteLimit))

// byteLimitGroupReady opens a fresh connection (so the byte limit is not reached) and reports
// whether GROUP e2e.test shows an article.
func byteLimitGroupReady(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return false
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	r := bufio.NewReader(conn)
	if g, err := r.ReadString('\n'); err != nil || !strings.HasPrefix(g, "20") {
		return false
	}
	if _, err := io.WriteString(conn, "GROUP e2e.test\r\n"); err != nil {
		return false
	}
	line, err := r.ReadString('\n')
	if err != nil {
		return false
	}
	f := strings.Fields(line)
	io.WriteString(conn, "QUIT\r\n")
	return len(f) > 1 && f[0] == "211" && f[1] != "0"
}

// TestReaderByteLimitStopsPipelined (D-245): with a readerdef byte limit, the client sends,
// in one write right after the connect (before it reads the greeting), a first command whose
// reply takes the session over the limit, DATE three times and QUIT. dreaderd answers the
// first command, sends "400 ... Session byte limit reached." and closes the connection: none
// of the commands after it is answered.
//
// "spool": the first command is ARTICLE, fetched from the spool server (readercache off); its
// body is checked line by line. "local": the first command is HELP. The test checks that each
// first reply alone is over the limit.
//
// Before this fix NNCommand() went on into NNCommand2() after NNTerminate(), and the first
// DATE was answered (111) after the 400, followed by a second 400. In runs on main the spool
// subtest failed that way every time, the local subtest only in some runs; it stays as a
// guard.
func TestReaderByteLimitStopsPipelined(t *testing.T) {
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: byteLimitAccess})
	id := msgID(t)
	var body []string
	for i := 0; i < 60; i++ {
		body = append(body, strings.Repeat("x", 70)) // about 4.3 KB, far over the limit
	}
	feedReader(t, s, st, "e2e.test", newArticle(id, "e2e.test", "byte limit", body...))
	testkit.WaitFor(t, testkit.Deadline(), "the article in GROUP e2e.test", func() bool {
		return byteLimitGroupReady(s.Addr)
	})

	for _, tc := range []struct {
		name, first string
		code        string
	}{
		{"spool", "ARTICLE " + id, "220"},
		{"local", "HELP", "100"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conn, err := net.DialTimeout("tcp", s.Addr, 5*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			conn.SetDeadline(time.Now().Add(testkit.Deadline()))
			// the commands go out right after the connect, before the greeting is read
			if _, err := io.WriteString(conn, tc.first+"\r\nDATE\r\nDATE\r\nDATE\r\nQUIT\r\n"); err != nil {
				t.Fatal(err)
			}
			r := bufio.NewReader(conn)
			g, err := r.ReadString('\n')
			if err != nil || !strings.HasPrefix(g, "20") {
				t.Fatalf("greeting %q, %v", g, err)
			}
			// read everything up to the close; size[i] is line i's length on the wire
			var lines []string
			var size []int
			for {
				line, err := r.ReadString('\n')
				if line != "" {
					lines = append(lines, strings.TrimRight(line, "\r\n"))
					size = append(size, len(line))
				}
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatalf("the connection was not closed after the session byte limit (%v); read so far: %q", err, lines)
				}
			}
			if len(lines) == 0 || !strings.HasPrefix(lines[0], tc.code+" ") {
				t.Fatalf("first reply %q, want %s; all: %q", lines, tc.code, lines)
			}
			dot := -1
			for i, l := range lines {
				if l == "." {
					dot = i
					break
				}
			}
			if dot < 0 {
				t.Fatalf("the reply to %q has no end: %q", tc.first, lines)
			}
			replyBytes := 0
			for _, n := range size[:dot+1] {
				replyBytes += n
			}
			if replyBytes <= byteLimit {
				t.Fatalf("the reply to %q is %d bytes, not over the limit of %d on its own", tc.first, replyBytes, byteLimit)
			}
			if tc.name == "spool" {
				blank := slices.Index(lines[:dot], "")
				if blank < 0 || !slices.Equal(lines[blank+1:dot], body) {
					t.Fatalf("the article's body is not complete before the 400: %q", lines[:dot])
				}
			}
			rest := lines[dot+1:]
			if len(rest) == 0 || !strings.HasPrefix(rest[0], "400 ") || !strings.Contains(rest[0], "Session byte limit reached") {
				t.Fatalf("after the reply to %q: %q, want the 400 of the session byte limit", tc.first, rest)
			}
			if len(rest) > 1 {
				t.Errorf("commands after the session byte limit were answered: %q", rest[1:])
			}
		})
	}
}

// TestReaderByteLimitRateLimitNoSpin: a readerdef with `bytelimit 300` and `ratelimit 2000`
// (all reply types); ARTICLE of a 12 KB article from the spool server passes the limit after
// its first 2000 bytes, and the rest goes out at the rate, about six seconds, while the
// connection is terminated (NNTerminate()). The reader processes must not spend that time
// on the CPU: their CPU time (utime + stime in /proc/PID/stat, Linux only) over the transfer
// must stay under one second. Before the change in NNTerminate(), it set the write bit again
// while the ratelimit timer held the output, so select() returned at once for the writable
// socket, pass after pass, and a reader process was on the CPU for about the whole transfer.
// The limit of one second is about a fifth of that busy time, so that a loaded host neither
// fails the fix nor lets a busy loop pass. The
// transfer must take at least two seconds, or the ratelimit did not hold the reply and the
// test has lost its trigger.
func TestReaderByteLimitRateLimitNoSpin(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("reads the CPU time of the reader processes from /proc (Linux only)")
	}
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: accessWith("bytelimit 300\n    ratelimit 2000")})
	id := msgID(t)
	var body []string
	for i := 0; i < 170; i++ {
		body = append(body, strings.Repeat("y", 70)) // about 12 KB
	}
	feedReader(t, s, st, "e2e.test", newArticle(id, "e2e.test", "byte limit and rate limit", body...))
	testkit.WaitFor(t, testkit.Deadline(), "the article in GROUP e2e.test", func() bool {
		return byteLimitGroupReady(s.Addr)
	})

	ticks := func() map[int]int {
		m := map[int]int{}
		for _, pid := range sitePids(t, s, "dreaderd") {
			if n := cpuTicks(pid); n >= 0 {
				m[pid] = n
			}
		}
		return m
	}
	conn, err := net.DialTimeout("tcp", s.Addr, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(60 * time.Second)) // the transfer takes about 6 s
	r := bufio.NewReader(conn)
	if g, err := r.ReadString('\n'); err != nil || !strings.HasPrefix(g, "20") {
		t.Fatalf("greeting %q, %v", g, err)
	}
	before := ticks()
	start := time.Now()
	if _, err := io.WriteString(conn, "ARTICLE "+id+"\r\n"); err != nil {
		t.Fatal(err)
	}
	var lines []string
	for {
		line, err := r.ReadString('\n')
		if line != "" {
			lines = append(lines, strings.TrimRight(line, "\r\n"))
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("the connection was not closed after the reply (%v); %d lines read", err, len(lines))
		}
	}
	took := time.Since(start)
	after := ticks()
	used := 0
	for pid, n := range after {
		used += n - before[pid] // a fork that started meanwhile counts whole
	}
	t.Logf("reply and close took %v; reader processes used %d clock ticks of CPU time", took.Round(10*time.Millisecond), used)

	dot := slices.Index(lines, ".")
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "220 ") || dot < 0 {
		t.Fatalf("no complete 220 reply: %d lines, first %q", len(lines), lines[:min(len(lines), 1)])
	}
	if blank := slices.Index(lines[:dot], ""); blank < 0 || !slices.Equal(lines[blank+1:dot], body) {
		t.Fatalf("the article's body is not complete")
	}
	if rest := lines[dot+1:]; len(rest) != 1 || !strings.Contains(rest[0], "Session byte limit reached") {
		t.Fatalf("after the article: %q, want only the 400 of the session byte limit", rest)
	}
	if took < 2*time.Second {
		t.Fatalf("the reply came in %v: the ratelimit did not hold it, the test has lost its trigger", took)
	}
	// USER_HZ is 100 on Linux: 100 ticks are one second
	if used > 100 {
		t.Errorf("the reader processes used %d clock ticks (%.2f s) of CPU time while the rate-limited rest of the reply went out in %v: a busy loop",
			used, float64(used)/100, took.Round(10*time.Millisecond))
	}
}
