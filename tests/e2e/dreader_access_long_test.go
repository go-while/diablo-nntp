package e2e

import (
	"strings"
	"testing"
	"time"
)

// dreader.access values went into fixed-size fields with strncpy(), cut without a word. A cut
// name, host name, address or secret is another one, so such a value is now refused (logged;
// the file is not installed, as for other errors); free text is cut with a warning.

func vserverWith(line string) string {
	return strings.Replace(DefaultReaderAccess, "vserverdef DEFAULT\n", "vserverdef DEFAULT\n    "+line+"\n", 1)
}

func isService(g string) bool { return strings.HasPrefix(g, "200") || strings.HasPrefix(g, "201") }

// TestAccessLongClusterNameRefused: the cluster name (64 bytes) goes into the Path: and Xref:
// headers dreaderd writes; an 80-character one was cut and dreaderd served under a name that
// is not the site's.
func TestAccessLongClusterNameRefused(t *testing.T) {
	s := NewReaderSite(t, ReaderOptions{SpoolAddr: "127.0.0.1:1", NoStart: true,
		Access: vserverWith("clustername " + strings.Repeat("c", 80) + ".example"),
		Config: map[string]string{"path_generallog": "%s/general.log"}})
	startReaderNoWait(t, s)
	var log string
	waitFor(t, 5*time.Second, "the refusal in the log", func() bool {
		log = generalLog(s)
		return strings.Contains(log, "value of clustername in line 2 is too long")
	})
	if !strings.Contains(log, "not updating") {
		t.Errorf("dreader.access with the long clustername was not refused:\n%s", log)
	}
	if g := readerGreeting(s.Addr); isService(g) {
		t.Errorf("dreaderd gave service with a cut cluster name: %q", strings.TrimSpace(g))
	}
}

// TestAccessLongWelcomeCut: free text (welcome, 256 bytes) is still cut, with a warning, and
// dreaderd serves.
func TestAccessLongWelcomeCut(t *testing.T) {
	s := NewReaderSite(t, ReaderOptions{SpoolAddr: "127.0.0.1:1",
		Access: vserverWith("welcome " + strings.Repeat("w", 300)),
		Config: map[string]string{"path_generallog": "%s/general.log"}})
	if g := readerGreeting(s.Addr); !isService(g) {
		t.Errorf("no service with a long welcome text: %q", g)
	}
	if log := generalLog(s); !strings.Contains(log, "value of welcome in line 2 cut to 255 characters") {
		t.Errorf("no warning for the cut welcome text:\n%s", log)
	}
}

// TestAccessLongIdentUserRefused: the ident user of an access entry is a 16-byte field, but
// its length was checked against the 100-byte pattern field: a 20-character user name was cut
// to 15 characters, and the entry then matched a client whose ident reply was that shorter
// name. Such an entry is now skipped with "Bad length"; the other entries still apply.
func TestAccessLongIdentUserRefused(t *testing.T) {
	s := NewReaderSite(t, ReaderOptions{SpoolAddr: "127.0.0.1:1",
		Access: DefaultReaderAccess + "access abcdefghijklmnopqrst@127.0.0.3 rp\n",
		Config: map[string]string{"path_generallog": "%s/general.log"}})
	if g := readerGreeting(s.Addr); !isService(g) {
		t.Errorf("no service for 127.0.0.1: %q", g)
	}
	if log := generalLog(s); !strings.Contains(log, "Bad length for ident user abcdefghijklmnopqrst") {
		t.Errorf("the long ident user was not refused:\n%s", log)
	}
}
