package e2e

import (
	"bytes"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fdsetProgSource does what lib/defs.h:27-34 does on glibc (__FD_SETSIZE raised to
// LARGE_FD_SETSIZE, 4096, lib/vendor.h) and then FD_SET/FD_ISSET of the descriptor number
// given as the argument, so the compiler cannot fold it.
const fdsetProgSource = `#include <features.h>
#include <bits/types.h>
#undef __FD_SETSIZE
#define __FD_SETSIZE 4096
#include <sys/select.h>
#include <stdio.h>
#include <stdlib.h>

int
main(int ac, char **av)
{
    fd_set s;
    int fd = ac > 1 ? atoi(av[1]) : 0;

    FD_ZERO(&s);
    FD_SET(fd, &s);
    printf("fd %d set %d size %d\n", fd, FD_ISSET(fd, &s) ? 1 : 0, (int)sizeof(s));
    return 0;
}
`

// fdFillSource is preloaded into diablo: at start it raises the soft descriptor limit to
// 4096 (LimitNOFILE of samples/init/systemd/diablo.service), and at the first accept() after
// the file named by R2_FILL exists it opens /dev/null until descriptor 1030 is taken, so the
// accepted connection and the pipe diablo makes for it get numbers above 1024, as after
// about a thousand open connections.
const fdFillSource = `#define _GNU_SOURCE
#include <dlfcn.h>
#include <fcntl.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <sys/resource.h>
#include <sys/socket.h>
/*MARK*/
static int filled;

__attribute__((constructor)) static void
fdfill_init(void)
{
    struct rlimit rl;

    if (getrlimit(RLIMIT_NOFILE, &rl) == 0 && rl.rlim_cur < 4096) {
	rl.rlim_cur = (rl.rlim_max == RLIM_INFINITY || rl.rlim_max >= 4096) ? 4096 : rl.rlim_max;
	setrlimit(RLIMIT_NOFILE, &rl);
    }
    if (getrlimit(RLIMIT_NOFILE, &rl) == 0)
	shim_mark("nofile %ld", (long)rl.rlim_cur);
}

int
accept(int s, struct sockaddr *addr, socklen_t *len)
{
    static int (*real)(int, struct sockaddr *, socklen_t *);
    const char *flag = getenv("R2_FILL");
    int fd;

    if (real == NULL)
	real = (int (*)(int, struct sockaddr *, socklen_t *))dlsym(RTLD_NEXT, "accept");
    if (!filled && flag != NULL && access(flag, F_OK) == 0) {
	int last = -1;

	filled = 1;
	while ((fd = open("/dev/null", O_RDONLY)) >= 0 && fd < 1030)
	    last = fd;
	if (fd >= 0)
	    last = fd;
	shim_mark("filled to %d", last);
    }
    fd = real(s, addr, len);
    if (filled && fd >= 0)
	shim_mark("accepted %d", fd);
    return fd;
}
`

// TestR2FdSetFortify (deep review r2 H4): lib/defs.h raises glibc's __FD_SETSIZE to 4096, but
// with _FORTIFY_SOURCE glibc checks FD_SET/FD_ISSET of a variable descriptor in
// __fdelt_chk() against its own 1024 and aborts ("buffer overflow detected").
//
//   - program: the defs.h override in a small program built with -O2 -D_FORTIFY_SOURCE=2;
//     FD_SET of 1030 aborts where the libc has the check (skipped where it has not, e.g. musl).
//   - diablo: the feeder as built on this host, its descriptors filled to 1030 before a peer
//     connects; the master must stay up and serve the next connection. This fails only where
//     the build is fortified (the binary calls __fdelt_chk); elsewhere it passes.
func TestR2FdSetFortify(t *testing.T) {
	t.Run("program", func(t *testing.T) {
		if !preloadHost(false) {
			t.Skip("glibc check: Linux only here")
		}
		cc := wmCC(t)
		dir := t.TempDir()
		src := filepath.Join(dir, "fdset.c")
		prog := filepath.Join(dir, "fdset")
		if err := os.WriteFile(src, []byte(fdsetProgSource), 0o600); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command(cc, "-O2", "-D_FORTIFY_SOURCE=2", "-o", prog, src).CombinedOutput(); err != nil {
			t.Skipf("the defs.h override does not build here (no glibc bits/types.h?): %v\n%s", err, out)
		}
		if b, _ := os.ReadFile(prog); !bytes.Contains(b, []byte("__fdelt_chk")) {
			t.Skip("this libc has no fortified FD_SET (__fdelt_chk)")
		}
		if out, err := exec.Command(prog, "1000").CombinedOutput(); err != nil {
			t.Fatalf("FD_SET of 1000: %v\n%s", err, out)
		}
		out, err := exec.Command(prog, "1030").CombinedOutput()
		if err != nil {
			t.Errorf("FD_SET of 1030 with __FD_SETSIZE 4096 and _FORTIFY_SOURCE=2: %v\n%s", err, out)
		} else {
			t.Logf("FD_SET of 1030: %s", strings.TrimSpace(string(out)))
		}
	})

	// probe: what the tree's own lib/defs.h makes of FD_SETSIZE and MAXFDS with the build's
	// flags (XMakefile.inc: -O2 -I<tree>), logged only.
	t.Run("probe", func(t *testing.T) {
		cc := wmCC(t)
		root := wildmatSource(t)
		dir := t.TempDir()
		src := filepath.Join(dir, "probe.c")
		prog := filepath.Join(dir, "probe")
		code := "#include \"lib/defs.h\"\n#include <stdio.h>\nint main(void) {\n" +
			"#ifdef __USE_FORTIFY_LEVEL\n int fl = __USE_FORTIFY_LEVEL;\n#else\n int fl = -1;\n#endif\n" +
			" printf(\"FD_SETSIZE %d __FD_SETSIZE %d sizeof(fd_set) %d MAXFDS %d MAXFORKS %d fortify %d\\n\"," +
			" (int)FD_SETSIZE, (int)__FD_SETSIZE, (int)sizeof(fd_set), (int)MAXFDS, (int)MAXFORKS, fl);\n return 0;\n}\n"
		if err := os.WriteFile(src, []byte(code), 0o600); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command(cc, "-O2", "-I"+root, "-D_FILE_OFFSET_BITS=64", "-o", prog, src).CombinedOutput(); err != nil {
			t.Skipf("probe does not build: %v\n%.2000s", err, out)
		}
		out, err := exec.Command(prog).CombinedOutput()
		t.Logf("lib/defs.h with -O2: %s (err %v)", strings.TrimSpace(string(out)), err)
	})

	t.Run("diablo", func(t *testing.T) {
		b, err := os.ReadFile(filepath.Join(bin(t), "diablo"))
		if err != nil {
			t.Fatal(err)
		}
		fortified := bytes.Contains(b, []byte("__fdelt_chk"))
		t.Logf("diablo calls __fdelt_chk (fortified FD_SET): %v", fortified)
		so := preloadShim(t, "fdfill", withMark(fdFillSource))
		mark := filepath.Join(t.TempDir(), "mark")
		flag := filepath.Join(t.TempDir(), "fill")
		s := NewSite(t, SiteOptions{Env: []string{lockFaultEnv(so), "SHIM_MARK=" + mark, "R2_FILL=" + flag}})
		t.Cleanup(func() {
			if t.Failed() {
				t.Logf("marks:\n%s", strings.Join(shimMarks(mark), "\n"))
			}
		})
		m := shimMarked(mark, "nofile")
		n := 0
		if len(m) > 0 {
			f := strings.Fields(m[0])
			n, _ = strconv.Atoi(f[len(f)-1])
		}
		if n < 1100 {
			t.Skipf("diablo could not get more than 1100 descriptors: %q", m)
		}
		if err := os.WriteFile(flag, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		d := net.Dialer{LocalAddr: &net.TCPAddr{IP: net.ParseIP(PeerIP)}, Timeout: 5 * time.Second}
		if nc, err := d.Dial("tcp", s.Addr); err == nil {
			_ = nc.SetReadDeadline(time.Now().Add(5 * time.Second))
			buf := make([]byte, 512)
			_, _ = nc.Read(buf) // the greeting, when the child gets to send it
			nc.Close()
		}
		if len(shimMarked(mark, "filled to")) == 0 {
			t.Fatalf("the descriptors were not filled")
		}
		select {
		case <-s.Proc.Done():
			t.Fatalf("the diablo master exited after a connection on a descriptor above 1024 (fortified: %v)\n%s",
				fortified, s.Proc.Stderr())
		case <-time.After(3 * time.Second):
		}
		p := s.Peer(t)
		p.ExpectCmd(205, "QUIT")
	})
}
