package e2e

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestDeepRevDexpireoverRewriteFail (L7-08, L5-01): "dexpireover -R" copies the overview
// records of a group into new data files and writes a new index (over file) with the new
// offsets. It removed the old data files first and wrote the new index after that; when the
// index could not be written (full disk, quota) it only removed the temporary index and
// still moved the new data files in. The group was left with the old index, whose offsets
// point into data files that have been packed: rows missing or cut.
//
// The index is now written before anything is removed, and when that fails the group is left
// as it was. The test makes the write fail with a directory at the temporary name, on a
// data file that has 10 bytes of junk before its only record (so packing moves the record).
// The offset in the index must still find the record. Without the directory the same run
// packs the file and the new index finds the record at its new place.
func TestDeepRevDexpireoverRewriteFail(t *testing.T) {
	s, home, rec, found := rewriteFixture(t)
	data := filepath.Join(home, "e2e", "d.0.e2e.test")
	tmpOver := filepath.Join(home, "e2e", ".o.0.e2e.test")

	// A directory (not empty, so remove() leaves it) where the new index is written.
	if err := os.MkdirAll(filepath.Join(tmpOver, "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := s.TryRun("dexpireover", "-R", "-v")
	t.Logf("dexpireover -R -v with the new index not writable: err=%v\n%s", err, out)
	if ok, why := found(); !ok {
		t.Errorf("after a failed rewrite of the index: %s", why)
	}
	if !strings.Contains(out, "over. file failed") {
		t.Errorf("dexpireover did not report the failed rewrite of the index")
	}
	if !rewriteExit1(err) {
		t.Errorf("dexpireover -R after a failed rewrite: exit status %v, want 1", err)
	}
	if l, _ := filepath.Glob(filepath.Join(home, "e2e", ".d.*")); len(l) != 0 {
		t.Errorf("temporary data files left behind: %q", l)
	}

	// The same run with nothing in the way packs the data file.
	if err := os.RemoveAll(tmpOver); err != nil {
		t.Fatal(err)
	}
	out = s.Run(t, "dexpireover", "-R", "-v")
	ok, why := found()
	if !ok {
		t.Errorf("after a good rewrite: %s\n%s", why, out)
	}
	if d, err := os.ReadFile(data); err != nil || !bytes.Equal(d, append(append([]byte{}, rec...), 0)) {
		t.Errorf("after a good rewrite the data file is not the packed record (%s): %v, %d bytes\n%s", why, err, len(d), out)
	}
}

// rewriteFixture is a group e2e.test (hierarchy layout) whose only article, number 1, has its
// record in d.0.e2e.test behind 10 bytes of junk, so that dexpireover -R moves it to offset 0.
// found reports whether the index entry of article 1 points at the record in the data file.
func rewriteFixture(t *testing.T) (s *Site, home string, rec []byte, found func() (bool, string)) {
	return rewriteFixtureGz(t, false)
}

// rewriteFixtureGz is rewriteFixture, with the data file gzipped (d.0.e2e.test.gz, as
// dexpireover -z leaves it) when gz; found then reads the .gz while there is no plain file.
func rewriteFixtureGz(t *testing.T, gz bool) (s *Site, home string, rec []byte, found func() (bool, string)) {
	t.Helper()
	s, home = overSite(t, map[string]string{"readergrouphash": "hierarchy"})
	s.WriteFile(t, "dexpire.ctl", "e2e.*:x30\n")
	over := filepath.Join(home, "e2e", "o.0.e2e.test")
	data := filepath.Join(home, "e2e", "d.0.e2e.test")

	// more than 256 bytes: a smaller .gz is refused as damaged ("Bad zlen")
	rec = []byte("Subject: deeprev rewrite\r\nMessage-ID: <deeprev-rewrite@e2e.invalid>\r\nX-Pad: " +
		strings.Repeat("padding ", 40) + "\r\n")
	junk := []byte("junkjunk.\x00")
	o := binary.NativeEndian
	head := overHead("e2e.test")
	index := append(head, make([]byte, 1000*32)...)
	slot := index[len(head)+32:] // article 1
	o.PutUint32(slot[0:], 1)
	o.PutUint32(slot[4:], uint32(len(junk)))
	o.PutUint32(slot[8:], uint32(len(rec)))
	o.PutUint32(slot[12:], 0x952a1fae)
	o.PutUint32(slot[16:], 0x54dcbdde)
	o.PutUint32(slot[20:], 1000)
	o.PutUint32(slot[24:], uint32(time.Now().Unix()))
	writeOver(t, over, index)
	raw := append(append(append([]byte{}, junk...), rec...), 0)
	if gz {
		var b bytes.Buffer
		w := gzip.NewWriter(&b)
		w.Write(raw)
		w.Close()
		writeOver(t, data+".gz", b.Bytes())
	} else {
		writeOver(t, data, raw)
	}

	found = func() (bool, string) {
		ix, err := os.ReadFile(over)
		if err != nil {
			return false, err.Error()
		}
		d, err := os.ReadFile(data)
		if errors.Is(err, os.ErrNotExist) {
			var z []byte
			if z, err = os.ReadFile(data + ".gz"); err == nil {
				var r *gzip.Reader
				if r, err = gzip.NewReader(bytes.NewReader(z)); err == nil {
					d, err = io.ReadAll(r)
				}
			}
		}
		if err != nil {
			return false, err.Error()
		}
		if len(ix) < 16 {
			return false, "over file too short"
		}
		headSize, maxArts := int(o.Uint32(ix[8:])), int(o.Uint32(ix[12:]))
		if maxArts <= 0 || len(ix) < headSize+maxArts*32 {
			return false, "over file header does not fit its size"
		}
		sl := ix[headSize+(1%maxArts)*32:]
		artNo, pos, n := int32(o.Uint32(sl[0:])), int(int32(o.Uint32(sl[4:]))), int(int32(o.Uint32(sl[8:])))
		if artNo != 1 {
			return false, "no index entry for article 1"
		}
		if pos < 0 || n < 0 || pos+n > len(d) || !bytes.Equal(d[pos:pos+n], rec) {
			return false, "index entry of article 1 (offset " + strconv.Itoa(pos) + ", " + strconv.Itoa(n) +
				" bytes) does not hold its record in the data file of " + strconv.Itoa(len(d)) + " bytes"
		}
		return true, "record at offset " + strconv.Itoa(pos)
	}
	if ok, why := found(); !ok {
		t.Fatalf("test setup: %s", why)
	}
	return s, home, rec, found
}

// rewriteExit1 reports whether err is a normal exit with status 1 (not 0, and not a crash).
func rewriteExit1(err error) bool {
	var ee *exec.ExitError
	return errors.As(err, &ee) && ee.ExitCode() == 1
}

// rewriteFaultSource makes write() to a file whose path holds REWRITEFAULT_WRITE fail with
// ENOSPC, mmap() of such a file (REWRITEFAULT_MMAP) fail with ENOMEM, and rename() of a path
// that holds REWRITEFAULT_RENAME fail with EIO; with REWRITEFAULT_VANISH the file is removed
// and rename() fails with ENOENT. Each hit appends the path to the file REWRITEFAULT_MARK.
const rewriteFaultSource = `#define _GNU_SOURCE
#define __EXTENSIONS__
#include <dlfcn.h>
#include <errno.h>
#include <fcntl.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/mman.h>
#include <unistd.h>
/*FDPATH*/
static ssize_t (*realWrite)(int, const void *, size_t);

static void
mark(const char *path)
{
    const char *m = getenv("REWRITEFAULT_MARK");
    int fd;

    if (m != NULL && (fd = open(m, O_WRONLY|O_CREAT|O_APPEND, 0644)) >= 0) {
	(void)realWrite(fd, path, strlen(path));
	(void)realWrite(fd, "\n", 1);
	close(fd);
    }
}

ssize_t
write(int fd, const void *buf, size_t n)
{
    const char *only = getenv("REWRITEFAULT_WRITE");
    char path[4096];

    if (realWrite == NULL)
	realWrite = (ssize_t (*)(int, const void *, size_t))dlsym(RTLD_NEXT, "write");
    if (only != NULL) {
	fdpath(fd, path, sizeof(path));
	if (path[0] != 0 && strstr(path, only) != NULL) {
	    mark(path);
	    errno = ENOSPC;
	    return -1;
	}
    }
    return realWrite(fd, buf, n);
}

int
rename(const char *from, const char *to)
{
    static int (*real)(const char *, const char *);
    const char *only = getenv("REWRITEFAULT_RENAME");
    const char *gone = getenv("REWRITEFAULT_VANISH");

    if (realWrite == NULL)
	realWrite = (ssize_t (*)(int, const void *, size_t))dlsym(RTLD_NEXT, "write");
    if (real == NULL)
	real = (int (*)(const char *, const char *))dlsym(RTLD_NEXT, "rename");
    if (only != NULL && strstr(from, only) != NULL) {
	mark(from);
	errno = EIO;
	return -1;
    }
    if (gone != NULL && strstr(from, gone) != NULL) {
	mark(from);
	(void)unlink(from);
	errno = ENOENT;
	return -1;
    }
    return real(from, to);
}

static int
mapfail(int fd)
{
    const char *only = getenv("REWRITEFAULT_MMAP");
    char path[4096];

    if (realWrite == NULL)
	realWrite = (ssize_t (*)(int, const void *, size_t))dlsym(RTLD_NEXT, "write");
    if (only == NULL || fd < 0)
	return 0;
    fdpath(fd, path, sizeof(path));
    if (path[0] == 0 || strstr(path, only) == NULL)
	return 0;
    mark(path);
    errno = ENOMEM;
    return 1;
}

void *
mmap(void *addr, size_t len, int prot, int flags, int fd, off_t off)
{
    static void *(*real)(void *, size_t, int, int, int, off_t);

    if (real == NULL)
	real = (void *(*)(void *, size_t, int, int, int, off_t))dlsym(RTLD_NEXT, "mmap");
    if (mapfail(fd))
	return MAP_FAILED;
    return real(addr, len, prot, flags, fd, off);
}

#ifdef __GLIBC__	/* the name mmap() has with _FILE_OFFSET_BITS=64 */
void *
mmap64(void *addr, size_t len, int prot, int flags, int fd, off64_t off)
{
    static void *(*real)(void *, size_t, int, int, int, off64_t);

    if (real == NULL)
	real = (void *(*)(void *, size_t, int, int, int, off64_t))dlsym(RTLD_NEXT, "mmap64");
    if (mapfail(fd))
	return MAP_FAILED;
    return real(addr, len, prot, flags, fd, off);
}
#endif
`

// TestDeepRevDexpireoverRewriteFault (second review of L7-08: Sol F2, Terra T1, and their
// re-review): the first fix stopped -R before anything was removed only when the new index
// could not be written. A new data file that could not be written (full disk) only lost its
// records: the original data files were still removed and the short new ones moved in. So did
// a source data file that is there but could not be mapped (or unpacked): it was taken as
// missing. When the final rename of the new index failed, the temporary index was removed and
// the old one stayed, with offsets that do not fit the packed data files that had replaced the
// originals. And a new data file gone at its rename was taken as a range without records. All
// exited 0.
//
// A preloaded library makes each fail:
//   - data_write: the write of the new data file; the group is left as it was;
//   - source_mmap, source_mmap_gz: mmap() of the source data file (plain, or .gz with zlib);
//     the group is left as it was;
//   - index_rename: the rename of the new index; dexpireover writes the new index into the
//     old file in place, so it fits the packed data;
//   - temp_gone: the new data file is removed at its rename; its records are lost, which
//     must be said.
//
// All must exit 1 and leave no temporary files.
func TestDeepRevDexpireoverRewriteFault(t *testing.T) {
	so := preloadShim(t, "rewritefault", rewriteFaultSource)
	zlib := builtWithZlib(t)
	for _, c := range []struct {
		name, env, msg string
		gz             bool
		want           string // kept, packed or lost
	}{
		{"data_write", "REWRITEFAULT_WRITE=/.d.0.e2e.test", "data files failed, group left as it was", false, "kept"},
		{"source_mmap", "REWRITEFAULT_MMAP=/d.0.e2e.test", "Unable to xmap", false, "kept"},
		{"source_mmap_gz", "REWRITEFAULT_MMAP=/d.0.e2e.test", "Unable to xmap for gzcat", true, "kept"},
		{"index_rename", "REWRITEFAULT_RENAME=/.o.0.e2e.test", "new over. file written in place", false, "packed"},
		{"temp_gone", "REWRITEFAULT_VANISH=/.d.0.e2e.test", "is gone, its records are lost", false, "lost"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if c.gz && !zlib {
				t.Skip("built without zlib")
			}
			s, home, rec, found := rewriteFixtureGz(t, c.gz)
			data := filepath.Join(home, "e2e", "d.0.e2e.test")
			if c.gz {
				data += ".gz"
			}
			oldData, _ := os.ReadFile(data)
			mark := filepath.Join(t.TempDir(), "hit")
			base := s.Env
			s.Env = append(slices.Clone(base), lockFaultEnv(so), c.env, "REWRITEFAULT_MARK="+mark)
			out, err := s.TryRun("dexpireover", "-R", "-v")
			s.Env = base
			if b, err := os.ReadFile(mark); err != nil || len(b) == 0 {
				t.Fatalf("the fault was never hit (%v):\n%s", err, out)
			}
			if !rewriteExit1(err) {
				t.Errorf("dexpireover -R: exit status %v, want 1", err)
			}
			if !strings.Contains(out, c.msg) {
				t.Errorf("no line %q", c.msg)
			}
			if c.want != "lost" {
				if ok, why := found(); !ok {
					t.Errorf("after the fault: %s", why)
				}
			}
			d, _ := os.ReadFile(data)
			switch c.want {
			case "packed":
				if !bytes.Equal(d, append(append([]byte{}, rec...), 0)) {
					t.Errorf("the data file is not the packed record: %d bytes", len(d))
				}
			case "kept":
				// (-R resizes the over file in place first; found() checks that it still
				// points at the record in the old data file)
				if !bytes.Equal(d, oldData) {
					t.Errorf("the data file was changed: %d bytes, was %d", len(d), len(oldData))
				}
			}
			if l, _ := filepath.Glob(filepath.Join(home, "e2e", ".[do].*")); len(l) != 0 {
				t.Errorf("temporary files left behind: %q", l)
			}
			if t.Failed() {
				t.Logf("dexpireover -R -v (%s): err=%v\n%s", c.env, err, out)
			}
		})
	}
}
