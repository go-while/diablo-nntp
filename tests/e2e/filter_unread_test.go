package e2e

// Articles the external spam filter (feederfilter) must get although diab_read() failed on
// them before, and records it must still leave alone (deep review r2 M14, D-241). Helpers:
// filter_firstline_test.go.

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/go-while/nntp-testkit/client"
)

// rawDotFilter is perDotFilter that also keeps the raw bytes it gets in filter.raw: tee
// passes a NUL byte on, the shell's read does not.
const rawDotFilter = `#!/bin/sh
cr=$(printf '\r')
ans=335
tee -a @home@/filter.raw | while IFS= read -r line; do
    printf '%s\n' "$line" >> @home@/filter.log
    case "$line" in
    "Subject: spam$cr") ans="435 spam" ;;
    ".$cr") echo "$ans"; ans=335; sleep 0.3 ;;
    esac
done
`

// Spool header (lib/defs.h SpoolArtHdr, 24 bytes): Magic1 Magic2 Version HeadLen StoreType,
// 3 bytes padding, ArtHdrLen, ArtLen, StoreLen (host order uint32), HdrEnd.
const (
	shHeadLen   = 3
	shStoreType = 4
	shArtHdrLen = 8
	shArtLen    = 12
	stGzip      = 0x02 // STORETYPE_GZIP
	stWire      = 0x04 // STORETYPE_WIRE
)

// hexLines returns n lines of 64 pseudo-random hex digits: text that gzip shrinks to about
// half, not to almost nothing.
func hexLines(seed uint64, n int) []string {
	r := rand.New(rand.NewPCG(seed, seed))
	var l []string
	for range n {
		l = append(l, fmt.Sprintf("%016x%016x%016x%016x", r.Uint64(), r.Uint64(), r.Uint64(), r.Uint64()))
	}
	return l
}

// filterJudged waits until the articles ids are in the nospam (OutLabel) and onlyspam queues as
// wantHam and wantSpam say, and checks that the filter got exactly sent articles.
func filterJudged(t *testing.T, s *Site, ids []string, wantHam, wantSpam []bool, sent int) {
	t.Helper()
	var ham, spam []bool
	for limit := time.Now().Add(15 * time.Second); ; time.Sleep(200 * time.Millisecond) {
		ham, spam = queued(t, s, OutLabel, ids...), queued(t, s, "spamonly", ids...)
		if (slices.Equal(ham, wantHam) && slices.Equal(spam, wantSpam)) || time.Now().After(limit) {
			break
		}
	}
	if !slices.Equal(ham, wantHam) {
		t.Errorf("queued for the nospam feed: %v, want %v", ham, wantHam)
	}
	if !slices.Equal(spam, wantSpam) {
		t.Errorf("queued for the onlyspam feed: %v, want %v", spam, wantSpam)
	}
	b, _ := os.ReadFile(filepath.Join(s.Dir, "filter.log"))
	if n := strings.Count(string(b), "\n.\r\n"); n != sent {
		t.Errorf("the filter got %d articles, want %d", n, sent)
	}
}

// firstSpoolHeader returns the first spool file of a site (the first article of a new site is
// at its offset 0) and the spool header of that first record.
func firstSpoolHeader(t *testing.T, s *Site) (string, []byte) {
	t.Helper()
	files := spoolFiles(t, s)
	if len(files) == 0 {
		t.Fatalf("no spool file")
	}
	f, err := os.Open(files[0])
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	hdr := make([]byte, 24)
	if _, err := f.ReadAt(hdr, 0); err != nil {
		t.Fatal(err)
	}
	if hdr[0] != 0xff || hdr[1] != 0x99 || hdr[shHeadLen] != 24 {
		t.Fatalf("first spool header % x: want the magic ff 99 and HeadLen 24", hdr)
	}
	return files[0], hdr
}

