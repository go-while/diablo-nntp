package e2e

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestDexpireoverRewriteGzipData: dexpireover -R on a group whose overview data file exists only
// as .gz (made by dexpireover -z). With zlib the data is unpacked into a plain file, bytes
// unchanged. A build with ./configure --without-zlib cannot read it: it used to delete the .gz
// and leave an empty data file with exit 0. Now it leaves the group's files alone, says that a
// zlib build is needed, and exits non-zero. Plain data is rewritten by both builds.
func TestDexpireoverRewriteGzipData(t *testing.T) {
	zlib := builtWithZlib(t)
	for _, compressed := range []bool{true, false} {
		name := "plain"
		if compressed {
			name = "gzip"
		}
		t.Run(name, func(t *testing.T) {
			s, home := overSite(t, map[string]string{"readergrouphash": "hierarchy"})
			s.Run(t, "dgrpctl", "adjust", "e2e.test", "NE", "=1")

			// article 1: one header block at offset 0 of d.0, ending in its NUL guard
			raw := []byte("Subject: overview data\r\nMessage-ID: <gz@example.com>\r\nX-Note: " +
				strings.Repeat("some header text ", 30) + "\r\n\x00")
			head := overHead("e2e.test")
			index := append(head, make([]byte, 1000*32)...)
			slot := index[len(head)+32:]
			o := binary.NativeEndian
			o.PutUint32(slot[0:], 1)
			o.PutUint32(slot[4:], 0)
			o.PutUint32(slot[8:], uint32(len(raw)-1))
			o.PutUint32(slot[12:], 0x952a1fae)
			o.PutUint32(slot[16:], 0x54dcbdde)
			o.PutUint32(slot[20:], 1000)
			o.PutUint32(slot[24:], uint32(time.Now().Unix()))
			over := filepath.Join(home, "e2e", "o.0.e2e.test")
			writeOver(t, over, index)

			data := filepath.Join(home, "e2e", "d.0.e2e.test")
			var gz bytes.Buffer
			if compressed {
				w := gzip.NewWriter(&gz)
				if _, err := w.Write(raw); err != nil {
					t.Fatal(err)
				}
				if err := w.Close(); err != nil {
					t.Fatal(err)
				}
				writeOver(t, data+".gz", gz.Bytes())
			} else {
				writeOver(t, data, raw)
			}

			out, err := s.TryRun("dexpireover", "-R", "-v")
			plain, plainErr := os.ReadFile(data)
			if compressed && !zlib {
				if err == nil {
					t.Errorf("dexpireover -R exits 0 on .gz data without zlib:\n%s", out)
				}
				if !strings.Contains(out, "needs a zlib build") {
					t.Errorf("no message that a zlib build is needed:\n%s", out)
				}
				if b, err := os.ReadFile(data + ".gz"); err != nil || !bytes.Equal(b, gz.Bytes()) {
					t.Errorf("the .gz data file changed or is gone (%v):\n%s", err, out)
				}
				if plainErr == nil {
					t.Errorf("a plain data file of %d bytes was made:\n%s", len(plain), out)
				}
				if b, err := os.ReadFile(over); err != nil || !bytes.Equal(b, index) {
					t.Errorf("the over file changed (%v):\n%s", err, out)
				}
				return
			}
			if err != nil {
				t.Errorf("dexpireover -R: %v\n%s", err, out)
			}
			if plainErr != nil || !bytes.Equal(plain, raw) {
				t.Errorf("data file after the rewrite: %d bytes, want the %d bytes of before (%v):\n%s",
					len(plain), len(raw), plainErr, out)
			}
			if _, err := os.Stat(data + ".gz"); err == nil {
				t.Errorf("the .gz data file is still there next to the plain one:\n%s", out)
			}
		})
	}
}
