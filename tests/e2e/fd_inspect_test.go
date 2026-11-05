package e2e

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
)

type inspectedFD struct {
	number int
	target string
	socket bool
}

func inspectDescriptors(pid int) ([]inspectedFD, error) {
	var b []byte
	var err error
	switch inspectionOS(runtime.GOOS) {
	case "freebsd":
		b, err = inspectionOutput("procstat", "-f", strconv.Itoa(pid))
	case "netbsd":
		b, err = inspectionOutput("fstat", "-p", strconv.Itoa(pid))
	case "solaris":
		// No -F: never steal control from another observer. pfiles briefly
		// stops this test's process and reports socket addresses and paths.
		b, err = inspectionOutput("pfiles", strconv.Itoa(pid))
	default:
		return nil, fmt.Errorf("no descriptor inspector for %s", runtime.GOOS)
	}
	if err != nil {
		return nil, err
	}
	fds := parseDescriptors(runtime.GOOS, pid, string(b))
	if len(fds) == 0 {
		return nil, fmt.Errorf("no numeric descriptors for pid %d in %q", pid, b)
	}
	return fds, nil
}

func parseDescriptors(goos string, pid int, out string) []inspectedFD {
	goos = inspectionOS(goos)
	if goos == "netbsd" {
		return parseFstatDescriptors(pid, out)
	}
	var fds []inspectedFD
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if goos == "solaris" {
			// Each block begins "N: S_IF...". The process heading is
			// also "PID:", but does not have a file type as its next word.
			if len(f) >= 2 && strings.HasSuffix(f[0], ":") && strings.HasPrefix(f[1], "S_IF") {
				n, err := strconv.Atoi(strings.TrimSuffix(f[0], ":"))
				if err == nil && n >= 0 {
					fds = append(fds, inspectedFD{n, strings.Join(f[1:], " "), f[1] == "S_IFSOCK"})
				}
			} else if len(fds) > 0 {
				// Do not use queue sizes, options or flags as socket
				// identity: those may change while the same socket lives.
				last := &fds[len(fds)-1]
				if strings.HasPrefix(strings.TrimSpace(line), "/") || (last.socket && (strings.HasPrefix(strings.TrimSpace(line), "sockname:") || strings.HasPrefix(strings.TrimSpace(line), "peername:"))) {
					last.target += " " + strings.Join(f, " ")
				}
			}
			continue
		}
		if goos == "freebsd" {
			// PID COMM FD T V FLAGS REF OFFSET PRO NAME; ignore cwd,
			// root, text and ctty, which are not numbered descriptors.
			if len(f) < 9 || f[0] != strconv.Itoa(pid) {
				continue
			}
			n, err := strconv.Atoi(f[2])
			if err == nil && n >= 0 {
				fds = append(fds, inspectedFD{n, strings.Join(f[8:], " "), f[3] == "s"})
			}
			continue
		}
	}
	slices.SortFunc(fds, func(a, b inspectedFD) int { return a.number - b.number })
	return fds
}

func parseFstatDescriptors(pid int, out string) []inspectedFD {
	var fds []inspectedFD
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		// BSD fstat: USER CMD PID FD MOUNT INUM ...; a socket has a
		// starred FD and domain/type/PCB (and endpoints) in the tail.
		if len(f) < 5 || f[2] != strconv.Itoa(pid) {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSuffix(f[3], "*"))
		if err == nil && n >= 0 {
			domain := f[4]
			sock := domain == "internet" || domain == "internet6" || domain == "inet" || domain == "inet6" || domain == "unix" || domain == "local"
			fds = append(fds, inspectedFD{n, strings.Join(f[4:], " "), sock})
		}
	}
	slices.SortFunc(fds, func(a, b inspectedFD) int { return a.number - b.number })
	return fds
}

func fstatFileDescriptors(pid int, files ...string) ([]inspectedFD, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("fstat file matching requires at least one file")
	}
	args := append([]string{"-p", strconv.Itoa(pid)}, files...)
	b, err := inspectionOutput("fstat", args...)
	if err != nil {
		return nil, err
	}
	return parseFstatDescriptors(pid, string(b)), nil
}

func portableFDTargets(s *Site, pid int) ([]string, error) {
	fds, err := inspectDescriptors(pid)
	if err != nil {
		return nil, err
	}
	spoolFDs := map[int]bool{}
	if runtime.GOOS == "freebsd" || runtime.GOOS == "netbsd" {
		// fstat normally reports mount/inode, not path. Supplying the
		// site's actual spool files identifies just their FD numbers,
		// preserving the Linux exclusion of variable article caches.
		var files []string
		err := filepath.WalkDir(filepath.Join(s.Dir, "spool", "news"), func(path string, e fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if e.Type().IsRegular() {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		if len(files) > 0 {
			matches, err := fstatFileDescriptors(pid, files...)
			if err != nil {
				return nil, err
			}
			for _, fd := range matches {
				spoolFDs[fd.number] = true
			}
		}
	}
	var targets []string
	for _, fd := range fds {
		if !spoolFDs[fd.number] && !strings.Contains(fd.target, "/spool/news/") {
			targets = append(targets, fmt.Sprintf("%d -> %s", fd.number, fd.target))
		}
	}
	return targets, nil
}
