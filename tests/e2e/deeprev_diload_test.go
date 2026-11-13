package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestDeepRevDiloadInputError (L7-03): diload stopped at a line it could not read and
// printed "Format error", but then set its exit status from HistoryClose() alone, so it
// exited 0. The recipe in diload's own header ("didump dhistory | diload -f dhistory.new;
// if status == 0, mv dhistory.new dhistory") then installed a history that ends where the
// bad line was, for example a dump cut off in the middle of a line.
//
// diload must exit non-zero after a line it cannot read. Input that ends cleanly without
// the "." line stays valid (hand-made entry lists and empty input are loaded that way).
func TestDeepRevDiloadInputError(t *testing.T) {
	now := time.Now().Unix() / 60
	entries := fmt.Sprintf("DUMP 11111111.22222222.0 gm=%d ex=0 boff=0 bsize=0 flags=\n"+
		"DUMP 33333333.44444444.0 gm=%d ex=0 boff=0 bsize=0 flags=\n"+
		"DUMP 55555555.66666666.0 gm=%d ex=0 boff=0 bsize=0 flags=\n", now, now, now)
	old := fmt.Sprintf("DUMP aaaaaaaa.bbbbbbbb.0000 gm=%d ex=0 \n"+
		"DUMP cccccccc.dddddddd.0000 gm=%d ex=0 \n", now, now)
	// as didump writes it (ex=%-2d boff=%-7d bsize=%-6d)
	cut := fmt.Sprintf("DUMP 77777777.88888888.0000 gm=%d ex=0  boff=0       bsize=0      flags=", now)
	for _, c := range []struct {
		name   string
		what   string
		stdin  string
		wantOK bool
	}{
		{"complete", "complete", entries + ".\n", true},
		{"no_terminator", "complete without the \".\" line", entries, true},
		{"empty", "empty", "", true},
		{"format_error", "broken off at a line it cannot read", entries + "this is not a dump line\n" + ".\n", false},
		{"cut_in_line", "cut off in the middle of a line", entries + "DUMP 77777777.888", false},
		// didump -o writes the old style, without boff=, bsize= and flags=
		{"old_style", "complete, old style", old + ".\n", true},
		{"old_style_no_newline", "complete, old style, without the newline at the end", strings.TrimSuffix(old, "\n"), true},
		// a new style line cut off inside a field (Terra T5): sscanf() took a line cut after
		// boff= as old style, and anything after bsize=
		{"cut_after_boff", "cut off after boff=", entries + cut[:strings.Index(cut, " bsize=")], false},
		{"cut_in_flags", "cut off in flags=", entries + cut[:strings.Index(cut, "flags=")+len("flags")], false},
		{"cut_in_file", "cut off in file=", entries + cut[:strings.Index(cut, " flags=")] + ` file="P.01/D.0000`, false},
		{"cut_new_line_short", "broken at a new style line that ends after boff=", entries + cut[:strings.Index(cut, " bsize=")] + "\n.\n", false},
		{"partial_flags", "broken at a line with half a flags= field", entries + cut[:strings.Index(cut, "flags=")+len("fla")] + "\n.\n", false},
		{"unknown_field", "broken at a line with an unknown field", entries + cut + " junk=1\n.\n", false},
		// A cut right after a field leaves a complete shorter line, which stays valid: it
		// cannot be told from a cut one (Sol R2, Terra, review of the first version).
		{"cut_after_ex", "cut off after ex= (a complete old style line)", entries + cut[:strings.Index(cut, " boff=")], true},
		{"cut_after_ex_pad", "cut off after ex= and its padding", entries + cut[:strings.Index(cut, "boff=")], true},
		{"cut_after_bsize", "cut off after bsize= (a complete line without flags=)", entries + cut[:strings.Index(cut, " flags=")], true},
		{"didump_style", "complete as didump writes it, -v (file=), an H flag and next=", entries + cut + "\n" +
			strings.Replace(cut, "7777", "9999", 1)[:strings.Index(cut, " flags=")] + ` file="P.01/D.00000000/B.0000" flags=H next=12` + "\n.\n", true},
		// hand made lists (the history tests) leave out flags=
		{"no_flags", "complete, without flags= (hand made)", strings.ReplaceAll(entries, " flags=", "") + ".\n", true},
		{"no_flags_no_newline", "complete, without flags= and the last newline", strings.TrimSuffix(strings.ReplaceAll(entries, " flags=", ""), "\n"), true},
		{"new_style_no_newline", "complete, without the newline at the end", entries + cut, true},
		// lines ending in CR LF, and lists of mixed lines, were loaded before and still are
		{"crlf_old", "old style with CR LF", strings.ReplaceAll(old, "\n", "\r\n") + ".\r\n", true},
		{"crlf_new", "new style with CR LF", strings.ReplaceAll(entries, "\n", "\r\n") + ".\r\n", true},
		{"mixed_old_last", "new style lines, then an old style one without the newline", entries + strings.TrimSuffix(old, "\n"), true},
		{"mixed_no_flags_last", "lines with flags=, then one without and without the newline", entries + cut[:strings.Index(cut, " flags=")], true},
		{"mixed_old_first", "old style lines, then new style ones", old + entries + ".\n", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := NewSite(t, SiteOptions{NoStart: true})
			hist := filepath.Join(s.Dir, "dhistory.new")
			cmd := exec.Command(filepath.Join(s.Bin, "diload"), "-f", hist)
			cmd.Dir = s.Dir
			cmd.Env = append(os.Environ(), s.Env...)
			cmd.Stdin = strings.NewReader(c.stdin)
			out, err := cmd.CombinedOutput()
			switch {
			case c.wantOK && err != nil:
				t.Errorf("diload of a dump that is %s: %v\n%s", c.what, err, out)
			case !c.wantOK && err == nil:
				t.Errorf("diload exits 0 for a dump that is %s; a script that tests the status installs the partial history\n%s",
					c.what, out)
			case !c.wantOK && !strings.Contains(string(out), "Format error"):
				t.Errorf("diload of a dump that is %s: %v, but no \"Format error\" line\n%s", c.what, err, out)
			}
		})
	}
}
