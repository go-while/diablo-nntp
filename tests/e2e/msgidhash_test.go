package e2e

// msgidhash: the hash of Message-IDs in the history, the overview index and the caches (D-210).

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/bits"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	testkit "github.com/go-while/nntp-testkit"
	"github.com/go-while/nntp-testkit/client"
)

// sipHash24 is SipHash-2-4 (Aumasson, Bernstein) of msg with the key k0, k1, written here
// from the paper, independently of lib/hash.c.
func sipHash24(k0, k1 uint64, msg []byte) uint64 {
	v0, v1 := uint64(0x736f6d6570736575)^k0, uint64(0x646f72616e646f6d)^k1
	v2, v3 := uint64(0x6c7967656e657261)^k0, uint64(0x7465646279746573)^k1
	round := func() {
		v0 += v1
		v1 = bits.RotateLeft64(v1, 13) ^ v0
		v0 = bits.RotateLeft64(v0, 32)
		v2 += v3
		v3 = bits.RotateLeft64(v3, 16) ^ v2
		v0 += v3
		v3 = bits.RotateLeft64(v3, 21) ^ v0
		v2 += v1
		v1 = bits.RotateLeft64(v1, 17) ^ v2
		v2 = bits.RotateLeft64(v2, 32)
	}
	n := len(msg)
	for len(msg) >= 8 {
		m := binary.LittleEndian.Uint64(msg)
		v3 ^= m
		round()
		round()
		v0 ^= m
		msg = msg[8:]
	}
	b := uint64(n) << 56
	for i, c := range msg {
		b |= uint64(c) << (8 * i)
	}
	v3 ^= b
	round()
	round()
	v0 ^= b
	v2 ^= 0xff
	for i := 0; i < 4; i++ {
		round()
	}
	return v0 ^ v1 ^ v2 ^ v3
}

// sipMsgIdHash is the hash "msgidhash siphash" stores for id, as dilookup prints it (h1.h2):
// SipHash-2-4 with the fixed key "Diablo msgid key", 62 bits in the crc layout (bit 31 of h1
// set, bit 31 of h2 clear).
func sipMsgIdHash(id string) string {
	key := []byte("Diablo msgid key")
	m := sipHash24(binary.LittleEndian.Uint64(key), binary.LittleEndian.Uint64(key[8:]), []byte(id))
	return fmt.Sprintf("%08x.%08x", uint32(m>>32)|0x80000000, uint32(m)&0x7fffffff)
}

// dilookupHash runs dilookup id (with -C config if config is not "") and returns the hash it
// prints and whether the id was found.
func dilookupHash(t *testing.T, s *Site, config, id string) (hv string, found bool) {
	t.Helper()
	args := []string{id}
	if config != "" {
		args = []string{"-C", config, id}
	}
	out, err := s.TryRun("dilookup", args...)
	if err == nil {
		// found: " [D.xxxxxxxx/B.xxxx hv=h1.h2 spool=..."
		if _, after, ok := strings.Cut(out, " hv="); ok {
			hv, _, _ = strings.Cut(after, " ")
		}
		return hv, true
	}
	// "Not Found: <id> (h1.h2)"
	if !strings.HasPrefix(out, "Not Found: ") {
		t.Fatalf("dilookup %q: %v\n%s", id, err, out)
	}
	if l, r := strings.LastIndex(out, "("), strings.LastIndex(out, ")"); l >= 0 && r > l {
		hv = out[l+1 : r]
	}
	return hv, false
}

// offer offers art with IHAVE from p and returns the code of the final reply: 435 when the
// Message-ID is refused at once, else the reply to the article.
func offer(t *testing.T, p *client.Conn, id string, art []string) int {
	t.Helper()
	code, line := p.Cmd("IHAVE %s", id)
	if code != 335 {
		return code
	}
	p.MustSend(client.Stuff(art))
	reply, err := p.ReadLine()
	if err != nil {
		t.Fatalf("IHAVE %s: %v (after %q)", id, err, line)
	}
	code, _ = client.ParseCode(reply)
	return code
}

