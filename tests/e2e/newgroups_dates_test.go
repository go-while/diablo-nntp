package e2e

// NEWGROUPS date and time arguments (RFC 3977 7.3.2): yymmdd or yyyymmdd, hhmmss, optional GMT.

import (
	"strings"
	"testing"
	"time"
)

// newgroupsList sends NEWGROUPS with args and returns the listed groups; it fails unless the
// reply is want (231 or 501).
func newgroupsList(t *testing.T, s *Site, want int, args string) []string {
	t.Helper()
	c := s.Reader(t)
	defer expect(t, c, 205, "QUIT")
	expect(t, c, want, "NEWGROUPS %s", args)
	if want != 231 {
		return nil
	}
	return c.ReadDotLines()
}

// TestNewgroupsDates: invalid months, days and times are 501 (they were rolled over into the
// next month or year); yyyymmdd works for any year (19990101 was 501); the groups made just
// now show for an old date and not for a future one (before 2038: NEWGROUPS compares 32 bit).
func TestNewgroupsDates(t *testing.T) {
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})

	for _, a := range []string{
		"991301 000000", "20001301 000000", "20000001 000000", "20000132 000000",
		"20000140 000000", "20000431 000000", "20010229 000000", "19000229 000000",
		"20000101 240000", "20000101 006000", "20000101 000060",
		"20000101 0000", "20000101 00000000", "20000101 00:00:00", "2000010 000000",
		"20000101x 000000", "0 000000", "200001011 000000", "20000101",
		"00000101 000000", "18991231 000000",
	} {
		t.Run("bad "+a, func(t *testing.T) { newgroupsList(t, s, 501, a) })
	}

	for _, a := range []string{
		"000101 000000", "20000101 000000", "20000101 000000 GMT", "20000229 235959 GMT",
		"19990101 000000", "19991231 235959 GMT", "20240229 120000", "1000101 000000",
		"20000101 000000 gmt", "20000101 000000 Gmt",
	} {
		t.Run("old "+a, func(t *testing.T) {
			got := newgroupsList(t, s, 231, a)
			if !strings.Contains(strings.Join(got, "\n"), "e2e.test") {
				t.Errorf("NEWGROUPS %s: %q, want the group e2e.test (made just now)", a, got)
			}
		})
	}
	for _, a := range []string{"20371231 000000", "20371231 235959 GMT"} {
		t.Run("future "+a, func(t *testing.T) {
			if got := newgroupsList(t, s, 231, a); len(got) != 0 {
				t.Errorf("NEWGROUPS %s: %q, want no group (the date is in the future)", a, got)
			}
		})
	}
}

// TestNewgroupsGMTAnyCase: GMT is a keyword of the ABNF (case-blind). The server runs in
// UTC+14; a time one hour ahead of now, given as GMT in any case, is in the future and lists
// no group. Read as local time it would be 13 hours in the past and list them.
func TestNewgroupsGMTAnyCase(t *testing.T) {
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, NoStart: true})
	s.Env = append(s.Env, "TZ=XXX-14")
	startReader(t, s, "[127.0.0.1]:"+s.Addr[strings.LastIndex(s.Addr, ":")+1:])

	ahead := time.Now().UTC().Add(time.Hour).Format("20060102 150405")
	for _, g := range []string{"GMT", "gmt", "Gmt"} {
		if got := newgroupsList(t, s, 231, ahead+" "+g); len(got) != 0 {
			t.Errorf("NEWGROUPS %s %s: %q, want no group (one hour ahead, UTC)", ahead, g, got)
		}
	}
	// without GMT the same wall clock is local time (UTC+14): 13 hours back, the groups show
	if got := newgroupsList(t, s, 231, ahead); !strings.Contains(strings.Join(got, "\n"), "e2e.test") {
		t.Errorf("NEWGROUPS %s: %q, want the group e2e.test (local time)", ahead, got)
	}
}
