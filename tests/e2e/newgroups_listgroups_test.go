package e2e

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Different readerdefs share a reader process, but keep their own listgroups
// when NEWGROUPS builds and reuses the group's creation-time cache.
func TestNewgroupsCacheListgroups(t *testing.T) {
	const access = `vserverdef DEFAULT
    hostname ` + PathHost + `
end
groupdef left
    group e2e.left
end
groupdef right
    group e2e.right
end
readerdef left
    read yes
    listgroups left
end
readerdef right
    read yes
    listgroups right
end
readerdef unrestricted
    read yes
end
access 127.0.0.1 left
access 127.0.0.2 right
access 127.0.0.3 unrestricted
`
	readers := []struct {
		ip   string
		want []string
	}{
		{"127.0.0.1", []string{"e2e.left"}},
		{"127.0.0.2", []string{"e2e.right"}},
		{"127.0.0.3", []string{"e2e.left", "e2e.right"}},
	}
	for _, first := range []int{0, 1, 2} {
		t.Run(readers[first].ip, func(t *testing.T) {
			_, spool := spoolStore(t)
			s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: access,
				Groups: []string{"e2e.left", "e2e.right"}})
			list := func(index int, command string) []string {
				t.Helper()
				c := s.dialFrom(t, readers[index].ip)
				defer c.Close()
				if command == "LIST ACTIVE" {
					c.ExpectCmd(215, "%s", command)
				} else {
					c.ExpectCmd(231, "%s", command)
				}
				var got []string
				for _, line := range c.ReadDotLines() {
					fields := strings.Fields(line)
					if len(fields) == 0 {
						t.Fatalf("%s returned an empty group line", command)
					}
					got = append(got, fields[0])
				}
				c.ExpectCmd(205, "QUIT")
				slices.Sort(got)
				return got
			}
			for i, reader := range readers {
				if got := list(i, "LIST ACTIVE"); !slices.Equal(got, reader.want) {
					t.Fatalf("LIST ACTIVE for %s: %q, want %q", reader.ip, got, reader.want)
				}
			}
			if got := list(first, "NEWGROUPS 20000101 000000 GMT"); !slices.Equal(got, readers[first].want) {
				t.Fatalf("initial NEWGROUPS: %q, want %q", got, readers[first].want)
			}
			for range 2 {
				for i, reader := range readers {
					if got := list(i, "NEWGROUPS 20000101 000000 GMT"); !slices.Equal(got, reader.want) {
						t.Errorf("NEWGROUPS omitted or added groups after another readerdef built the cache: reader %s got %q, want %q",
							reader.ip, got, reader.want)
					}
					if got := list(i, "NEWGROUPS 20371231 235959 GMT"); len(got) != 0 {
						t.Errorf("future NEWGROUPS for %s: %q, want an empty list", reader.ip, got)
					}
				}
			}
			s.Run(t, "dgrpctl", "newgroup", "e2e.added", "y")
			if got := list(first, "NEWGROUPS 20371231 235959 GMT"); len(got) != 0 {
				t.Fatalf("future NEWGROUPS while rebuilding: %q, want an empty list", got)
			}
			for i, reader := range readers {
				want := slices.Clone(reader.want)
				if i == 2 {
					want = append(want, "e2e.added")
					slices.Sort(want)
				}
				if got := list(i, "NEWGROUPS 20000101 000000 GMT"); !slices.Equal(got, want) {
					t.Errorf("NEWGROUPS after adding a group and building at a future cutoff: reader %s got %q, want %q",
						reader.ip, got, want)
				}
			}
		})
	}
}

