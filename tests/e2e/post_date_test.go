package e2e

import (
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
	"github.com/go-while/nntp-testkit/client"
	"github.com/go-while/nntp-testkit/fakeserver"
)

// datePoster is a reader with an outgoing server: post returns POST's reply, and for an
// accepted article the header the outgoing server got.
type datePoster struct {
	s    *Site
	mu   sync.Mutex
	arts [][]string
}

func newDatePoster(t *testing.T, access string, config map[string]string) *datePoster {
	t.Helper()
	_, spool := spoolStore(t)
	d := &datePoster{}
	post := fakeserver.New(t, fakeserver.Options{Handler: fakeserver.PeerHandler(&fakeserver.PeerOptions{
		OnArticle: func(id string, wire []string) {
			d.mu.Lock()
			defer d.mu.Unlock()
			d.arts = append(d.arts, wire)
		},
	})})
	d.s = NewReaderSite(t, ReaderOptions{Spool: spool, Post: post, Access: access, Config: config})
	return d
}

func (d *datePoster) post(t *testing.T, hdrs ...string) (int, string, []string) {
	t.Helper()
	return d.postHdrs(t, append([]string{"Message-ID: " + msgID(t)}, hdrs...)...)
}

// postHdrs is post without the Message-ID it adds.
func (d *datePoster) postHdrs(t *testing.T, hdrs ...string) (int, string, []string) {
	t.Helper()
	d.mu.Lock()
	n := len(d.arts)
	d.mu.Unlock()
	c := d.s.Reader(t)
	defer c.Close()
	c.ExpectCmd(340, "POST")
	c.MustSend(client.Stuff(append(append([]string{
		"From: E2E Poster <poster@example.invalid>",
		"Newsgroups: e2e.test",
		"Subject: dates",
	}, hdrs...), "", "body")))
	code, line, err := c.ReadStatus()
	if err != nil {
		t.Fatalf("POST %q: %v", hdrs, err)
	}
	if code != 240 {
		return code, line, nil
	}
	testkit.WaitFor(t, 15*time.Second, "the outgoing server to receive the post", func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		return len(d.arts) > n
	})
	d.mu.Lock()
	head, _ := splitArticle(slices.Clone(d.arts[n]))
	d.mu.Unlock()
	return code, line, head
}

func rfcDate(d time.Duration) string {
	return time.Now().Add(d).UTC().Format("Mon, 02 Jan 2006 15:04:05 -0700")
}

// TestReaderPostDate (F-07 step 3): the Date dreaderd adds is an RFC 5322 date-time in +0000
// (RFC 5536 3.1.1: not the obsolete "GMT" zone), and with "rfc5537 yes" a poster's Date or
// Injection-Date is refused (441) when it cannot be read, is more than 24 hours in the future,
// or is older than the feeder accepts (feedermaxacceptage, at least 72 hours; RFC 5537 3.5
// step 3). Before, dreaderd wrote "05 Nov 2026 12:00:00 GMT" and took any Date.
func TestReaderPostDate(t *testing.T) {
	d := newDatePoster(t, injAccess(""), nil) // feedermaxacceptage: the default of 14 days
	re := regexp.MustCompile(`^(Mon|Tue|Wed|Thu|Fri|Sat|Sun), [0-3][0-9] (Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec) [0-9]{4} [0-2][0-9]:[0-5][0-9]:[0-6][0-9] \+0000 \(UTC\)$`)
	if code, line, head := d.post(t); code != 240 {
		t.Fatalf("POST without a Date: %q", line)
	} else if v := headerValue(head, "Date"); !re.MatchString(v) {
		t.Errorf("the Date dreaderd added is %q, want an RFC 5322 date in +0000", v)
	}

	for _, c := range []struct {
		hdr  string
		want int
	}{
		{"Date: " + rfcDate(0), 240},
		{"Date: " + rfcDate(0) + " (UTC)", 240}, // the comment dreaderd and INN write
		{"Date: " + rfcDate(23*time.Hour), 240},
		{"Date: " + rfcDate(26*time.Hour), 441},
		{"Date: " + rfcDate(-10*24*time.Hour), 240},
		{"Date: " + rfcDate(-20*24*time.Hour), 441},
		{"Date: not a date", 441},
		{"Injection-Date: " + rfcDate(26*time.Hour), 441},
		{"Injection-Date: " + rfcDate(-time.Hour), 240},
	} {
		if code, line, _ := d.post(t, c.hdr); code != c.want {
			t.Errorf("POST with %q: %q, want %d", c.hdr, line, c.want)
		}
	}
}

// TestReaderPostDateWindowConfig (F-07 step 3): the past limit follows feedermaxacceptage, but
// is never shorter than 72 hours (RFC 5537 3.5 step 3); feedermaxacceptage 0 turns it off.
func TestReaderPostDateWindowConfig(t *testing.T) {
	d := newDatePoster(t, injAccess(""), map[string]string{"feedermaxacceptage": "1"})
	for _, c := range []struct {
		age  time.Duration
		want int
	}{{-2 * 24 * time.Hour, 240}, {-4 * 24 * time.Hour, 441}} {
		if code, line, _ := d.post(t, "Date: "+rfcDate(c.age)); code != c.want {
			t.Errorf("feedermaxacceptage 1, Date %v ago: %q, want %d", -c.age, line, c.want)
		}
	}
	off := newDatePoster(t, injAccess(""), map[string]string{"feedermaxacceptage": "0"})
	if code, line, _ := off.post(t, "Date: Mon, 01 Jan 2001 12:00:00 +0000"); code != 240 {
		t.Errorf("feedermaxacceptage 0, a Date in 2001: %q, want 240", line)
	}
	if code, line, _ := off.post(t, "Date: "+rfcDate(48*time.Hour)); code != 441 || !strings.Contains(line, "future") {
		t.Errorf("feedermaxacceptage 0, a Date 2 days ahead: %q, want 441 (in the future)", line)
	}
}

// TestReaderPostDateDefault (F-07): the default checks a poster's Date and
// Injection-Date. The explicit opt-outs keep the old lax checks; generated dates
// remain in the RFC 5322 form in +0000 in every mode.
func TestReaderPostDateDefault(t *testing.T) {
	for _, c := range []struct {
		name, access string
		want         int
	}{
		{"default", DefaultReaderAccess, 441},
		{"rfc5537 no", plainAccess("    rfc5537 no\n"), 240},
		{"legacytrace yes", plainAccess("    legacytrace yes\n"), 240},
	} {
		t.Run(c.name, func(t *testing.T) {
			d := newDatePoster(t, c.access, nil)
			if code, line, head := d.post(t); code != 240 {
				t.Fatalf("POST without a Date: %q", line)
			} else if v := headerValue(head, "Date"); !injDateRe.MatchString(v) {
				t.Errorf("the Date dreaderd added is %q, want an RFC 5322 date in +0000", v)
			}
			for _, h := range []string{
				"Date: " + rfcDate(26*time.Hour),
				"Date: " + rfcDate(-20*24*time.Hour),
				"Date: not a date",
				"Injection-Date: " + rfcDate(26*time.Hour),
			} {
				if code, line, _ := d.post(t, h); code != c.want {
					t.Errorf("POST with %q: %q, want %d", h, line, c.want)
				}
			}
		})
	}
}
