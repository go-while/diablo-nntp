package e2e

import (
	"bytes"
	"compress/zlib"
	"strings"
	"testing"
)

// TestFeederModeCompressTwiceRefused: diablo's MODE COMPRESS (207) makes it inflate what the
// peer sends from then on (zlib, peer to server only; replies stay plain). A second MODE
// COMPRESS, sent inside the compressed stream, gets 502 (as RFC 8054 answers COMPRESS when
// compression is already active), and diablo keeps inflating the same stream: no new inflate
// state is set up (each one used to be allocated and never freed), so the following commands
// on the stream still work.
func TestFeederModeCompressTwiceRefused(t *testing.T) {
	skipWithoutZlib(t)
	s := NewSite(t, SiteOptions{})
	p := s.Peer(t)
	p.ExpectCmd(207, "MODE COMPRESS")

	// Each command is compressed into buf and sent in one write: written straight to the
	// socket, zlib's flush can need a second write, and after QUIT diablo may already have
	// closed (a broken pipe under load).
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	zsend := func(cmd string) {
		t.Helper()
		if _, err := zw.Write([]byte(cmd + "\r\n")); err != nil {
			t.Fatalf("%s: %v", cmd, err)
		}
		if err := zw.Flush(); err != nil {
			t.Fatalf("%s: %v", cmd, err)
		}
		if _, err := p.Conn.Write(buf.Bytes()); err != nil {
			t.Fatalf("%s: %v", cmd, err)
		}
		buf.Reset()
	}
	zsend("MODE STREAM")
	p.ExpectStatus(203, "MODE STREAM (compressed)")
	for i := 0; i < 50; i++ {
		zsend("MODE COMPRESS")
		p.ExpectStatus(502, "second MODE COMPRESS (compressed)")
	}
	// Still the first stream: a fresh inflate state would reject these bytes.
	zsend("MODE STREAM")
	p.ExpectStatus(203, "MODE STREAM after the refused MODE COMPRESS")
	zsend("QUIT")
	p.ExpectStatus(205, "QUIT (compressed)")
}

// TestNoZlibCompressionOff: a build with ./configure --without-zlib has no compression. MODE
// COMPRESS gets 500 and the session goes on uncompressed, and dexpireover -z stops with a
// message instead of leaving the data files as they are. Skipped on a build with zlib.
func TestNoZlibCompressionOff(t *testing.T) {
	if builtWithZlib(t) {
		t.Skip("built with zlib")
	}
	s := NewSite(t, SiteOptions{})
	p := s.Peer(t)
	p.ExpectCmd(500, "MODE COMPRESS")
	p.ExpectCmd(203, "MODE STREAM")
	p.ExpectCmd(205, "QUIT")

	out, err := s.TryRun("dexpireover", "-z")
	if err == nil || !strings.Contains(out, "built without zlib") {
		t.Errorf("dexpireover -z without zlib: err=%v\n%s", err, out)
	}
}
