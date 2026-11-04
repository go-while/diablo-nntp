package e2e

// dreaderd's DNS result cache (readerdnscachettl, readerdnscachenegttl, readerdnscachesize)
// and the AUTHINFO re-send that carries the connect-time lookup result.
//
// The tests preload testdata/dnsshim/dnsshim.c into dreaderd. It fakes the reverse and
// forward lookups of 127.a.b.c (h-a-b-c.bench.test) and ::1 (v6-1.bench.test), and logs
// every faked lookup, so a test can count them per address. Special nets: 127.9 no name,
// 127.8 reverse timeout, 127.5 reverse timeout once, 127.7 forward mismatch, 127.6 forward
// timeout (see the shim).

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// dnsCacheAccess: names under bench.test read and post (200), 127.9.1.0/24 reads only
// (201), anything else from 127.0.0.0/8 is refused. 127.0.0.1 (the harness probe) reads.
const dnsCacheAccess = `vserverdef DEFAULT
    hostname ` + PathHost + `
end
readerdef rp
    read yes
    post yes
end
readerdef ro
    read yes
end
access 127.0.0.1 ro
access 127.9.1.0/24 ro
access *.bench.test rp
`

type dnsShim struct {
	so, log string
}

// buildDNSShim compiles the resolver shim, or skips (it needs glibc's LD_PRELOAD).
func buildDNSShim(t *testing.T) *dnsShim {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("the resolver shim needs Linux (LD_PRELOAD of getnameinfo/getaddrinfo)")
	}
	cc := wmCC(t)
	dir := t.TempDir()
	so := filepath.Join(dir, "dnsshim.so")
	src, err := filepath.Abs(filepath.Join("testdata", "dnsshim", "dnsshim.c"))
	if err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(cc, "-O2", "-shared", "-fPIC", "-o", so, src, "-ldl").CombinedOutput(); err != nil {
		t.Skipf("cannot build the resolver shim: %v\n%s", err, out)
	}
	return &dnsShim{so: so, log: filepath.Join(dir, "lookups.log")}
}

// startDNSCacheReader writes a reader site with the shim preloaded and starts dreaderd on
// bind ("" = [127.0.0.1]:port).
func startDNSCacheReader(t *testing.T, sh *dnsShim, access string, conf map[string]string, bind string) *Site {
	t.Helper()
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: access, Config: conf, NoStart: true})
	s.Env = append(s.Env, "LD_PRELOAD="+sh.so, "DSHIM_COUNT="+sh.log)
	_, port, _ := net.SplitHostPort(s.Addr)
	if bind == "" {
		bind = "127.0.0.1"
	}
	s.Addr = net.JoinHostPort(bind, port)
	bind = "[" + bind + "]:" + port
	startReader(t, s, bind)
	return s
}

// lookups returns how many faked lookups the shim logged for ip so far (reverse lookups of
// ip plus forward lookups of its name).
func (sh *dnsShim) lookups(ip string) int {
	b, _ := os.ReadFile(sh.log)
	tail := "v6-1.bench.test"
	if f := strings.Split(ip, "."); len(f) == 4 {
		tail = "-" + f[1] + "-" + f[2] + "-" + f[3] + ".bench.test" // h-, x- or t- names
	}
	n := 0
	for _, l := range strings.Split(string(b), "\n") {
		if l == "rev "+ip {
			n++
		} else if fwd, ok := strings.CutPrefix(l, "fwd "); ok &&
			(fwd == tail || len(fwd) == len(tail)+1 && strings.HasSuffix(fwd, tail)) {
			n++
		}
	}
	return n
}

// dnsLogin connects from ip, reads the greeting, sends QUIT and returns the greeting code
// and the number of lookups the login did.
func dnsLogin(t *testing.T, s *Site, sh *dnsShim, ip string) (int, int) {
	t.Helper()
	before := sh.lookups(ip)
	c := s.dialFrom(t, ip)
	code, _ := strconv.Atoi(c.Greeting[:3])
	c.TryCmd("QUIT") // a refused login is already closed
	c.Close()
	return code, sh.lookups(ip) - before
}

