package e2e

import (
	"bufio"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// shimMarkC is the C function shim_mark(fmt, ...) for the dreaderd fault injectors (the text
// /*MARK*/ in their source, after the includes): it appends one line, the process id and the
// formatted text, to the file named by SHIM_MARK. The test reads it back with shimMarks()
// and so knows that the injector acted, and in which process.
const shimMarkC = `
static void
shim_mark(const char *fmt, ...)
{
    const char *p = getenv("SHIM_MARK");
    char line[512];
    va_list ap;
    int fd;
    int n;

    if (p == NULL)
	return;
    n = snprintf(line, sizeof(line), "%ld ", (long)getpid());
    va_start(ap, fmt);
    vsnprintf(line + n, sizeof(line) - n - 1, fmt, ap);
    va_end(ap);
    n = strlen(line);
    line[n++] = '\n';
    if ((fd = open(p, O_WRONLY|O_CREAT|O_APPEND, 0644)) >= 0) {
	(void)write(fd, line, n);
	close(fd);
    }
}
`

// withMark puts shimMarkC into a shim source at its /*MARK*/ mark.
func withMark(src string) string {
	return strings.Replace(src, "/*MARK*/", shimMarkC, 1)
}

// shimMarks returns the lines the injector wrote to the mark file (none when it never acted).
func shimMarks(path string) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return strings.Split(strings.TrimRight(string(b), "\n"), "\n")
}

// shimMarked returns the mark lines that contain what.
func shimMarked(path, what string) []string {
	var m []string
	for _, l := range shimMarks(path) {
		if strings.Contains(l, what) {
			m = append(m, l)
		}
	}
	return m
}

// startReaderArgs starts dreaderd -d for a reader site (written with NoStart) with the
// further arguments args, in its own process group (the cleanup kills the group), and does
// not wait for a greeting.
func startReaderArgs(tb testing.TB, s *Site, args ...string) {
	tb.Helper()
	port := s.Addr[strings.LastIndex(s.Addr, ":")+1:]
	a := append([]string{"-d", "-p", PathHost, "-h", PathHost, "-P", "[127.0.0.1]:" + port}, args...)
	runDetached(tb, s, exec.Command(filepath.Join(s.Bin, "dreaderd"), a...))
}

// waitGreeting dials addr until a line comes back or d runs out, and returns that line.
func waitGreeting(addr string, d time.Duration) string {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if g := readerGreeting(addr); g != "" {
			return g
		}
		time.Sleep(200 * time.Millisecond)
	}
	return ""
}

// readToEOF reads what conn sends until it closes the connection or d runs out. eof tells
// whether the connection was closed (rather than the time running out).
func readToEOF(conn net.Conn, d time.Duration) (text string, eof bool) {
	conn.SetReadDeadline(time.Now().Add(d))
	var b strings.Builder
	r := bufio.NewReader(conn)
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		b.Write(buf[:n])
		if errors.Is(err, io.EOF) {
			return b.String(), true
		}
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				return b.String(), false
			}
			return b.String(), true // reset: closed as well
		}
	}
}
