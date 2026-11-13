package e2e

import (
	"strings"
	"testing"
	"time"
)

// TestDeepRevPostDuplicateHeaders: a post with two Newsgroups, From, Subject, Date or
// Message-ID headers was accepted. dreaderd's checks (the groups the poster may post to,
// the date, the Message-ID) then went by the last one of each, while a reader shows the
// first. Each of the five is refused now, with rfc5537 on and off; the same post with one
// of each is accepted. These five by decision, not every header RFC 5536 allows once.
func TestDeepRevPostDuplicateHeaders(t *testing.T) {
	// a date of now: a fixed one gets too old for the post's date check in time
	date := "Date: " + time.Now().UTC().Format(time.RFC1123Z)
	for _, m := range []struct {
		name string
		d    *datePoster
	}{
		{"default", newDatePoster(t, DefaultReaderAccess, nil)},
		{"rfc5537_no", newDatePoster(t, plainAccess("    rfc5537 no\n"), nil)},
	} {
		t.Run(m.name, func(t *testing.T) {
			if code, line, _ := m.d.postHdrs(t, "Message-ID: "+msgID(t)); code != 240 {
				t.Fatalf("a post with one of each header: %q, want 240", line)
			}
			for _, c := range []struct {
				name string
				hdrs []string
			}{
				{"Newsgroups", []string{"Newsgroups: e2e.other"}},
				{"From", []string{"From: Other Poster <other@example.invalid>"}},
				{"Subject", []string{"Subject: another subject"}},
				{"Date", []string{date, date}},
				{"Message-ID", []string{"Message-ID: " + msgID(t), "Message-ID: " + msgID(t)}},
			} {
				code, line, _ := m.d.postHdrs(t, c.hdrs...)
				if code != 441 || !strings.Contains(line, "More than one "+c.name+" header") {
					t.Errorf("POST with two %s headers: %q, want 441 More than one %s header", c.name, line, c.name)
				}
			}
		})
	}
}