// TestFilterUnreadArticlesJudged (deep review r2 M14, D-241): diab_read() failed on any NUL
// byte in the stored article and on every record of a compressed spool object (compresslvl);
// DiabFilter() then sent nothing and the article counted as "not spam", so it reached the
// nospam feed unjudged (on main also the onlyspam feed). rawDotFilter calls the articles with the
// subject "spam" spam and the others not. Each article must get its own verdict:
//   - nul: a spam article with a NUL byte in its body (the spool and the filter's raw input
//     hold the NUL) goes only to the onlyspam feed, a plain one after it only to the nospam
//     feed (the NUL does not put the filter out of step);
//   - compressed: on a compressed spool object (records with the gzip store type) a small
//     spam article and a 260 KB plain one whose gzip member is over 64 KB (several 16 KB
//     reads when it is inflated); the filter gets its last line;
//   - compressed-wireformat: the same with wireformat on (gzip and wire store types).
//
// Fails on main and on gw-reply-codes-filter: the NUL article and every article on the
// compressed spool go to the nospam feed, not judged.
func TestFilterUnreadArticlesJudged(t *testing.T) {
	t.Run("nul", func(t *testing.T) {
		s := filterSite(t, rawDotFilter, "", nil)
		a, b, c := msgID(t), msgID(t), msgID(t)
		injectLines(t, s, a, newArticle(a, "e2e.test", "spam", "body"))
		injectLines(t, s, b, newArticle(b, "e2e.test", "spam", "a\x00b", "after the NUL"))
		injectLines(t, s, c, newArticle(c, "e2e.test", "ham", "body"))
		var spool []byte
		for _, f := range spoolFiles(t, s) {
			d, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			spool = append(spool, d...)
		}
		if !bytes.Contains(spool, []byte("a\x00b\n")) {
			t.Fatalf("the spool does not hold the article with the NUL byte (\"a\\x00b\")")
		}
		filterJudged(t, s, []string{a, b, c}, []bool{false, false, true}, []bool{true, true, false}, 3)
		if raw, _ := os.ReadFile(filepath.Join(s.Dir, "filter.raw")); !bytes.Contains(raw, []byte("\r\na\x00b\r\nafter the NUL\r\n")) {
			t.Errorf("the filter did not get the body line with the NUL byte")
		}
	})

	for _, mode := range []string{"compressed", "compressed-wireformat"} {
		t.Run(mode, func(t *testing.T) {
			skipWithoutZlib(t)
			conf := map[string]string{}
			want := byte(stGzip)
			if mode == "compressed-wireformat" {
				conf["wireformat"] = "on"
				want |= stWire
			}
			s := filterSite(t, rawDotFilter, compressedSpoolCtl, conf)
			a, b := msgID(t), msgID(t)
			big := hexLines(1, 4000)
			injectLines(t, s, a, newArticle(a, "e2e.test", "spam", "body"))
			injectLines(t, s, b, newArticle(b, "e2e.test", "ham", big...))
			_, hdr := firstSpoolHeader(t, s)
			if hdr[shStoreType]&(stGzip|stWire) != want {
				t.Fatalf("store type of the first record %#x, want %#x", hdr[shStoreType], want)
			}
			var size int64 // both records (a 10-minute spool directory boundary may split them)
			for _, f := range spoolFiles(t, s) {
				st, err := os.Stat(f)
				if err != nil {
					t.Fatal(err)
				}
				size += st.Size()
			}
			if bigLen := int64(len(big) * 65); size <= 4*16384 || size >= bigLen {
				t.Fatalf("spool files of %d bytes: want the large article compressed (under %d bytes) and over 64 KB", size, bigLen)
			}
			filterJudged(t, s, []string{a, b}, []bool{false, true}, []bool{true, false}, 2)
			if raw, _ := os.ReadFile(filepath.Join(s.Dir, "filter.raw")); !bytes.Contains(raw, []byte(big[len(big)-1]+"\r\n")) {
				t.Errorf("the filter did not get the whole large article")
			}
		})
	}
}