// A rejected record can be busy without holding up a selective list reply.
func TestReaderListRejectedRecordLock(t *testing.T) {
	const access = `vserverdef DEFAULT
    hostname ` + PathHost + `
end
groupdef visible
    group e2e.left
    group e2e.zlast
end
readerdef restricted
    read yes
    listgroups visible
end
readerdef unrestricted
    read yes
end
access 127.0.0.1 restricted
access 127.0.0.3 unrestricted
`
	for _, cached := range []bool{false, true} {
		name := "without-cache"
		if cached {
			name = "cached"
		}
		t.Run(name, func(t *testing.T) {
			_, spool := spoolStore(t)
			s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: access,
				Groups: []string{"e2e.left", "e2e.right", "e2e.zlast"}})
			checkUnrestricted := func() {
				t.Helper()
				c := s.dialFrom(t, "127.0.0.3")
				defer c.Close()
				c.ExpectCmd(231, "NEWGROUPS 20000101 000000 GMT")
				var names []string
				for _, line := range c.ReadDotLines() {
					name, _, _ := strings.Cut(line, " ")
					names = append(names, name)
				}
				slices.Sort(names)
				if !slices.Equal(names, []string{"e2e.left", "e2e.right", "e2e.zlast"}) {
					t.Fatalf("unrestricted NEWGROUPS returned %q, want all three groups", names)
				}
				c.ExpectCmd(205, "QUIT")
			}
			if cached {
				checkUnrestricted()
			}
			db := filepath.Join(s.Dir, "dactive.kp")
			data, err := os.ReadFile(db)
			if err != nil {
				t.Fatal(err)
			}
			offset := int64(0)
			found := false
			for _, line := range bytes.SplitAfter(data, []byte{'\n'}) {
				if len(line) > 15 && line[0] == '+' &&
					bytes.HasPrefix(line[15:], []byte("e2e.right ")) {
					found = true
					break
				}
				offset += int64(len(line))
			}
			if !found {
				t.Fatal("active record for e2e.right not found")
			}
			release := holdHistLock(t, db, offset, 4)
			defer release()
			// e2e.right sorts between the two visible groups: a reply that
			// stops at the busy record, instead of skipping it, loses e2e.zlast.
			both := []string{"e2e.left", "e2e.zlast"}
			for _, tc := range []struct {
				ip, command string
				code        int
				want        []string
			}{
				{"127.0.0.3", "LIST ACTIVE e2e.left*", 215, []string{"e2e.left"}},
				{"127.0.0.1", "LIST ACTIVE", 215, both},
				{"127.0.0.1", "LIST NEWSGROUPS", 215, both},
				{"127.0.0.1", "LIST ACTIVE.TIMES", 215, both},
				{"127.0.0.1", "XGTITLE *", 282, both},
				{"127.0.0.1", "NEWGROUPS 20000101 000000 GMT", 231, both},
			} {
				c := s.dialFrom(t, tc.ip)
				c.Deadline = 5 * time.Second
				code, line, err := c.TryCmd("%s", tc.command)
				if err != nil || code != tc.code {
					t.Fatalf("LIST waited for a record rejected by its pattern or listgroups: %s: %q, %v", tc.command, line, err)
				}
				got, err := c.TryReadDotLines()
				if err != nil {
					t.Fatalf("LIST waited for a record rejected by its pattern or listgroups: %s: %v", tc.command, err)
				}
				var names []string
				for _, line := range got {
					if f := strings.Fields(line); len(f) > 0 {
						names = append(names, f[0])
					}
				}
				slices.Sort(names)
				if !slices.Equal(names, tc.want) {
					t.Fatalf("%s returned %q, want %q (groups after the busy record must be listed)", tc.command, got, tc.want)
				}
				c.ExpectCmd(205, "QUIT")
			}
			release()
			checkUnrestricted()
		})
	}
}

// activeRecordOffset returns the file offset of the live record of group in dactive.kp.
func activeRecordOffset(t *testing.T, db, group string) int64 {
	t.Helper()
	data, err := os.ReadFile(db)
	if err != nil {
		t.Fatal(err)
	}
	offset := int64(0)
	for _, line := range bytes.SplitAfter(data, []byte{'\n'}) {
		if len(line) > 15 && line[0] == '+' && bytes.HasPrefix(line[15:], []byte(group+" ")) {
			return offset
		}
		offset += int64(len(line))
	}
	t.Fatalf("active record for %s not found", group)
	return 0
}

