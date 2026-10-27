package e2e

import (
	"strings"
	"testing"
)

// TestHostCacheLongLabel: a label name longer than the host cache loader's buffer (255) must not
// overwrite the stack (loadDnewsFeeds copied it with strcpy). The label's hosts are left out,
// and the labels around it are read as usual.
func TestHostCacheLongLabel(t *testing.T) {
	long := strings.Repeat("l", 3000)
	dump := hostCacheDump(t, GlobalLabel+"label "+long+"\n    inhost 127.0.0.4\nend\nlabel peer\n    inhost "+PeerIP+"\nend\n", "")
	if !strings.Contains(dump, PeerIP+" label=peer") {
		t.Errorf("the label after the long one is missing from the host cache:\n%s", dump)
	}
	if strings.Contains(dump, "127.0.0.4") {
		t.Errorf("a host of the over-long label was entered:\n%s", dump)
	}
}