// TestFilterBadRecordPassesLogged (deep review r2 M14, D-241): a record diab_read() must not
// judge, because its spool header or its gzip member is damaged, is not sent to the filter: it
// passes as before (to the nospam feed, not to the onlyspam feed, which gets only what the
// filter called spam), "cannot read article ..., not filtered" is logged, and the
// articles after it get their own verdicts. The damage is written into the spool file between
// the store and the filter: the diablo master is stopped (SIGSTOP) while the article is
// offered, so its SOUT line waits in the pipe; the test changes the record (the first of the
// site's only spool file, offset 0) and lets the master go on (SIGCONT).
func TestFilterBadRecordPassesLogged(t *testing.T) {
	if runtime.GOOS == "illumos" || runtime.GOOS == "solaris" {
		t.Skip("needs the SOUT line to wait in the pipe while the master is stopped")
	}
	for _, tc := range []struct {
		name string
		gzip bool
		// damage returns the bytes to write at off
		damage func(hdr []byte) (off int64, val []byte)
	}{
		{"gzip data", true, func([]byte) (int64, []byte) { return 24 + 64, bytes.Repeat([]byte{0x55}, 16) }},
		{"gzip ArtLen off by one", true, func(h []byte) (int64, []byte) {
			return shArtLen, binary.NativeEndian.AppendUint32(nil, binary.NativeEndian.Uint32(h[shArtLen:])+1)
		}},
		{"gzip HeadLen 30", true, func([]byte) (int64, []byte) { return shHeadLen, []byte{30} }},
		{"plain HeadLen 30", false, func([]byte) (int64, []byte) { return shHeadLen, []byte{30} }},
		{"plain HeadLen 7", false, func([]byte) (int64, []byte) { return shHeadLen, []byte{7} }},
		{"plain ArtHdrLen past the article", false, func(h []byte) (int64, []byte) {
			return shArtHdrLen, binary.NativeEndian.AppendUint32(nil, binary.NativeEndian.Uint32(h[shArtLen:])+1)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctl := ""
			if tc.gzip {
				skipWithoutZlib(t)
				ctl = compressedSpoolCtl
			}
			s := filterSite(t, rawDotFilter, ctl, nil)
			bad, spam, ham := msgID(t), msgID(t), msgID(t)

			p := s.Peer(t)
			defer p.Close()
			if err := s.Proc.Signal(syscall.SIGSTOP); err != nil {
				t.Fatalf("SIGSTOP diablo: %v", err)
			}
			t.Cleanup(func() { _ = s.Proc.Signal(syscall.SIGCONT) }) // a stopped master cannot be shut down
			p.ExpectCmd(335, "IHAVE %s", bad)
			p.MustSend(client.Stuff(newArticle(bad, "e2e.test", "spam", hexLines(2, 16)...)))
			p.ExpectStatus(235, "IHAVE "+bad)
			p.ExpectCmd(205, "QUIT")
			time.Sleep(300 * time.Millisecond) // the 235 is written after the SOUT line

			file, hdr := firstSpoolHeader(t, s)
			if got := hdr[shStoreType]&stGzip != 0; got != tc.gzip {
				t.Fatalf("first record gzip %v, want %v", got, tc.gzip)
			}
			off, val := tc.damage(hdr)
			f, err := os.OpenFile(file, os.O_RDWR, 0)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.WriteAt(val, off); err != nil {
				t.Fatal(err)
			}
			_ = f.Close()
			if err := s.Proc.Signal(syscall.SIGCONT); err != nil {
				t.Fatalf("SIGCONT diablo: %v", err)
			}

			injectLines(t, s, spam, newArticle(spam, "e2e.test", "spam", "body"))
			injectLines(t, s, ham, newArticle(ham, "e2e.test", "ham", "body"))
			filterJudged(t, s, []string{bad, spam, ham}, []bool{true, false, true}, []bool{false, true, false}, 2)
			if log := s.logOutput(); !strings.Contains(log, "cannot read article") || !strings.Contains(log, "not filtered") {
				t.Errorf("no \"cannot read article ..., not filtered\" log line:\n%s", log)
			}
		})
	}
}
