package e2e

import (
	"strings"
	"testing"
)

// TestSpoolCtlSizeSuffix (D-274): the size suffixes of dspool.ctl (spool minfree, spool
// maxsize, metaspool maxsize) are k, m, g and t, in either case; a plain number is kilobytes
// and only the first letter of a longer suffix (4mb, 4tb) counts. The t suffix multiplied only
// once, so "maxsize 4t" was read as 4 MiB, not 4 TiB. "dexpire -n -h0 -d4" prints the values
// read from dspool.ctl; the existing suffixes must print as before.
func TestSpoolCtlSizeSuffix(t *testing.T) {
	for _, tc := range []struct{ name, size, want string }{
		{"none", "4", "4.000K"},
		{"k", "4k", "4.000K"},
		{"K", "4K", "4.000K"},
		{"m", "4m", "4.000M"},
		{"M", "4M", "4.000M"},
		{"g", "4g", "4.000G"},
		{"G", "4G", "4.000G"},
		{"t", "4t", "4.000T"},
		{"T", "4T", "4.000T"},
		{"mb", "4mb", "4.000M"},
		{"tb", "4tb", "4.000T"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctl := "spool 00\n    minfree " + tc.size + "\n    maxsize " + tc.size + "\nend\n" +
				"metaspool trad\n    maxsize " + tc.size + "\n    spool 00\nend\nexpire * trad\n"
			s := NewSite(t, SiteOptions{NoStart: true, SpoolCtl: ctl})
			out, err := s.TryRun("dexpire", "-n", "-h0", "-d4")
			// dexpire -n only reads the configuration, dumps it and exits with status 0 (also on
			// main); a crash, a non-zero exit or the time limit after the dump is a failure too.
			if err != nil {
				t.Errorf("size %s: dexpire -n -h0 -d4 did not finish cleanly: %v\n%s", tc.size, err, out)
			}
			for _, key := range []string{"SPOOL minfree  : ", "SPOOL maxsize  : ", "meta maxsize: "} {
				if !strings.Contains(out, key+tc.want+"\n") {
					t.Errorf("size %s: want line %q in the dump (dexpire: %v)\n%s", tc.size, key+tc.want, err, out)
				}
			}
		})
	}
}