// TestMsgIdHashSipHash: with "msgidhash siphash" the history holds SipHash-2-4 of the
// Message-ID with the fixed key, the same on every host. The Go SipHash is first checked
// against the paper's test vectors, then dilookup's stored hash (of the one article fed)
// and computed hash (of the others) must equal it, and for the first IDs the values pinned
// here: the stored format must not change.
func TestMsgIdHashSipHash(t *testing.T) {
	// SipHash paper, appendix A: key 00..0f; the empty message and 00..0e.
	k := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	k0, k1 := binary.LittleEndian.Uint64(k), binary.LittleEndian.Uint64(k[8:])
	if got := sipHash24(k0, k1, nil); got != 0x726fdb47dd0e0e31 {
		t.Fatalf("test SipHash-2-4 of the empty message: %016x, want 726fdb47dd0e0e31", got)
	}
	if got := sipHash24(k0, k1, k[:15]); got != 0xa129ca6149be45e5 {
		t.Fatalf("test SipHash-2-4 of 00..0e: %016x, want a129ca6149be45e5", got)
	}

	long := "<"
	for i := 1; i < 241; i++ {
		long += string(rune('a' + (i*7)%26))
	}
	long += "@long.example>"
	ids := []string{
		"<abc@example.com>",
		"<20261102.123456.7890@news.e2e.test>",
		"<\xe4\xf6\xfc\xff\x80\x01 \x7f@example.com>", // bytes with the high bit set
		long,
		"<1510900044@gn>",
		"<1510900045@gn>",
	}
	pinned := []string{"c11e68ec.00b834a0", "8589f3d5.24bbbdae", "a97d5fc1.77717756", "96bb7893.0e2a54b0"}
	// every length modulo 8, so each tail case of the last word is taken
	for n := 8; n < 16; n++ {
		ids = append(ids, "<"+strings.Repeat("x", n-len("<@e.t>"))+"@e.t>")
	}

	s := NewSite(t, SiteOptions{Config: map[string]string{"msgidhash": "siphash"}})
	const stored = 1
	p := s.Peer(t)
	p.ExpectCmd(335, "IHAVE %s", ids[stored])
	p.MustSend(client.Stuff(newArticle(ids[stored], "e2e.test", "hash", "body")))
	p.ExpectStatus(235, "IHAVE")
	p.ExpectCmd(205, "QUIT")

	for i, id := range ids {
		hv, found := dilookupHash(t, s, "", id)
		if found != (i == stored) {
			t.Errorf("dilookup %q: found %v, want %v", id, found, i == stored)
		}
		if want := sipMsgIdHash(id); hv != want {
			t.Errorf("msgidhash siphash of %q (length %d): dilookup %q, want %s", id, len(id), hv, want)
		}
		if i < len(pinned) && hv != pinned[i] {
			t.Errorf("msgidhash siphash of id %d: dilookup %q, pinned %s", i, hv, pinned[i])
		}
	}
}

// TestMsgIdHashNeighbours: "<1510900044@gn>" and "<1510900045@gn>" differ only in bit 0 of
// the 5th byte from the end. hash crc folds that bit away, so both get the hash
// eeae8c4f.667591ad and the second one is refused as a duplicate of the first (D-210, found
// on an archive feed). With msgidhash siphash both are accepted and each is found under its
// own hash.
func TestMsgIdHashNeighbours(t *testing.T) {
	ids := []string{"<1510900044@gn>", "<1510900045@gn>"}
	for _, c := range []struct {
		method string
		want   int // reply to the second one
	}{
		{"hash", 435}, // hash crc, the default: today's behaviour, pinned
		{"siphash", 235},
	} {
		t.Run(c.method, func(t *testing.T) {
			var config map[string]string // the default: the same on a build without msgidhash
			if c.method != "hash" {
				config = map[string]string{"msgidhash": c.method}
			}
			s := NewSite(t, SiteOptions{Config: config})
			p := s.Peer(t)
			if code := offer(t, p, ids[0], newArticle(ids[0], "e2e.test", "first", "body")); code != 235 {
				t.Fatalf("IHAVE %s: %d, want 235", ids[0], code)
			}
			if code := offer(t, p, ids[1], newArticle(ids[1], "e2e.test", "second", "body")); code != c.want {
				t.Errorf("IHAVE %s after %s: %d, want %d", ids[1], ids[0], code, c.want)
			}
			p.ExpectCmd(205, "QUIT")
			if c.method != "siphash" {
				return
			}
			for _, id := range ids {
				hv, found := dilookupHash(t, s, "", id)
				if !found || hv != sipMsgIdHash(id) {
					t.Errorf("dilookup %s: found %v, hash %q, want found under %s", id, found, hv, sipMsgIdHash(id))
				}
			}
		})
	}
}