func wantLogin(t *testing.T, s *Site, sh *dnsShim, ip string, wantCode, wantLookups int, what string) {
	t.Helper()
	code, n := dnsLogin(t, s, sh, ip)
	if code != wantCode || n != wantLookups {
		t.Errorf("%s: login from %s got greeting %d with %d lookups, want %d with %d",
			what, ip, code, n, wantCode, wantLookups)
	}
}

// dnsCacheStat returns one counter of "drcmd dnscache".
func dnsCacheStat(t *testing.T, s *Site, key string) int {
	t.Helper()
	out := s.Run(t, "drcmd", "dnscache")
	for _, f := range strings.Fields(out) {
		if v, ok := strings.CutPrefix(f, key+"="); ok {
			n, _ := strconv.Atoi(v)
			return n
		}
	}
	t.Fatalf("drcmd dnscache: no %s in %q", key, out)
	return 0
}

// TestReaderDNSCacheRepeatLogin: a second login from the same address within the TTL does
// no lookups, and the cached name still matches the name-based access rule. Fails on main:
// every login did the reverse and the forward lookup again.
func TestReaderDNSCacheRepeatLogin(t *testing.T) {
	sh := buildDNSShim(t)
	s := startDNSCacheReader(t, sh, dnsCacheAccess, nil, "")
	wantLogin(t, s, sh, "127.1.0.1", 200, 2, "first login")
	wantLogin(t, s, sh, "127.1.0.1", 200, 0, "second login")
	wantLogin(t, s, sh, "127.1.0.2", 200, 2, "other address")
}

// TestReaderDNSAuthinfoNoRelookup: AUTHINFO USER/PASS sends the connection back to a DNS
// worker for the auth check; it carries the connect-time lookup result, so a login with
// AUTHINFO does 2 lookups, not 4, with the cache on and off. Fails on main (4 lookups).
func TestReaderDNSAuthinfoNoRelookup(t *testing.T) {
	access := `vserverdef DEFAULT
    hostname ` + PathHost + `
end
authdef fred
    user fred
    pass fredspass
end
readerdef userfred
    read yes
    post yes
    auth fred
end
readerdef ro
    read yes
end
access 127.0.0.1 ro
access *.bench.test userfred
`
	for _, tc := range []struct {
		name string
		conf map[string]string
	}{
		{"cache on", nil},
		{"cache off", map[string]string{"readerdnscachettl": "0", "readerdnscachenegttl": "0"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sh := buildDNSShim(t)
			s := startDNSCacheReader(t, sh, access, tc.conf, "")
			ip := "127.1.0.3"
			c := s.dialFrom(t, ip)
			c.ExpectCmd(381, "AUTHINFO USER fred")
			c.ExpectCmd(281, "AUTHINFO PASS fredspass")
			c.ExpectCmd(211, "GROUP e2e.test")
			c.Cmd("QUIT")
			c.Close()
			if n := sh.lookups(ip); n != 2 {
				t.Errorf("login with AUTHINFO did %d lookups, want 2 (one reverse, one forward)", n)
			}
		})
	}
}

// TestReaderDNSCacheTTL: positive and negative entries expire after their own TTL. A failed
// reverse lookup is cached too, and the login falls back to the address rules, as without
// the cache.
func TestReaderDNSCacheTTL(t *testing.T) {
	sh := buildDNSShim(t)
	s := startDNSCacheReader(t, sh, dnsCacheAccess, map[string]string{
		"readerdnscachettl": "10", "readerdnscachenegttl": "3"}, "")
	const pos, neg, refused = "127.1.0.4", "127.9.1.1", "127.9.2.1"
	// Only some libcs tell "no name" from a timeout (glibc, illumos); elsewhere a missing
	// reverse name is not cached and costs its lookup on every login.
	negLookups := 1
	if dnsCacheStat(t, s, "noname") == 1 {
		negLookups = 0
	}
	start := time.Now()
	wantLogin(t, s, sh, pos, 200, 2, "name, first login")
	wantLogin(t, s, sh, neg, 201, 1, "no name, first login (address rule)")
	wantLogin(t, s, sh, refused, 502, 1, "no name, no address rule, first login")
	wantLogin(t, s, sh, pos, 200, 0, "name, cached")
	wantLogin(t, s, sh, neg, 201, negLookups, "no name, cached (address rule)")
	wantLogin(t, s, sh, refused, 502, negLookups, "no name, no address rule, cached")

	time.Sleep(time.Until(start.Add(4500 * time.Millisecond)))
	wantLogin(t, s, sh, neg, 201, 1, "no name after the negative TTL")
	wantLogin(t, s, sh, pos, 200, 0, "name within the positive TTL")

	time.Sleep(time.Until(start.Add(12 * time.Second)))
	wantLogin(t, s, sh, pos, 200, 2, "name after the positive TTL")
}

