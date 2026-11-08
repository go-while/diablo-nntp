package e2e

import (
	"strings"
	"testing"
)

// TestReaderPostRFC5537Default checks the v7 default for the implicit DEFAULT,
// an explicit DEFAULT and a named vserver after an opt-out definition. RFC 5537
// 3.5 steps 2, 3 and 9-11 apply unless rfc5537 no or legacytrace yes opts out.
func TestReaderPostRFC5537Default(t *testing.T) {
	implicit := DefaultReaderAccess[strings.Index(DefaultReaderAccess, "readerdef rp\n"):]
	named := "vserverdef old\n    rfc5537 no\nend\n" + strings.ReplaceAll(DefaultReaderAccess, "vserverdef DEFAULT\n", "vserverdef posting\n")
	named = strings.Replace(named, "readerdef rp\n", "readerdef rp\n    vserver posting\n", 1)
	for _, c := range []struct {
		name, access string
		strict       bool
	}{
		{"implicit default", implicit, true},
		{"explicit default", DefaultReaderAccess, true},
		{"named after opt-out", named, true},
		{"rfc5537 yes", plainAccess("    rfc5537 yes\n"), true},
		{"rfc5537 no", plainAccess("    rfc5537 no\n"), false},
		{"legacytrace no", plainAccess("    legacytrace no\n"), true},
		{"legacytrace yes", plainAccess("    legacytrace yes\n"), false},
	} {
		t.Run(c.name, func(t *testing.T) {
			access := strings.Replace(c.access, "    hostname "+PathHost+"\n", "    hostname "+PathHost+"\n    postpath "+PathHost+"\n", 1)
			d := newDatePoster(t, access, map[string]string{"readerhostname": PathHost, "readerpath": PathHost})
			code, line, head := d.post(t)
			if code != 240 {
				t.Fatalf("ordinary POST: %q, want 240", line)
			}
			if c.strict {
				if !strings.HasPrefix(headerValue(head, "Injection-Info"), PathHost+";") {
					t.Errorf("Injection-Info %q, want path-identity %s", headerValue(head, "Injection-Info"), PathHost)
				}
				if !injDateRe.MatchString(headerValue(head, "Injection-Date")) {
					t.Errorf("Injection-Date %q, want generated date", headerValue(head, "Injection-Date"))
				}
				if p := headerValue(head, "Path"); p != PathHost+"!.POSTED!not-for-mail" {
					t.Errorf("Path %q, want %s!.POSTED!not-for-mail", p, PathHost)
				}
			}
			for _, h := range []string{"NNTP-Posting-Host", "X-Trace"} {
				if hasHeader(head, h) == c.strict {
					t.Errorf("%s present=%v, strict=%v", h, hasHeader(head, h), c.strict)
				}
			}
			if !c.strict && (hasHeader(head, "Injection-Info") || hasHeader(head, "Injection-Date") || strings.Contains(headerValue(head, "Path"), ".POSTED")) {
				t.Errorf("opt-out POST has injection headers or .POSTED: %q", head)
			}
			want := 240
			if c.strict {
				want = 441
			}
			for _, hdrs := range [][]string{
				{"Message-ID: <bad.." + strings.TrimPrefix(msgID(t), "<")},
				{"Message-ID: " + msgID(t), "Date: not a date"},
				{"Message-ID: " + msgID(t), "Injection-Date: not a date"},
			} {
				if code, line, _ := d.postHdrs(t, hdrs...); code != want {
					t.Errorf("POST %q: %q, want %d", hdrs, line, want)
				}
			}
		})
	}
}