// TestMsgIdHashMismatch: the history header records the method of its entries. A history
// of msgidhash hash is written as version 2 with the magic a1b2c3d4, as before; one of
// msgidhash siphash as version 3 with its own magic a1b2c3d5, so that every older program
// refuses it (the older dhisexpire and didump only warned about a newer version). A program whose diablo.config has the other method refuses
// the history instead of missing every entry, and dhisexpire refuses to copy the entries
// into a history of the other method. dhisctl -h shows the file's method whatever
// diablo.config says.
func TestMsgIdHashMismatch(t *testing.T) {
	for _, c := range []struct {
		method, other, version, magic string
	}{
		{"hash", "siphash", "2", "0xa1b2c3d4"},
		{"siphash", "hash", "3", "0xa1b2c3d5"},
	} {
		t.Run(c.method, func(t *testing.T) {
			s := NewSite(t, SiteOptions{Config: map[string]string{"msgidhash": c.method}})
			id := msgID(t)
			p := s.Peer(t)
			if code := offer(t, p, id, newArticle(id, "e2e.test", "mismatch", "body")); code != 235 {
				t.Fatalf("IHAVE %s: %d, want 235", id, code)
			}
			p.ExpectCmd(205, "QUIT")

			other := filepath.Join(s.Dir, "other.config")
			s.WriteFile(t, "other.config", s.ReadFile(t, "diablo.config")+"\nmsgidhash "+c.other+"\n")
			hist := filepath.Join(s.Dir, "dhistory")

			for _, args := range [][]string{{"-C", other}, nil} {
				out := s.Run(t, "dhisctl", append(args, "-h", hist)...)
				if !strings.Contains(out, "Magic     : "+c.magic+"\n") ||
					!strings.Contains(out, "Version   : "+c.version+"\n") ||
					!strings.Contains(out, "MsgIdHash : "+c.method+"\n") {
					t.Errorf("dhisctl %v -h: want Magic %s, Version %s and MsgIdHash %s:\n%s", args, c.magic, c.version, c.method, out)
				}
			}

			want := "DHistory is hashed with msgidhash " + c.method + ", diablo.config has msgidhash " + c.other
			if out, err := s.TryRun("dilookup", "-C", other, id); err == nil || !strings.Contains(out, want) {
				t.Errorf("dilookup with msgidhash %s on a %s history: err=%v, want %q:\n%s", c.other, c.method, err, want, out)
			}
			if hv, found := dilookupHash(t, s, "", id); !found {
				t.Errorf("dilookup with msgidhash %s: %s not found (%s)", c.method, id, hv)
			}

			if out, err := s.TryRun("dhisexpire", "-C", other, hist, hist+".new"); err == nil ||
				!strings.Contains(out, "hashed with msgidhash "+c.method+", diablo.config has msgidhash "+c.other+": not expired") {
				t.Errorf("dhisexpire with msgidhash %s on a %s history: err=%v:\n%s", c.other, c.method, err, out)
			}
			if fileExists(hist + ".new") {
				t.Errorf("dhisexpire wrote %s.new for a history of the other method", hist)
			}
		})
	}
}