// replaceActiveRecord does what KPDBWrite() does when a record grows: append a
// longer copy, bump the append sequence (bytes 8-15 of the header) and the
// append counter (bytes 26-33), then mark the old record deleted.
func replaceActiveRecord(t *testing.T, db string, off int64) {
	t.Helper()
	f, err := os.OpenFile(db, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	data, err := os.ReadFile(db)
	if err != nil {
		t.Fatal(err)
	}
	end := bytes.IndexByte(data[off:], '\n')
	if end < 0 || data[off] != '+' {
		t.Fatalf("no live record at %d", off)
	}
	line := append([]byte("+00000000"), data[off+9:off+int64(end)]...)
	line = append(line, " ZZ=a-longer-copy-of-the-record-so-that-it-no-longer-fits-in-place\n"...)
	if _, err := f.WriteAt(line, int64(len(data))); err != nil {
		t.Fatal(err)
	}
	for _, at := range []int64{8, 26} {
		v, err := strconv.ParseUint(string(data[at:at+8]), 16, 32)
		if err != nil {
			t.Fatalf("header field at %d: %v", at, err)
		}
		if _, err := f.WriteAt([]byte(fmt.Sprintf("%08x", v+1)), at); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.WriteAt([]byte("-"), off); err != nil {
		t.Fatal(err)
	}
}

// A reader that waits for the record lock of a group while a writer replaces
// that record with a longer copy (append, then mark the old one deleted) must
// still list the group: the old copy it looked up is gone, the new one has to
// be found again by name.
func TestReaderListRecordReplacedWhileLocked(t *testing.T) {
	const access = `vserverdef DEFAULT
    hostname ` + PathHost + `
end
readerdef unrestricted
    read yes
end
access 127.0.0.3 unrestricted
`
	for _, tc := range []struct {
		name, command string
		code          int
		cached        bool
	}{
		{"list-active", "LIST ACTIVE e2e.right", 215, false},
		{"list-newsgroups", "LIST NEWSGROUPS e2e.right", 215, false},
		{"cached-newgroups", "NEWGROUPS 20000101 000000 GMT", 231, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, spool := spoolStore(t)
			s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: access,
				Groups: []string{"e2e.left", "e2e.right"}})
			names := func(lines []string) []string {
				var got []string
				for _, line := range lines {
					if f := strings.Fields(line); len(f) > 0 {
						got = append(got, f[0])
					}
				}
				slices.Sort(got)
				return got
			}
			want := []string{"e2e.right"}
			if tc.cached {
				want = []string{"e2e.left", "e2e.right"}
				c := s.dialFrom(t, "127.0.0.3") // warm the cache
				c.ExpectCmd(231, "NEWGROUPS 20000101 000000 GMT")
				if got := names(c.ReadDotLines()); !slices.Equal(got, want) {
					t.Fatalf("warming NEWGROUPS returned %q, want %q", got, want)
				}
				c.ExpectCmd(205, "QUIT")
			}
			db := filepath.Join(s.Dir, "dactive.kp")
			off := activeRecordOffset(t, db, "e2e.right")
			release := holdHistLock(t, db, off, 4)
			defer release()
			c := s.dialFrom(t, "127.0.0.3")
			c.Deadline = 20 * time.Second
			c.MustSend(tc.command + "\r\n")
			time.Sleep(500 * time.Millisecond) // let the reader wait for the record lock
			replaceActiveRecord(t, db, off)
			release()
			c.ExpectStatus(tc.code, tc.command)
			got, err := c.TryReadDotLines()
			if err != nil {
				t.Fatalf("%s: %v", tc.command, err)
			}
			if gotNames := names(got); !slices.Equal(gotNames, want) {
				t.Fatalf("%s after the record was replaced returned %q, want %q", tc.command, got, want)
			}
			c.ExpectCmd(205, "QUIT")
		})
	}
}
