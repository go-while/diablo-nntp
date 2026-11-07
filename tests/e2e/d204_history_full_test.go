package e2e

import (
	"encoding/binary"
	"os"
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
	"github.com/go-while/nntp-testkit/client"
)

// D-204: a version 2 dhistory holds record numbers (HistIndex, uint32). HistoryAdd() took the
// next number from the end of the file without a check, so the record after number 2^32 - 1
// got number 0: its hash slot was emptied, the article was not found, and the same article was
// accepted again when offered again. Number 2^32 - 1 itself is (HistIndex)-1, the "not found" of
// HistoryPosLookupByHash(), so that record could not be updated, expired or cancelled. HistoryAdd()
// now refuses a record past number 2^32 - 2 (436, try again later) and logs "dhistory is full" once.

// TestHistoryFullRefused grows the history (sparse) to 2^32 - 2 records after a first article. The article that gets
// the last record number (2^32 - 2) is stored, found again and cancelled by dcancel -h, which finds
// it through HistoryPosLookupByHash(); the next one is refused with 436, also when it is offered
// again, and the one before it is still found. Fails on main: the next article is taken (235)
// under number 2^32 - 1 (and on the first version of #350, which refused only from 2^32).
func TestHistoryFullRefused(t *testing.T) {
	s := NewSite(t, SiteOptions{Config: map[string]string{"path_generallog": "%s/general.log"}})
	path := strings.TrimSpace(s.Run(t, "dpath", "dhistory"))

	// diablo creates the history with the first article
	p := s.Peer(t)
	first := msgID(t)
	p.ExpectCmd(335, "IHAVE %s", first)
	p.MustSend(client.Stuff(newArticle(first, "e2e.test", "first article", "body")))
	p.ExpectStatus(235, "IHAVE of the first article")

	// HistHead: hmagic, hashSize (uint32), version, henSize, headSize (uint16), native order
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	head := make([]byte, 14)
	_, err = f.ReadAt(head, 0)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	ne := binary.NativeEndian
	magic, hashSize := ne.Uint32(head[0:]), ne.Uint32(head[4:])
	version, henSize, headSize := ne.Uint16(head[8:]), ne.Uint16(head[10:]), ne.Uint16(head[12:])
	if magic != 0xA1B2C3D4 || version != 2 || henSize != 28 {
		t.Fatalf("dhistory header: magic %#x version %d henSize %d, want 0xa1b2c3d4, 2, 28", magic, version, henSize)
	}
	entOff := int64(headSize) + int64(hashSize)*4
	full := entOff + int64(henSize)*(1<<32-2) // records 0 .. 2^32 - 3: the next one is 2^32 - 2
	if err := os.Truncate(path, full); err != nil {
		t.Skipf("no sparse file of %d bytes here: %v", full, err)
	}

	last, next := msgID(t), msgID(t)
	p.ExpectCmd(335, "IHAVE %s", last)
	p.MustSend(client.Stuff(newArticle(last, "e2e.test", "last record number", "body")))
	p.ExpectStatus(235, "IHAVE of the article with record number 2^32 - 2")
	p.ExpectCmd(435, "IHAVE %s", last)
	// the last record is found through the hash chain lookup: dcancel -h marks it expired
	if out := dcancelRun(t, s, last+"\n", "-h", "-v"); !strings.Contains(out, "Cancelled history entry for: "+last) {
		t.Errorf("dcancel -h of the article with record number 2^32 - 2:\n%s", out)
	}

	for i := 0; i < 2; i++ {
		p.ExpectCmd(335, "IHAVE %s", next)
		p.MustSend(client.Stuff(newArticle(next, "e2e.test", "past the last record number", "body")))
		p.ExpectStatus(436, "IHAVE of the article past record number 2^32 - 2")
	}
	p.ExpectCmd(435, "IHAVE %s", last)
	p.ExpectCmd(205, "QUIT")

	if st, err := os.Stat(path); err != nil || st.Size() != full+int64(henSize) {
		t.Errorf("dhistory size %v (%v), want %d: one record appended", st.Size(), err, full+int64(henSize))
	}
	testkit.WaitFor(t, 10*time.Second, "the full history in general.log", func() bool {
		return strings.Contains(generalLog(s), "dhistory is full")
	})
	if n := strings.Count(generalLog(s), "dhistory is full"); n != 1 {
		t.Errorf("%d \"dhistory is full\" lines, want 1:\n%s", n, generalLog(s))
	}
}