// TestMsgIdHashReaderOverview: the reader's overview index stores the Message-ID hash of
// each article (oa_MsgHash), and XHDR by Message-ID finds the article through it. With
// msgidhash siphash, the neighbouring pair that collides under hash crc is fed to dreaderd
// and each Message-ID gets its own article's header.
func TestMsgIdHashReaderOverview(t *testing.T) {
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Config: map[string]string{"msgidhash": "siphash"}})
	ids := []string{"<1510900044@gn>", "<1510900045@gn>"}
	feedReader(t, s, st, "e2e.test",
		newArticle(ids[0], "e2e.test", "first of the pair", "body"),
		newArticle(ids[1], "e2e.test", "second of the pair", "body"))

	c := s.Reader(t)
	defer c.ExpectCmd(205, "QUIT")
	testkit.WaitFor(t, testkit.Deadline(), "both articles in GROUP e2e.test", func() bool {
		_, line := c.Cmd("GROUP e2e.test")
		f := strings.Fields(line)
		return len(f) > 1 && f[0] == "211" && f[1] == "2"
	})
	for i, subject := range []string{"first of the pair", "second of the pair"} {
		c.ExpectCmd(221, "XHDR Subject %s", ids[i])
		if got := c.ReadDotLines(); len(got) != 1 || got[0] != ids[i]+" "+subject {
			t.Errorf("XHDR Subject %s: %q, want [%q]", ids[i], got, ids[i]+" "+subject)
		}
	}
}

// runWithStdin runs the Diablo program name with stdin as its input and returns its
// standard output, its standard error and its error.
func runWithStdin(s *Site, stdin, name string, args ...string) (string, string, error) {
	cmd := exec.Command(filepath.Join(s.Bin, name), args...)
	cmd.Dir = s.Dir
	cmd.Env = append(os.Environ(), s.Env...)
	cmd.Stdin = strings.NewReader(stdin)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	return out.String(), errOut.String(), err
}

// TestMsgIdHashDumpLoad: diload loads the hashes of a didump as they are, so a dump must say
// which method they are of. didump of a siphash history writes "MSGIDHASH siphash" before
// the entries (a dump of a hash history, and didump -m for dilookup -s, are unchanged), and
// diload refuses a dump of another
// method than its diablo.config's before it makes a history; a dump of its own method loads
// into a history of that method with the same entries.
func TestMsgIdHashDumpLoad(t *testing.T) {
	for _, c := range []struct {
		method, other, version string
	}{
		{"hash", "siphash", "2"},
		{"siphash", "hash", "3"},
	} {
		t.Run(c.method, func(t *testing.T) {
			s := NewSite(t, SiteOptions{Config: map[string]string{"msgidhash": c.method}})
			id := msgID(t)
			p := s.Peer(t)
			if code := offer(t, p, id, newArticle(id, "e2e.test", "dump", "body")); code != 235 {
				t.Fatalf("IHAVE %s: %d, want 235", id, code)
			}
			p.ExpectCmd(205, "QUIT")
			other := filepath.Join(s.Dir, "other.config")
			s.WriteFile(t, "other.config", s.ReadFile(t, "diablo.config")+"\nmsgidhash "+c.other+"\n")

			dump, stderr, err := runWithStdin(s, "", "didump", filepath.Join(s.Dir, "dhistory"))
			if err != nil {
				t.Fatalf("didump: %v\n%s", err, stderr)
			}
			if got, want := strings.Contains(dump, "MSGIDHASH siphash\n"), c.method == "siphash"; got != want {
				t.Errorf("didump of a %s history: MSGIDHASH siphash line %v, want %v:\n%s", c.method, got, want, dump)
			}
			if strings.Contains(dump, "MSGIDHASH hash") {
				t.Errorf("didump of a %s history writes a MSGIDHASH hash line:\n%s", c.method, dump)
			}

			// didump -m (hashes only) is read by dilookup -s, not by diload: no MSGIDHASH line
			hashes, stderr, err := runWithStdin(s, "", "didump", "-m", filepath.Join(s.Dir, "dhistory"))
			if err != nil || strings.Contains(hashes, "MSGIDHASH") {
				t.Errorf("didump -m of a %s history: err=%v, want only hashes:\n%s%s", c.method, err, hashes, stderr)
			}
			// dilookup -s looks the stored hash up. Its status is not checked: it counts the
			// misses, and it ends with "argument error: ." for didump's last line, as before.
			if out, stderr, _ := runWithStdin(s, hashes, "dilookup", "-s"); !strings.Contains(out, " hv=") ||
				strings.Contains(out+stderr, "MSGIDHASH") || strings.Contains(stderr, "argument error: siphash") {
				t.Errorf("didump -m | dilookup -s on a %s history: want the stored entry looked up:\n%s%s", c.method, out, stderr)
			}

			refused := filepath.Join(s.Dir, "dhistory.refused")
			_, stderr, err = runWithStdin(s, dump, "diload", "-C", other, "-f", refused)
			if want := "the input is hashed with msgidhash " + c.method + ", diablo.config has msgidhash " + c.other + ": not loaded"; err == nil || !strings.Contains(stderr, want) {
				t.Errorf("diload with msgidhash %s of a %s dump: err=%v, want %q:\n%s", c.other, c.method, err, want, stderr)
			}
			if fileExists(refused) {
				t.Errorf("diload made %s from a dump of the other method", refused)
			}

			loaded := filepath.Join(s.Dir, "dhistory.loaded")
			out, stderr, err := runWithStdin(s, dump, "diload", "-f", loaded)
			if err != nil || !strings.Contains(out, "1/1 entries loaded") {
				t.Fatalf("diload of a %s dump: err=%v\n%s%s", c.method, err, out, stderr)
			}
			if out := s.Run(t, "dhisctl", "-h", loaded); !strings.Contains(out, "Version   : "+c.version+"\n") ||
				!strings.Contains(out, "MsgIdHash : "+c.method+"\n") {
				t.Errorf("dhisctl -h of the loaded history: want Version %s and MsgIdHash %s:\n%s", c.version, c.method, out)
			}
			if again, stderr, err := runWithStdin(s, "", "didump", loaded); err != nil || again != dump {
				t.Errorf("didump of the loaded history: err=%v, differs from the original dump:\n%s\nwant:\n%s%s", err, again, dump, stderr)
			}
		})
	}
}