// TestReaderDNSCacheOff: with both TTLs 0 every login does its lookups, as on main.
func TestReaderDNSCacheOff(t *testing.T) {
	sh := buildDNSShim(t)
	s := startDNSCacheReader(t, sh, dnsCacheAccess, map[string]string{
		"readerdnscachettl": "0", "readerdnscachenegttl": "0"}, "")
	for i := 0; i < 2; i++ {
		wantLogin(t, s, sh, "127.1.0.5", 200, 2, "name")
		wantLogin(t, s, sh, "127.9.1.5", 201, 1, "no name")
	}
	if n := dnsCacheStat(t, s, "stores"); n != 0 {
		t.Errorf("cache off: %d entries stored, want 0", n)
	}
}

// TestReaderDNSCacheAccessReevaluated: the cache holds lookup results only. A changed
// dreader.access applies to the next login from a cached address.
func TestReaderDNSCacheAccessReevaluated(t *testing.T) {
	sh := buildDNSShim(t)
	s := startDNSCacheReader(t, sh, dnsCacheAccess, nil, "")
	ip := "127.1.0.6"
	wantLogin(t, s, sh, ip, 200, 2, "before the change")
	// the workers notice a new access cache only in a later second
	time.Sleep(1100 * time.Millisecond)
	p := filepath.Join(s.Dir, "dreader.access")
	s.WriteFile(t, "dreader.access", strings.Replace(dnsCacheAccess,
		"access *.bench.test rp\n", "access *.bench.test ro\naccess h-1-0-7.bench.test rp\n", 1))
	future := time.Now().Add(2 * time.Second)
	os.Chtimes(p, future, future)
	wantLogin(t, s, sh, ip, 201, 0, "after the change (read only now)")
	wantLogin(t, s, sh, "127.1.0.7", 200, 2, "new rule for another name")
}

// TestReaderDNSCacheIPv6: IPv6 clients are cached too.
func TestReaderDNSCacheIPv6(t *testing.T) {
	if l, err := net.Listen("tcp6", "[::1]:0"); err != nil {
		t.Skipf("no IPv6 loopback: %v", err)
	} else {
		l.Close()
	}
	// dreaderd resolves its -P address with AI_ADDRCONFIG, which refuses ::1 ("Address
	// family for hostname not supported") unless a non-loopback interface has an IPv6
	// address, as in containers without IPv6.
	if !hasNonLoopbackIPv6() {
		t.Skip("no IPv6 address outside loopback (getaddrinfo AI_ADDRCONFIG refuses ::1)")
	}
	sh := buildDNSShim(t)
	access := strings.Replace(dnsCacheAccess, "access 127.0.0.1 ro\n", "", 1)
	s := startDNSCacheReader(t, sh, access, nil, "::1")
	// The harness probe also connected from ::1: wait for it with one login, then empty
	// the cache.
	dnsLogin(t, s, sh, "::1")
	s.Run(t, "drcmd", "dnscache", "flush")
	wantLogin(t, s, sh, "::1", 200, 2, "first login")
	wantLogin(t, s, sh, "::1", 200, 0, "second login")
}

