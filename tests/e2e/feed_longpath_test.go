package e2e

import (
	"strings"
	"testing"
	"time"
)

// feedLongPath offers an article with the given Path: header from the peer, checks that it is
// accepted, and that the diablo master (which runs FeedWrite() for every accepted article)
// is still serving afterwards: a new connection is greeted and the same article is refused
// as a duplicate.
func feedLongPath(t *testing.T, s *Site, path string) {
	t.Helper()
	p := s.Peer(t)
	id := msgID(t)
	art := newArticle(id, "e2e.test", "long path", "body")
	art[0] = "Path: " + path
	art = append([]string{"Distribution: world"}, art...)
	expect(t, p, 335, "IHAVE %s", id)
	send(t, p, wire(art))
	status(t, p, 235, "IHAVE")
	expect(t, p, 205, "QUIT")
	s.Dicmd(t, "flush") // the master has handled the article's SOUT line when this returns
	time.Sleep(200 * time.Millisecond)
	select {
	case <-s.Proc.Done():
		t.Fatalf("diablo exited after the article with a long Path: header")
	default:
	}
	p2 := s.Peer(t)
	expect(t, p2, 435, "IHAVE %s", id)
	expect(t, p2, 205, "QUIT")
}

// TestFeedWriteLongFirstPathElement: the master's FeedWrite() copied the first Path: element
// into a 4096-byte buffer with snprintf(buf, element length + 1, ...). With a feeder buffer
// larger than a page ("feederbuffersize 32k" is suggested in the sample diablo.config, and
// it is the default where pages are 16k or 64k) diablo takes header lines that long, and a
// 6000-character element from the peer ran past the master's stack buffer.
func TestFeedWriteLongFirstPathElement(t *testing.T) {
	s := NewSite(t, SiteOptions{Config: map[string]string{"feederbuffersize": "64k"}})
	feedLongPath(t, s, strings.Repeat("a", 6000)+"!not-for-mail")
}

// TestFeedSpamAliasLongPathElements: with a "spamalias" in dnewsfeeds the master's
// feedQuerySpamPaths() copied the last two Path: elements with strcpy() into a 256-byte stack
// buffer. Two 1500-character elements (a header line diablo takes with its default buffer)
// ran past it.
func TestFeedSpamAliasLongPathElements(t *testing.T) {
	feeds := GlobalLabel + `label peer
    inhost ` + PeerIP + `
    spamalias spammer.example
    addgroup *
end
`
	s := NewSite(t, SiteOptions{Newsfeeds: feeds})
	feedLongPath(t, s, "e2e.peer!"+strings.Repeat("b", 1500)+"!"+strings.Repeat("c", 1500))
}