// TestMsgIdHashMagicVersion: a version 3 history carries the magic a1b2c3d5; with the magic of
// version 2 (a1b2c3d4) its header is damaged and the history is refused, instead of read as a
// version 3 file by this Diablo and taken by older programs.
func TestMsgIdHashMagicVersion(t *testing.T) {
	s := NewSite(t, SiteOptions{Config: map[string]string{"msgidhash": "siphash"}})
	id := msgID(t)
	p := s.Peer(t)
	if code := offer(t, p, id, newArticle(id, "e2e.test", "magic", "body")); code != 235 {
		t.Fatalf("IHAVE %s: %d, want 235", id, code)
	}
	p.ExpectCmd(205, "QUIT")

	b, err := os.ReadFile(filepath.Join(s.Dir, "dhistory"))
	if err != nil {
		t.Fatal(err)
	}
	// the header is in the host's byte order
	var order binary.ByteOrder = binary.LittleEndian
	if binary.BigEndian.Uint32(b) == 0xa1b2c3d5 {
		order = binary.BigEndian
	}
	if order.Uint32(b) != 0xa1b2c3d5 {
		t.Fatalf("magic of a siphash history: %x, want a1b2c3d5", b[:4])
	}
	order.PutUint32(b, 0xa1b2c3d4)
	bad := filepath.Join(s.Dir, "dhistory.badmagic")
	if err := os.WriteFile(bad, b, 0o644); err != nil {
		t.Fatal(err)
	}
	want := "DHistory magic 0xa1b2c3d4 does not match its version 3: damaged header"
	if out, err := s.TryRun("dhisctl", "-h", bad); err == nil || !strings.Contains(out, want) {
		t.Errorf("dhisctl -h of a version 3 history with the version 2 magic: err=%v, want %q:\n%s", err, want, out)
	}
}
