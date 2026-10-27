package e2e

import (
	"compress/zlib"
	"testing"
)

// TestFeederModeCompressTwiceRefused: diablo's MODE COMPRESS (207) makes it inflate what the
// peer sends from then on (zlib, peer to server only; replies stay plain). A second MODE
// COMPRESS, sent inside the compressed stream, gets 502 (as RFC 8054 answers COMPRESS when
// compression is already active), and diablo keeps inflating the same stream: no new inflate
// state is set up (each one used to be allocated and never freed), so the following commands
// on the stream still work.
func TestFeederModeCompressTwiceRefused(t *testing.T) {
	s := NewSite(t, SiteOptions{})
	p := s.Peer(t)
	expect(t, p, 207, "MODE COMPRESS")

	zw := zlib.NewWriter(p.Conn)
	zsend := func(cmd string) {
		t.Helper()
		if _, err := zw.Write([]byte(cmd + "\r\n")); err != nil {
			t.Fatalf("%s: %v", cmd, err)
		}
		if err := zw.Flush(); err != nil {
			t.Fatalf("%s: %v", cmd, err)
		}
	}
	zsend("MODE STREAM")
	status(t, p, 203, "MODE STREAM (compressed)")
	for i := 0; i < 50; i++ {
		zsend("MODE COMPRESS")
		status(t, p, 502, "second MODE COMPRESS (compressed)")
	}
	// Still the first stream: a fresh inflate state would reject these bytes.
	zsend("MODE STREAM")
	status(t, p, 203, "MODE STREAM after the refused MODE COMPRESS")
	zsend("QUIT")
	status(t, p, 205, "QUIT (compressed)")
}