// TestReaderDNSCacheBound: the cache never holds more than readerdnscachesize entries; a
// full set drops an entry and the newest entry is still served.
func TestReaderDNSCacheBound(t *testing.T) {
	sh := buildDNSShim(t)
	s := startDNSCacheReader(t, sh, dnsCacheAccess, map[string]string{"readerdnscachesize": "16"}, "")
	const n = 64
	ip := func(i int) string { return "127.1.1." + strconv.Itoa(i) }
	stores0 := dnsCacheStat(t, s, "stores") // the harness probe from 127.0.0.1
	for i := 1; i <= n; i++ {
		wantLogin(t, s, sh, ip(i), 200, 2, "distinct address")
	}
	size, used := dnsCacheStat(t, s, "size"), dnsCacheStat(t, s, "used")
	stores, evict := dnsCacheStat(t, s, "stores")-stores0, dnsCacheStat(t, s, "evictions")
	t.Logf("size=%d used=%d stores=%d evictions=%d", size, used, stores, evict)
	if size != 16 || used > size {
		t.Errorf("size=%d used=%d, want size 16 and used <= 16", size, used)
	}
	if stores != n || evict != stores+stores0-used {
		t.Errorf("stores=%d evictions=%d used=%d, want %d stores and evictions = all stores - used",
			stores, evict, used, n)
	}
	wantLogin(t, s, sh, ip(n), 200, 0, "newest entry")
	// at most 16 of the 64 addresses can be cached
	hits := 0
	for i := 1; i <= n; i++ {
		if _, k := dnsLogin(t, s, sh, ip(i)); k == 0 {
			hits++
		}
	}
	if hits > 16 {
		t.Errorf("%d of %d addresses were served from a 16-entry cache", hits, n)
	}
}

// TestReaderDNSProxiedAddress: a useproxied client address replaces the connecting one;
// the re-send must look up the new address, not reuse the result of the proxy's.
func TestReaderDNSProxiedAddress(t *testing.T) {
	sh := buildDNSShim(t)
	access := dnsCacheAccess + `readerdef viaproxy
    read yes
    useproxied yes
end
access h-1-0-8.bench.test viaproxy
`
	s := startDNSCacheReader(t, sh, access, nil, "")
	c := s.dialFrom(t, "127.1.0.8")
	code, line := c.Cmd("proxied 127.1.0.9:1234")
	t.Logf("proxied: %d %s", code, line)
	c.Cmd("QUIT")
	c.Close()
	if n := sh.lookups("127.1.0.9"); n != 2 {
		t.Errorf("proxied address 127.1.0.9: %d lookups, want 2", n)
	}
	if code != 200 {
		t.Errorf("proxied as 127.1.0.9 (h-1-0-9, rule rp): reply %d %q, want 200", code, line)
	}
}

// authAccess: names under bench.test read and post after AUTHINFO; 127.5.0.0/16 reads only,
// also after AUTHINFO.
const dnsAuthAccess = `vserverdef DEFAULT
    hostname ` + PathHost + `
end
authdef fred
    user fred
    pass fredspass
end
readerdef userfred
    read yes
    post yes
    auth fred
end
readerdef ipauth
    read yes
    auth fred
end
readerdef ro
    read yes
end
access 127.0.0.1 ro
access 127.5.0.0/16 ipauth
access *.bench.test userfred
`

// TestReaderDNSAuthinfoAfterTempFail: when the connect-time reverse lookup failed
// temporarily, AUTHINFO looks up again (as on main) and gets the name-based rule, instead of
// reusing the failure for the rest of the connection. Failed before the fix: POST got 440
// (the address rule) with 1 lookup.
func TestReaderDNSAuthinfoAfterTempFail(t *testing.T) {
	sh := buildDNSShim(t)
	s := startDNSCacheReader(t, sh, dnsAuthAccess, nil, "")
	ip := "127.5.1.1" // the first reverse lookup times out
	c := s.dialFrom(t, ip)
	if !strings.HasPrefix(c.Greeting, "201") {
		t.Errorf("greeting %q, want 201 (address rule, no name yet)", c.Greeting)
	}
	c.ExpectCmd(381, "AUTHINFO USER fred")
	c.ExpectCmd(281, "AUTHINFO PASS fredspass")
	code, line := c.Cmd("POST")
	if code == 340 {
		c.MustSend(".\r\n")
		c.ReadStatus()
	}
	c.TryCmd("QUIT")
	c.Close()
	if code != 340 {
		t.Errorf("POST after AUTHINFO: %q, want 340 (name rule after a new lookup)", line)
	}
	if n := sh.lookups(ip); n != 3 {
		t.Errorf("%d lookups, want 3 (failed reverse at connect; reverse and forward at AUTHINFO)", n)
	}
}

