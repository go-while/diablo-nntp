package e2e

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
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
