package e2e

import (
	"strings"
	"testing"
)

// The Message-ID hash (lib/hash.c hhash) is stored in dhistory, the overview index and the
// reader's group files, so a change to it makes every stored article unfindable. The values
// below were computed by hhash before it was rewritten in unsigned arithmetic (left shifts of
// negative int32 values are undefined behaviour). For each of the three "hash" methods the
// site stores one of the Message-IDs (diablo hashes it into dhistory), and dilookup prints
// the stored hash of that one and the computed hash of the others: none may change.
func TestMessageIDHashUnchanged(t *testing.T) {
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
	}
	want := map[string][]string{
		"prime": {"7320cf1c.47ebb1c7", "5f69b871.549fce60", "1336f39b.479149ae", "60848211.37503895"},
		"crc":   {"952a1fae.54dcbdde", "90f282a5.4d654f8f", "87684bb0.05cdd685", "ea8c5d59.5f5d77cf"},
		"ocrc":  {"8da01d8d.6a650953", "f25a6f6a.67ce6583", "8cdb2c9c.4154f1ce", "97e10fbb.1b81dd9e"},
	}
	for _, method := range []string{"prime", "crc", "ocrc"} {
		t.Run(method, func(t *testing.T) {
			s := NewSite(t, SiteOptions{Config: map[string]string{"hash": method}})
			const stored = 1
			p := s.Peer(t)
			expect(t, p, 335, "IHAVE %s", ids[stored])
			send(t, p, wire(newArticle(ids[stored], "e2e.test", "hash", "body")))
			status(t, p, 235, "IHAVE")
			expect(t, p, 205, "QUIT")

			for i, id := range ids {
				out, err := s.TryRun("dilookup", id)
				var hv string
				if i == stored {
					// found: " [D.xxxxxxxx/B.xxxx hv=h1.h2 spool=..."
					if err != nil {
						t.Fatalf("dilookup %q: %v\n%s", id, err, out)
					}
					if _, after, ok := strings.Cut(out, " hv="); ok {
						hv, _, _ = strings.Cut(after, " ")
					}
				} else {
					// "Not Found: <id> (h1.h2)"
					if err == nil || !strings.HasPrefix(out, "Not Found: ") {
						t.Fatalf("dilookup %q: want Not Found, got err=%v\n%s", id, err, out)
					}
					if l, r := strings.LastIndex(out, "("), strings.LastIndex(out, ")"); l >= 0 && r > l {
						hv = out[l+1 : r]
					}
				}
				if hv != want[method][i] {
					t.Errorf("hash %s of id %d: got %q, want %s\n%s", method, i, hv, want[method][i], out)
				}
			}
		})
	}
}
