package e2e

import (
	"bufio"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// readLockHolder takes a shared (read) fcntl lock on bytes 0-3 of file, as ReadAccessCache()
// does while it opens the cache files, and holds it until release is called.
func readLockHolder(t *testing.T, file string) (release func()) {
	t.Helper()
	cc := wmCC(t)
	dir := t.TempDir()
	c := filepath.Join(dir, "rholder.c")
	bin := filepath.Join(dir, "rholder")
	src := `#include <fcntl.h>
#include <stdio.h>
#include <string.h>
#include <unistd.h>
int
main(int ac, char **av)
{
    struct flock fl;
    char c;
    int fd = open(av[1], O_RDONLY);

    memset(&fl, 0, sizeof(fl));
    fl.l_type = F_RDLCK;
    fl.l_whence = SEEK_SET;
    fl.l_len = 4;
    if (fd < 0 || fcntl(fd, F_SETLKW, &fl) < 0) {
	perror("lock");
	return 1;
    }
    printf("locked\n");
    fflush(stdout);
    while (read(0, &c, 1) > 0)
	;
    return 0;
}
`
	if err := os.WriteFile(c, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(cc, "-o", bin, c).CombinedOutput(); err != nil {
		t.Fatalf("build lock holder: %v\n%s", err, out)
	}
	cmd := exec.Command(bin, file)
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if line, err := bufio.NewReader(out).ReadString('\n'); err != nil || line != "locked\n" {
		t.Fatalf("lock holder: %q %v", line, err)
	}
	done := false
	release = func() {
		if !done {
			done = true
			in.Close()
			cmd.Wait()
		}
	}
	t.Cleanup(release)
	return release
}

// firstLine connects to addr and returns the first line (the greeting, or a refusal).
func firstLine(t *testing.T, addr string) string {
	t.Helper()
	c, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetReadDeadline(time.Now().Add(10 * time.Second))
	l, err := bufio.NewReader(c).ReadString('\n')
	if err != nil {
		t.Fatalf("first line: %v", err)
	}
	return strings.TrimSpace(l)
}

// TestReaderAccessCacheRenameWaitsForReaders: dreaderd installs a changed dreader.access by
// renaming five new cache files over the old ones, under the lock that readers take (shared)
// while they open the five files. The installer took a shared lock too, so a reader could open
// some old files and some new ones. It now takes the lock exclusively: while a reader holds
// its shared lock (here a helper process), the files stay as they are; once it lets go, the
// new rules are installed and apply. A wait of more than 10 s is logged.
func TestReaderAccessCacheRenameWaitsForReaders(t *testing.T) {
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool,
		Config: map[string]string{"path_generallog": "%s/general.log"}})
	if l := firstLine(t, s.Addr); !strings.HasPrefix(l, "20") {
		t.Fatalf("greeting: %q", l)
	}
	ino := func() uint64 {
		fi, err := os.Stat(filepath.Join(s.Dir, "cache.readeraccess"))
		if err != nil {
			t.Fatal(err)
		}
		return uint64(fi.Sys().(*syscall.Stat_t).Ino)
	}
	before := ino()

	release := readLockHolder(t, filepath.Join(s.Dir, ".accesslock"))
	// the lock file's time has one-second steps: let the next change be a new second
	time.Sleep(1100 * time.Millisecond)
	// 127.0.0.1 may no longer read
	s.WriteFile(t, "dreader.access", strings.Replace(DefaultReaderAccess,
		"readerdef rp\n    read yes\n    post yes\n", "readerdef rp\n    read no\n    post no\n", 1))

	// each connection makes the master look at dreader.access again
	for i := 0; i < 4; i++ {
		l := firstLine(t, s.Addr)
		if ino() != before {
			t.Fatalf("the cache files were renamed while a reader held the shared lock")
		}
		if !strings.HasPrefix(l, "20") {
			t.Fatalf("connection %d while the lock is held: %q, want the old rules", i, l)
		}
		time.Sleep(300 * time.Millisecond)
	}

	// after 10 s the wait is logged
	deadline := time.Now().Add(15 * time.Second)
	for !strings.Contains(generalLog(s), "access cache install waits") {
		if time.Now().After(deadline) {
			t.Fatalf("the long wait for the lock was not logged:\n%s", generalLog(s))
		}
		firstLine(t, s.Addr)
		if ino() != before {
			t.Fatalf("the cache files were renamed while a reader held the shared lock")
		}
		time.Sleep(500 * time.Millisecond)
	}

	release()
	deadline = time.Now().Add(10 * time.Second)
	for {
		l := firstLine(t, s.Addr)
		if ino() != before && !strings.HasPrefix(l, "20") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the new dreader.access was not installed after the lock was released (last %q)", l)
		}
		time.Sleep(200 * time.Millisecond)
	}
}
