package e2e

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestControlWithoutFromNoFdLeak: a fed article with a Control: header but no From: makes
// ParseControl() return early. It opened dcontrol.ctl before checking From: and did not close
// it, so every such article cost the reader feed process one descriptor until it ran out
// (GHSA-qjg5-xp53-hj7h). After 40 of them the dreaderd processes of the site must not hold
// (nearly) 40 more descriptors. Linux only (/proc).
func TestControlWithoutFromNoFdLeak(t *testing.T) {
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{
		Spool: spool,
		Files: map[string]string{"dcontrol.ctl": "newgroup:*:*:doit\n"},
	})
	noFrom := func(i int) []string {
		var a []string
		for _, l := range newArticle(msgID(t), "e2e.test", fmt.Sprintf("cmsg cancel %d", i), "body") {
			if strings.HasPrefix(l, "From: ") {
				continue
			}
			a = append(a, l)
		}
		// the Control: header goes into the head, before the blank line
		for j, l := range a {
			if l == "" {
				return append(append(append([]string{}, a[:j]...), fmt.Sprintf("Control: cancel <gone%d@e2e.test>", i)), a[j:]...)
			}
		}
		t.Fatal("no blank line in the article")
		return nil
	}
	fds := func() map[int]int {
		m := map[int]int{}
		for _, pid := range sitePids(s, "dreaderd") {
			if c := fdCount(pid); c >= 0 {
				m[pid] = c
			}
		}
		if len(m) == 0 {
			t.Skip("no /proc or no dreaderd processes found")
		}
		return m
	}

	// Keep one feed connection open across the warm-up and the 40 articles:
	// the leak is in the child that handles this connection, and a new
	// connection would be a new pid that the before map does not know.
	p := s.Peer(t)
	sendOne := func(i int) {
		a := noFrom(i)
		id := headerValue(a, "Message-ID")
		expect(t, p, 335, "IHAVE %s", id)
		send(t, p, wire(a))
		status(t, p, 235, "IHAVE control without From")
	}
	sendOne(0) // warm-up: the feed child has opened what it keeps open
	time.Sleep(500 * time.Millisecond)
	before := fds()
	for i := 1; i <= 40; i++ {
		sendOne(i)
	}
	time.Sleep(time.Second)
	worst := 0
	for pid, n := range before {
		if d := fdCount(pid) - n; d > worst {
			worst = d
		}
	}
	if worst >= 20 {
		t.Errorf("40 control articles without From: left the feed child with %d more open descriptors", worst)
	}
	expect(t, p, 205, "QUIT")
}
