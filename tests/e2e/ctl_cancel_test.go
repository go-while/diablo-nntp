package e2e

import (
	"testing"
)

// TestReaderControlCancelNoNewsgroups: a Control: cancel article without a
// Newsgroups: header must not take the dreaderd reader/feed process down.
// CtlCancel() walked the groups list without a NULL check; groups is only set
// when the article has a Newsgroups: header.  The positive control first
// proves that a normal cancel with Newsgroups: still cancels its target.
//
// NOTE: on bafa2b4 the no-Newsgroups article is refused by NNFeedOverview's
// required-header check ("437 Rejected, headers missing") before ExecuteControl
// runs, so the CtlCancel NULL walk is not reached by this path.  The test pins
// that safe refusal (or a fixed build accepting it and ignoring it) and the
// connection staying in step; the positive control keeps a real cancel honest.
func TestReaderControlCancelNoNewsgroups(t *testing.T) {
	s, st := canLockSiteAny(t, "all")
	r := s.Reader(t)
	target := msgID(t)
	feedAndSettle(t, s, st, r, newArticle(target, "e2e.test", "target", "body"))
	if !present(t, r, 1) {
		t.Fatalf("target is not article 1 of e2e.test")
	}

	// Positive control: a cancel WITH Newsgroups is unchanged.
	feedAndSettle(t, s, st, r, cancelArticle(t, target, "e2e.test"))
	if present(t, r, 1) {
		t.Errorf("positive control: the target was not cancelled")
	}

	// The article under test: From: and Control:, no Newsgroups:.
	mid := msgID(t)
	art := []string{
		"From: E2E <e2e@example.invalid>",
		"Message-ID: " + mid,
		"Subject: cancel without newsgroups",
		"Date: Mon, 02 Jan 2006 15:04:05 -0700",
		"Control: cancel <other@e2e.test>",
		"",
		"body",
	}
	p := s.dialFrom(t, PeerIP)
	expect(t, p, 335, "IHAVE %s", mid)
	send(t, p, wire(art))
	code, line, err := p.ReadStatus()
	if err != nil {
		t.Fatalf("control article without Newsgroups: no reply (process died?): %v", err)
	}
	t.Logf("control article without Newsgroups: %d %q", code, line)
	switch code {
	case 235, 437, 439:
		// 235 accepted, 437/439 refused before the control ran: all safe.
	default:
		t.Errorf("control article without Newsgroups: got %d %q, want 235/437/439", code, line)
	}

	// The feed process must still be in step.
	if code, line := p.Cmd("QUIT"); code != 205 {
		t.Errorf("QUIT after the no-Newsgroups control: got %d %q, want 205", code, line)
	}
}
