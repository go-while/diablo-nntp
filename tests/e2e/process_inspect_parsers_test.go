package e2e

import (
	"errors"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// Native command formats are exercised on every OS, including Linux CI. These
// fixtures complement, but do not replace, running the commands on the BSD VMs.
func TestProcessInspectionParsers(t *testing.T) {
	t.Run("OS aliases", func(t *testing.T) {
		for _, goos := range []string{"linux", "freebsd", "netbsd", "illumos", "solaris"} {
			want := goos
			if goos == "illumos" {
				want = "solaris"
			}
			if got := inspectionOS(goos); got != want {
				t.Errorf("inspectionOS(%q)=%q, want %q", goos, got, want)
			}
		}
		want := []string{"-e", "-o", "pid=", "-o", "ppid=", "-o", "comm="}
		for _, goos := range []string{"illumos", "solaris"} {
			if got := processTableArgs(goos); !slices.Equal(got, want) {
				t.Errorf("%s process table command: %v, want %v", goos, got, want)
			}
		}
		for _, goos := range []string{"freebsd", "netbsd"} {
			if got := processTableArgs(goos); got[0] != "-ax" || got[len(got)-1] != "ucomm=" {
				t.Errorf("%s process table command: %v", goos, got)
			}
		}
	})
	t.Run("process tree", func(t *testing.T) {
		ps := parseProcesses("21 4 dreaderd\n4 94 dreaderd\n94 1 /dbin/dreaderd\n7 1 dnewslink\nPID PPID COMM\n")
		if len(ps) != 4 || ps[2].name != "dreaderd" {
			t.Fatalf("ps records: %+v", ps)
		}
		owned := processDescendants(ps, 94)
		if len(owned) != 3 || !owned[4] || !owned[21] || owned[7] {
			t.Errorf("descendants: %v (must not depend on PID order)", owned)
		}
	})
	t.Run("illumos full command name", func(t *testing.T) {
		ps := parseProcesses("42 1 /dbin/dnewslink\n43 1 /dbin/dspoolout\n")
		if len(ps) != 2 || ps[0].name != "dnewslink" || ps[1].name != "dspoolout" {
			t.Errorf("full comm parsing: %+v", ps)
		}
	})
	t.Run("exact site environment", func(t *testing.T) {
		const want = "DIABLO_CONFIG_PATH=/tmp/site/diablo.config"
		for _, out := range []string{
			"dnewslink -r " + want + " PATH=/bin",
			"123: dnewslink\nenvp[0]: " + want + "\nenvp[1]: PATH=/bin\n",
			" PID COMM ENVIRONMENT\n123 dnewslink PATH=/bin " + want + "\n",
		} {
			if !hasSiteEnvironment(out, want) {
				t.Errorf("site not found in %q", out)
			}
		}
		for _, out := range []string{want + ".other", "OTHER_" + want, "DIABLO_CONFIG_PATH=/tmp/other/diablo.config", ""} {
			if hasSiteEnvironment(out, want) {
				t.Errorf("matched another site in %q", out)
			}
		}
	})
	t.Run("fstat identity with absent procstat path", func(t *testing.T) {
		fds := parseDescriptors("freebsd", 42, "42 dnewslink 8 v r r------- 1 0 - -\n")
		if len(fds) != 1 || descriptorNamesFile(fds[0], "/tmp/site/dqueue/fake") {
			t.Fatalf("unnamed procstat descriptor: %+v", fds)
		}
		// fstat -p 42 /tmp/site/dqueue/fake has already selected the
		// filesystem/inode. The NAME text is irrelevant to FD selection.
		matches := parseFstatDescriptors(42, "USER CMD PID FD MOUNT INUM MODE SZ|DV R/W NAME\nnews dnewslink 42 8 / 123 -rw-r--r-- 0 r /tmp/site/dqueue/fake\nnews dnewslink 43 9 / 123 -rw-r--r-- 0 r /tmp/site/dqueue/fake\n")
		if len(matches) != 1 || matches[0].number != fds[0].number {
			t.Errorf("fstat file-identity descriptors: %+v", matches)
		}
		if _, err := fstatFileDescriptors(42); err == nil {
			t.Error("missing file filter would inspect all FDs, not just the active queue")
		}
	})
	t.Run("active queue not batch queue", func(t *testing.T) {
		const active = "/tmp/site/dqueue/fake"
		if !descriptorNamesFile(inspectedFD{target: "- " + active}, active) {
			t.Error("active queue descriptor not recognized")
		}
		for _, name := range []string{active + ".S00000", active + "-other", "/tmp/other/dqueue/fake"} {
			if descriptorNamesFile(inspectedFD{target: "- " + name}, active) {
				t.Errorf("batch/foreign queue %q recognized as active", name)
			}
		}
	})
	fixtures := map[string]string{
		"freebsd": `  PID COMM FD T V FLAGS REF OFFSET PRO NAME
42 dnewslink cwd v d r------- - - - /tmp/site
42 dnewslink text v r r------- - - - /dbin/dnewslink
42 dnewslink 0 v c rw------ 1 0 - /dev/null
42 dnewslink 3 v r r------- 1 0 - /tmp/site/spool/news/B.0000
42 dnewslink 7 s - rw------ 2 0 TCP 127.0.0.1:1234 127.0.0.1:119
43 dnewslink 8 s - rw------ 2 0 TCP 127.0.0.1:5678 127.0.0.1:119
`,
		"netbsd": `USER CMD PID FD MOUNT INUM MODE SZ|DV R/W
news dnewslink 42 wd / 100 drwxr-xr-x 512 r
news dnewslink 42 text / 101 -r-xr-xr-x 1000 r
news dnewslink 42 0 /dev 1 crw-rw-rw- null rw
news dnewslink 42 3 / 200 -rw-r--r-- 1000 r
news dnewslink 42 7* internet stream tcp ffff800000012340 127.0.0.1:1234 <-> 127.0.0.1:119
news dnewslink 43 8* internet stream tcp ffff800000056780 127.0.0.1:5678 <-> 127.0.0.1:119
`,
		"solaris": `42: /dbin/dnewslink -r
 Current rlimit: 256 file descriptors
 0: S_IFCHR mode:0666 dev:1,2 ino:1 uid:8 gid:8 rdev:13,2
    O_RDWR
    /dev/null
 3: S_IFREG mode:0644 dev:0,1 ino:200 uid:8 gid:8 size:1000
    O_RDONLY
    /tmp/site/spool/news/B.0000
 7: S_IFSOCK mode:0666 dev:0,0 ino:300 uid:8 gid:8 size:0
    O_RDWR|O_NONBLOCK
    SOCK_STREAM
    SO_SNDBUF(65536),SO_RCVBUF(65536)
    sockname: AF_INET 127.0.0.1 port: 1234
    peername: AF_INET 127.0.0.1 port: 119
`,
	}
	fixtures["illumos"] = fixtures["solaris"]
	for goos, out := range fixtures {
		t.Run(goos+" descriptors", func(t *testing.T) {
			fds := parseDescriptors(goos, 42, out)
			if len(fds) != 3 || fds[0].number != 0 || fds[1].number != 3 || fds[2].number != 7 || !fds[2].socket || fds[0].socket || fds[1].socket {
				t.Fatalf("parsed descriptors: %+v", fds)
			}
			if got := parseDescriptors(goos, 42, "permission denied\nPID COMM FD\n"); len(got) != 0 {
				t.Errorf("diagnostics counted as descriptors: %+v", got)
			}
			changed := strings.ReplaceAll(out, "rw------ 2 0 TCP", "rw--n--- 4 123 TCP")
			changed = strings.ReplaceAll(changed, "SO_SNDBUF(65536),SO_RCVBUF(65536)", "SO_SNDBUF(32768),SO_RCVBUF(131072)")
			if !slices.Equal(fds, parseDescriptors(goos, 42, changed)) {
				t.Error("socket flags/buffer sizes changed descriptor identities")
			}
		})
	}
}

func TestInspectionOutputFailure(t *testing.T) {
	_, err := inspectionOutput("sh", "-c", "echo inspection-denied >&2; exit 7")
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 7 || !strings.Contains(err.Error(), "inspection-denied") {
		t.Errorf("command failure lost status/stderr: %v", err)
	}
}