// TestReaderDNSCacheTempFailNotCached: a reverse or forward lookup that failed temporarily
// is not cached; a forward mismatch is cached (negative TTL) and gives the same decision.
func TestReaderDNSCacheTempFailNotCached(t *testing.T) {
	sh := buildDNSShim(t)
	access := dnsCacheAccess + "access 127.7.2.0/24 ro\n"
	s := startDNSCacheReader(t, sh, access, nil, "")
	for _, tc := range []struct {
		ip, what           string
		code, first, again int
	}{
		{"127.8.1.1", "reverse timeout", 502, 1, 1},
		{"127.6.1.1", "forward timeout", 502, 2, 2},
		{"127.7.1.1", "forward mismatch, name rule", 502, 2, 0},
		{"127.7.2.1", "forward mismatch, address rule", 201, 2, 0},
	} {
		wantLogin(t, s, sh, tc.ip, tc.code, tc.first, tc.what+", first login")
		wantLogin(t, s, sh, tc.ip, tc.code, tc.again, tc.what+", second login")
	}
}

// TestReaderDNSCacheTTLLowered: an entry is checked against the current TTL, so a TTL
// lowered with drcmd config applies to entries stored before. Failed before: the entry kept
// its old expiry (0 lookups).
func TestReaderDNSCacheTTLLowered(t *testing.T) {
	sh := buildDNSShim(t)
	s := startDNSCacheReader(t, sh, dnsCacheAccess, nil, "")
	ip := "127.1.4.4"
	wantLogin(t, s, sh, ip, 200, 2, "first login (ttl 300)")
	wantLogin(t, s, sh, ip, 200, 0, "second login (ttl 300)")
	if out := s.Run(t, "drcmd", "config", "readerdnscachettl", "1"); !strings.Contains(out, "Set") {
		t.Fatalf("drcmd config readerdnscachettl 1: %q", out)
	}
	time.Sleep(2500 * time.Millisecond)
	wantLogin(t, s, sh, ip, 200, 2, "after lowering the TTL to 1 s")
}

// TestReaderDNSCacheFlush: "drcmd dnscache flush" empties the cache; other words do not.
func TestReaderDNSCacheFlush(t *testing.T) {
	sh := buildDNSShim(t)
	s := startDNSCacheReader(t, sh, dnsCacheAccess, nil, "")
	ip := "127.1.4.5"
	wantLogin(t, s, sh, ip, 200, 2, "first login")
	s.Run(t, "drcmd", "dnscache", "flushfoo")
	wantLogin(t, s, sh, ip, 200, 0, "after 'dnscache flushfoo'")
	s.Run(t, "drcmd", "dnscache", "flush")
	if n := dnsCacheStat(t, s, "size"); n != 0 {
		t.Errorf("after a flush the table is freed: size=%d, want 0", n)
	}
	wantLogin(t, s, sh, ip, 200, 2, "after 'dnscache flush'")
	wantLogin(t, s, sh, ip, 200, 0, "cached again")
}

// hasNonLoopbackIPv6 reports whether an interface other than loopback has an IPv6 address.
func hasNonLoopbackIPv6() bool {
	ifs, err := net.Interfaces()
	if err != nil {
		return false
	}
	for _, ifc := range ifs {
		if ifc.Flags&net.FlagLoopback != 0 || ifc.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok && n.IP.To4() == nil && !n.IP.IsLoopback() {
				return true
			}
		}
	}
	return false
}
