package e2e

import (
	"strings"
	"testing"
)

// TestReaderPostMessageIDSyntax (F-07 step 4, RFC 5537 3.5 step 2): with "rfc5537 yes" a
// poster's Message-ID must be a msg-id as RFC 5536 3.1.3 defines it, "<" dot-atom-text "@"
// (dot-atom-text or a "[...]" literal) ">", at most 250 octets and without a comment; others
// are refused with 441. Without rfc5537 (the default) the old lax check stays: such ids are
// taken, as INN does with laxmid. Before, there was only the lax check.
func TestReaderPostMessageIDSyntax(t *testing.T) {
	strict := newDatePoster(t, injAccess(""), nil)
	lax := newDatePoster(t, DefaultReaderAccess, nil)
	// tok makes each id unique: the test name and a counter, a dot-atom-text
	tok := func() string {
		id := msgID(t)
		return strings.TrimPrefix(id[:strings.IndexByte(id, '@')], "<")
	}
	for _, c := range []struct {
		name        string
		id          func() string
		strict, lax int // POST's reply; 0 is not checked
	}{
		{"dot-atom", func() string { return "<" + tok() + ".a-b+c@e2e.test>" }, 240, 240},
		{"domain literal", func() string { return "<" + tok() + "@[192.0.2.1]>" }, 240, 240},
		{"white space around", func() string { return "  <" + tok() + "@e2e.test>  " }, 240, 240},
		{"two dots", func() string { return "<" + tok() + "..x@e2e.test>" }, 441, 240},
		{"dot before @", func() string { return "<" + tok() + ".@e2e.test>" }, 441, 240},
		{"leading dot", func() string { return "<." + tok() + "@e2e.test>" }, 441, 240},
		{"two @", func() string { return "<" + tok() + "@x@e2e.test>" }, 441, 240},
		{"quoted id-left", func() string { return `<"` + tok() + `"@e2e.test>` }, 441, 240},
		{"parenthesis", func() string { return "<" + tok() + "(x)@e2e.test>" }, 441, 240},
		{"comment after", func() string { return "<" + tok() + "@e2e.test> (a comment)" }, 441, 240},
		{"dots in id-right", func() string { return "<" + tok() + "@e2e..test>" }, 441, 240},
		{"backslash in literal", func() string { return "<" + tok() + `@[a\b]>` }, 441, 240},
		{"no @", func() string { return "<" + tok() + ">" }, 441, 0},
		{"251 octets", func() string {
			l := tok()
			return "<" + l + strings.Repeat("x", 251-len(l)-len("<@e2e.test>")) + "@e2e.test>"
		}, 441, 441},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, m := range []struct {
				name string
				d    *datePoster
				want int
			}{{"rfc5537", strict, c.strict}, {"default", lax, c.lax}} {
				if m.want == 0 {
					continue
				}
				id := c.id()
				if code, line, _ := m.d.postHdrs(t, "Message-ID: "+id); code != m.want {
					t.Errorf("%s: POST with Message-ID %q: %q, want %d", m.name, id, line, m.want)
				}
			}
		})
	}
}
