package e2e

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestDeepRevDexpireoverNewGroup (L7-04): dexpireover reads the list of groups from
// dactive.kp once, at the start, and removes every overview file whose group is not in that
// list as stale. A run takes hours on a large reader; a group created meanwhile was not in
// the list, so the overview dreaderd had just written for it was removed.
//
// dexpireover now looks for groups added to dactive.kp since its scan before it takes a
// file as stale. The test stops dexpireover in the first pass over the directory (it holds
// the lock on the over file of e2e.test that the resize waits for), adds a group to
// dactive.kp, and lets it go on: the data file of that group must stay, the one of a group
// that is in no active file must go.
func TestDeepRevDexpireoverNewGroup(t *testing.T) {
	s, home := overSite(t, nil)
	s.Run(t, "dgrpctl", "adjust", "e2e.test", "NE", "=1000")
	s.WriteFile(t, "dexpire.ctl", "e2e.*:x30\n")
	// overFile returns the directory and the name of the over file of a group.
	overFile := func(group string) (dir, name string) {
		f := strings.Fields(s.Run(t, "dgrpctl", "listgrouphashfile", group))
		if len(f) != 2 || !strings.HasPrefix(filepath.Base(f[0]), "over.0.") {
			t.Fatalf("dgrpctl listgrouphashfile %s: %q", group, f)
		}
		return filepath.Dir(f[0]), filepath.Base(f[0])
	}
	// dexpireover works through one directory after the other, in each all over files
	// first and then the data files. To have it wait in the right place, the two other
	// groups must have their files in the directory of e2e.test.
	dir, overName := overFile("e2e.test")
	inDir := func(prefix string) (group, data string) {
		// The directory is the low byte of the first CRC word, which the last eight
		// characters of a name reach only by their position: count in front of them.
		for i := 0; i < 5000; i++ {
			g := prefix + strconv.Itoa(i) + ".newsgroup"
			if d, n := overFile(g); d == dir {
				return g, filepath.Join(home, dir, "data.256."+strings.TrimPrefix(n, "over.0."))
			}
		}
		t.Fatalf("no group name %sN.newsgroup with its overview in directory %s", prefix, dir)
		return "", ""
	}
	lateGroup, late := inDir("e2e.late")
	_, gone := inDir("e2e.gone")
	if err := os.MkdirAll(filepath.Join(home, dir), 0o755); err != nil {
		t.Fatal(err)
	}
	over := filepath.Join(home, dir, overName)
	// 500 slots for NB..NE 1..1000: -s resizes, and waits for the lock at offset 4
	writeOver(t, over, append(overHeadMax("e2e.test", 500), make([]byte, 500*32)...))
	writeOver(t, late, []byte("overview data of a new group\x00"))
	writeOver(t, gone, []byte("overview data of a removed group\x00"))

	f, err := os.OpenFile(over, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	lk := syscall.Flock_t{Type: syscall.F_WRLCK, Whence: 0, Start: 4, Len: 4}
	if err := syscall.FcntlFlock(f.Fd(), syscall.F_SETLK, &lk); err != nil {
		t.Fatalf("lock %s: %v", over, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	// -l60: the resize gives up on the lock after 1 s by default
	cmd := exec.CommandContext(ctx, filepath.Join(s.Bin, "dexpireover"), "-a", "-s", "-y", "-v", "-l60")
	cmd.Dir = s.Dir
	cmd.Env = append(os.Environ(), s.Env...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	if err := waitLockWaiter(cmd.Process.Pid, done); err != nil {
		t.Fatalf("dexpireover did not wait for the lock: %v\n%s", err, out.String())
	}

	// The group table has been read; the data files are looked at after all over files.
	db := filepath.Join(s.Dir, "dactive.kp")
	dkp := filepath.Join(s.Bin, "dkp")
	for _, a := range [][]string{
		{"-w", db, lateGroup, "NB=0000000001"},
		{"-w", db, lateGroup, "NE=0000000300"},
		{"-w", db, lateGroup, "S=y"},
	} {
		c := exec.Command(dkp, a...)
		c.Dir = s.Dir
		c.Env = append(os.Environ(), s.Env...)
		if o, err := c.CombinedOutput(); err != nil {
			t.Fatalf("dkp %s: %v\n%s", strings.Join(a, " "), err, o)
		}
	}
	if live, _, _ := kpRecords(t, db, lateGroup); len(live) != 1 {
		t.Fatalf("%s is not in dactive.kp: %q", lateGroup, live)
	}

	lk.Type = syscall.F_UNLCK
	if err := syscall.FcntlFlock(f.Fd(), syscall.F_SETLK, &lk); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("dexpireover: %v\n%s", err, out.String())
	}

	if _, err := os.Stat(late); err != nil {
		t.Errorf("dexpireover removed the data file of a group created during its run: %v", err)
	}
	if _, err := os.Stat(gone); err == nil {
		t.Errorf("dexpireover kept the data file of a group that is in no active file")
	}
	if t.Failed() {
		t.Logf("dexpireover -a -s -y -v -l60:\n%s", out.String())
	}
}

// TestDeepRevDexpireoverRecreatedGroup (Terra T2, second review of L7-04): the files of a group
// carry its iteration (ITER) in their names, and a group removed and created again gets a new
// one. The look for groups added during the run skipped a name it knew already, so for a group
// recreated during the run the table kept the old iteration, and the files of the new one were
// removed as stale.
//
// As in TestDeepRevDexpireoverNewGroup dexpireover waits for a lock in its first pass; meanwhile
// the test removes group e2e.reN.newsgroup (ITER 0) and creates it again with ITER 1. Its data
// file of iteration 1 must stay.
func TestDeepRevDexpireoverRecreatedGroup(t *testing.T) {
	s, home := overSite(t, nil)
	s.Run(t, "dgrpctl", "adjust", "e2e.test", "NE", "=1000")
	s.WriteFile(t, "dexpire.ctl", "e2e.*:x30\n")
	db := filepath.Join(s.Dir, "dactive.kp")
	overFile := func(group string) (dir, name string) {
		f := strings.Fields(s.Run(t, "dgrpctl", "listgrouphashfile", group))
		if len(f) != 2 || !strings.HasPrefix(filepath.Base(f[0]), "over.0.") {
			t.Fatalf("dgrpctl listgrouphashfile %s: %q", group, f)
		}
		return filepath.Dir(f[0]), filepath.Base(f[0])
	}
	dir, overName := overFile("e2e.test")
	var group, data string
	for i := 0; i < 5000 && group == ""; i++ {
		g := "e2e.re" + strconv.Itoa(i) + ".newsgroup"
		if d, n := overFile(g); d == dir {
			group = g
			// the name of a file of iteration 1 ends in ".1"
			data = filepath.Join(home, dir, "data.256."+strings.TrimPrefix(n, "over.0.")+".1")
		}
	}
	if group == "" {
		t.Fatalf("no group name e2e.reN.newsgroup with its overview in directory %s", dir)
	}
	dkp := func(a ...string) {
		t.Helper()
		c := exec.Command(filepath.Join(s.Bin, "dkp"), a...)
		c.Dir = s.Dir
		c.Env = append(os.Environ(), s.Env...)
		if o, err := c.CombinedOutput(); err != nil {
			t.Fatalf("dkp %s: %v\n%s", strings.Join(a, " "), err, o)
		}
	}
	create := func(iter string) {
		for _, f := range []string{"NB=0000000001", "NE=0000000300", "S=y", "ITER=" + iter} {
			dkp("-w", db, group, f)
		}
	}
	create("000000")
	if err := os.MkdirAll(filepath.Join(home, dir), 0o755); err != nil {
		t.Fatal(err)
	}
	over := filepath.Join(home, dir, overName)
	// 500 slots for NB..NE 1..1000: -s resizes, and waits for the lock at offset 4
	writeOver(t, over, append(overHeadMax("e2e.test", 500), make([]byte, 500*32)...))
	writeOver(t, data, []byte("overview data of the recreated group\x00"))

	f, err := os.OpenFile(over, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	lk := syscall.Flock_t{Type: syscall.F_WRLCK, Whence: 0, Start: 4, Len: 4}
	if err := syscall.FcntlFlock(f.Fd(), syscall.F_SETLK, &lk); err != nil {
		t.Fatalf("lock %s: %v", over, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(s.Bin, "dexpireover"), "-a", "-s", "-y", "-v", "-l60")
	cmd.Dir = s.Dir
	cmd.Env = append(os.Environ(), s.Env...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	if err := waitLockWaiter(cmd.Process.Pid, done); err != nil {
		t.Fatalf("dexpireover did not wait for the lock: %v\n%s", err, out.String())
	}

	dkp("-x", db, group)
	create("000001")
	if live, _, _ := kpRecords(t, db, group); len(live) != 1 || !strings.Contains(live[0], "ITER=000001") {
		t.Fatalf("%s was not created again with ITER 1: %q", group, live)
	}

	lk.Type = syscall.F_UNLCK
	if err := syscall.FcntlFlock(f.Fd(), syscall.F_SETLK, &lk); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("dexpireover: %v\n%s", err, out.String())
	}
	if _, err := os.Stat(data); err != nil {
		t.Errorf("dexpireover removed the data file of a group created again during its run: %v\n%s", err, out.String())
	}
}
