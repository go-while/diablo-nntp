package e2e

// readerauthtimeout: an auth backend that does not answer in time refuses the login, and the
// next login is served (readerdns 1 here). Before, a backend that hung held the DNS worker
// for good (LDAP, Perl, PAM, the NetRemote read) or for minutes (the NetRemote connect), and
// logins stopped once every worker was held.
//
// LDAP and NetRemote run into their own timeouts and the worker goes on. Perl and PAM are cut
// off by the worker's alarm: the worker sends the refusal, exits, and main starts a new one.
//
// The slow backend sits on its own access line behind "realm slow", so only "x@slow" logins
// reach it; pat logs in with user and pass on the line before. Each test needs dreaderd built
// with the backend and skips otherwise.

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/go-while/nntp-testkit/client"
)

// authTimeout is readerauthtimeout in these tests. A refused login must come within
// authTimeout plus the 2 s grace for backends with their own timeout, plus slack.
const authTimeout = 2

const authTimeoutBound = (authTimeout + 2 + 3) * time.Second

// builtWith skips the test unless dreaderd contains marker (a log text of the backend).
func builtWith(t *testing.T, marker, what string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(bin(t), "dreaderd"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b, []byte(marker)) {
		t.Skip("dreaderd is built without " + what)
	}
}

// slowAuthAccess is a dreader.access where "x@slow" logins go to the backend line backend
// (e.g. "ldap ldap://...") and pat/patspass logs in on its own line.
func slowAuthAccess(backend string) string {
	return `vserverdef DEFAULT
    hostname ` + PathHost + `
end
authdef good
    user pat
    pass patspass
end
authdef slow
    realm slow
    ` + backend + `
end
readerdef rgood
    read yes
    auth good
end
readerdef rslow
    read yes
    auth slow
end
access 127.0.0.1 rgood
access 127.0.0.1 rslow
`
}

// authTimeoutSite starts a reader with dreader.access access, readerauthtimeout and the
// extra environment env (e.g. an LD_PRELOAD shim). It logs to log/general.log (stderr does
// not get the syslog lines everywhere: illumos has no LOG_PERROR).
func authTimeoutSite(t *testing.T, access string, env ...string) *Site {
	t.Helper()
	return authTimeoutSiteN(t, access, authTimeout, env...)
}

// authTimeoutSiteN is authTimeoutSite with readerauthtimeout secs.
func authTimeoutSiteN(t *testing.T, access string, secs int, env ...string) *Site {
	t.Helper()
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: access, NoStart: true,
		Config: map[string]string{"readerauthtimeout": strconv.Itoa(secs),
			"path_generallog": "%s/general.log"}})
	s.Env = append(s.Env, env...)
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("general.log:\n%s", generalLog(s))
		}
	})
	_, port, err := net.SplitHostPort(s.Addr)
	if err != nil {
		t.Fatal(err)
	}
	startReader(t, s, "[127.0.0.1]:"+port)
	return s
}

// slowLogin logs in as user/pass and wants a refusal (not 281) within authTimeoutBound. The
// client gives up after twice that, so a login that hangs fails the test instead of the run.
func slowLogin(t *testing.T, s *Site, user, pass string) {
	t.Helper()
	c := client.DialWith(t, s.Addr, 2*authTimeoutBound)
	defer c.Close()
	expect(t, c, 381, "AUTHINFO USER %s", user)
	t0 := time.Now()
	code, line, err := c.TryCmd("AUTHINFO PASS %s", pass)
	took := time.Since(t0)
	switch {
	case err != nil && took >= authTimeoutBound:
		t.Fatalf("AUTHINFO PASS for %s: no answer after %v (%v)", user, took, err)
	case code == 281:
		t.Fatalf("AUTHINFO PASS for %s: logged in: %s", user, line)
	case took >= authTimeoutBound:
		t.Errorf("AUTHINFO PASS for %s: refused after %v, want within %v", user, took, authTimeoutBound)
	}
	t.Logf("%s refused after %v: %d %s %v", user, took.Round(time.Millisecond), code, line, err)
}

// goodLogin: pat logs in.
func goodLogin(t *testing.T, s *Site) {
	t.Helper()
	c := client.DialWith(t, s.Addr, 2*authTimeoutBound)
	defer c.Close()
	expect(t, c, 381, "AUTHINFO USER pat")
	expect(t, c, 281, "AUTHINFO PASS patspass")
}

// wantTimeoutLog: dreaderd logged n timeouts of backend what (n > 0), and replaced the DNS
// worker for each when replaced is true (the alarm cut the backend off), else never. A
// worker is never lost otherwise.
func wantTimeoutLog(t *testing.T, s *Site, what string, n int, replaced bool) {
	t.Helper()
	want := what + " authentication from 127.0.0.1 timed out after "
	const repl = "replaced after an auth timeout"
	nrepl := 0
	if replaced {
		nrepl = n
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		l := generalLog(s)
		got, gotRepl := strings.Count(l, want), strings.Count(l, repl)
		if got == n && gotRepl == nrepl {
			if strings.Contains(l, "Lost dns resolver") {
				t.Errorf("a DNS worker was lost:\n%s", l)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Errorf("%d times %q (want %d) and %d times %q (want %d) in general.log:\n%s",
				got, want, n, gotRepl, repl, nrepl, l)
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// silentServer accepts connections and never writes; it reads and drops what it gets.
func silentServer(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				buf := make([]byte, 4096)
				for {
					if _, err := c.Read(buf); err != nil {
						return
					}
				}
			}()
		}
	}()
	return l.Addr().(*net.TCPAddr).Port
}

// deafServer is a listener that never accepts, with its accept queue full, so a connect to
// it gets no answer to its SYN. Skips where that cannot be made (a probe connect succeeds).
func deafServer(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	if rc, err := l.(*net.TCPListener).SyscallConn(); err == nil {
		rc.Control(func(fd uintptr) { syscall.Listen(int(fd), 0) })
	}
	addr := l.Addr().String()
	for i := 0; i < 16; i++ {
		c, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err != nil {
			return l.Addr().(*net.TCPAddr).Port
		}
		t.Cleanup(func() { c.Close() })
	}
	t.Skip("cannot make a listener whose connects hang here")
	return 0
}

// TestAuthTimeoutNetRemoteSilent: a NetRemote server that accepts and never sends its time.
func TestAuthTimeoutNetRemoteSilent(t *testing.T) {
	builtWith(t, "NetRemote socket connect", "NetRemote support (NETREMOTE_ENABLED in lib/vendor.h)")
	port := silentServer(t)
	s := authTimeoutSite(t, slowAuthAccess("netremote secret@127.0.0.1:"+strconv.Itoa(port)))
	slowLogin(t, s, "x@slow", "pw")
	goodLogin(t, s)
	slowLogin(t, s, "y@slow", "pw")
	goodLogin(t, s)
	wantTimeoutLog(t, s, "NetRemote", 2, false)
}

// TestAuthTimeoutNetRemoteNoAccept: a NetRemote server whose connects hang (about 127 s per
// address before, the kernel's SYN retries).
func TestAuthTimeoutNetRemoteNoAccept(t *testing.T) {
	builtWith(t, "NetRemote socket connect", "NetRemote support (NETREMOTE_ENABLED in lib/vendor.h)")
	port := deafServer(t)
	s := authTimeoutSite(t, slowAuthAccess("netremote secret@127.0.0.1:"+strconv.Itoa(port)))
	slowLogin(t, s, "x@slow", "pw")
	goodLogin(t, s)
	wantTimeoutLog(t, s, "NetRemote", 1, false)
}

// ldapURL is an LDAP URL for the authdef of the build (the default form: a filter with %s, escaped).
func ldapURL(port int) string {
	return "ldap ldap://127.0.0.1:" + strconv.Itoa(port) + "/dc=e2e,dc=test?userPassword?sub?(uid=%25s)"
}

// TestAuthTimeoutLDAPSilent: an LDAP server that accepts and never answers the bind.
func TestAuthTimeoutLDAPSilent(t *testing.T) {
	builtWith(t, "LDAP bind failed", "LDAP support (LDAP_ENABLED)")
	port := silentServer(t)
	s := authTimeoutSite(t, slowAuthAccess(ldapURL(port)))
	slowLogin(t, s, "x@slow", "pw")
	goodLogin(t, s)
	slowLogin(t, s, "y@slow", "pw")
	goodLogin(t, s)
	wantTimeoutLog(t, s, "LDAP", 2, false)
}

// TestAuthTimeoutLDAPNoAccept: an LDAP server whose connects hang.
func TestAuthTimeoutLDAPNoAccept(t *testing.T) {
	builtWith(t, "LDAP bind failed", "LDAP support (LDAP_ENABLED)")
	port := deafServer(t)
	s := authTimeoutSite(t, slowAuthAccess(ldapURL(port)))
	slowLogin(t, s, "x@slow", "pw")
	goodLogin(t, s)
	if l := generalLog(s); strings.Contains(l, "Lost dns resolver") || strings.Contains(l, "replaced after") {
		t.Errorf("the DNS worker was replaced:\n%s", l)
	}
}

// ldapTLV is one BER element: tag, length, body.
func ldapTLV(tag byte, body []byte) []byte {
	n := len(body)
	l := []byte{byte(n)}
	if n >= 128 {
		l = []byte{0x82, byte(n >> 8), byte(n)}
	}
	return append(append([]byte{tag}, l...), body...)
}

// ldapReadTLV reads one BER element.
func ldapReadTLV(r *bufio.Reader) (byte, []byte, error) {
	tag, err := r.ReadByte()
	if err != nil {
		return 0, nil, err
	}
	l, err := r.ReadByte()
	if err != nil {
		return 0, nil, err
	}
	n := int(l)
	if l&0x80 != 0 {
		n = 0
		for i := 0; i < int(l&0x7f); i++ {
			b, err := r.ReadByte()
			if err != nil {
				return 0, nil, err
			}
			n = n<<8 | int(b)
		}
	}
	buf := make([]byte, n)
	_, err = io.ReadFull(r, buf)
	return tag, buf, err
}

// slowLDAPServer answers a bind (success), a search (one entry) and a compare (true), each
// after its delay. Enough LDAP for the default LDAP code of dreaderd.
func slowLDAPServer(t *testing.T, bind, search, compare time.Duration) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	result := func(code byte) []byte {
		r := ldapTLV(0x0a, []byte{code})
		r = append(r, ldapTLV(0x04, nil)...)
		return append(r, ldapTLV(0x04, nil)...)
	}
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				r := bufio.NewReader(c)
				for {
					_, msg, err := ldapReadTLV(r)
					if err != nil || len(msg) < 3 || msg[0] != 0x02 || len(msg) < 3+int(msg[1]) {
						return
					}
					id := msg[2 : 2+int(msg[1])]
					idTLV := ldapTLV(0x02, id)
					var out []byte
					switch msg[2+int(msg[1])] {
					case 0x60: // bind
						time.Sleep(bind)
						out = ldapTLV(0x30, append(idTLV, ldapTLV(0x61, result(0))...))
					case 0x63: // search: one entry, then done
						time.Sleep(search)
						vals := ldapTLV(0x31, ldapTLV(0x04, []byte("pw")))
						attr := ldapTLV(0x30, append(ldapTLV(0x04, []byte("userPassword")), vals...))
						entry := ldapTLV(0x64, append(ldapTLV(0x04, []byte("uid=x,dc=e2e,dc=test")), ldapTLV(0x30, attr)...))
						out = ldapTLV(0x30, append(idTLV, entry...))
						out = append(out, ldapTLV(0x30, append(idTLV, ldapTLV(0x65, result(0))...))...)
					case 0x6e: // compare: true
						time.Sleep(compare)
						out = ldapTLV(0x30, append(idTLV, ldapTLV(0x6f, result(6))...))
					default:
						continue
					}
					if _, err := c.Write(out); err != nil {
						return
					}
				}
			}(c)
		}
	}()
	return l.Addr().(*net.TCPAddr).Port
}

// TestAuthTimeoutLDAPSlowSteps: an LDAP server that answers, but slowly. The steps share the
// limit: when the time runs out in the bind, the search or the compare, the login is refused
// at the limit and counted as a timeout, and the worker goes on. A quick server logs in.
func TestAuthTimeoutLDAPSlowSteps(t *testing.T) {
	builtWith(t, "LDAP bind failed", "LDAP support (LDAP_ENABLED)")
	ms := time.Millisecond
	port := slowLDAPServer(t, 0, 0, 0)
	s := authTimeoutSite(t, slowAuthAccess(ldapURL(port)))
	backendLogin(t, s, "x@slow")
	for _, d := range [][3]time.Duration{
		{3000 * ms, 0, 0},
		{800 * ms, 800 * ms, 800 * ms},
		{0, 1500 * ms, 1500 * ms},
		{0, 0, 10000 * ms},
	} {
		t.Run(fmt.Sprintf("%v-%v-%v", d[0], d[1], d[2]), func(t *testing.T) {
			port := slowLDAPServer(t, d[0], d[1], d[2])
			s := authTimeoutSite(t, slowAuthAccess(ldapURL(port)))
			slowLogin(t, s, "x@slow", "pw")
			goodLogin(t, s)
			wantTimeoutLog(t, s, "LDAP", 1, false)
		})
	}
}

// perlSite starts a reader whose x@slow logins run checkuser of script.
func perlSite(t *testing.T, script string) *Site {
	t.Helper()
	builtWith(t, "PERL auth request", "Perl support (PERL_ENABLED)")
	path := filepath.Join(t.TempDir(), "auth.pl")
	if err := os.WriteFile(path, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	return authTimeoutSite(t, slowAuthAccess("perl "+path))
}

// TestAuthTimeoutPerlSleep: a checkuser that sleeps is cut off at the limit; the next worker
// loads the script again and serves the next logins.
func TestAuthTimeoutPerlSleep(t *testing.T) {
	s := perlSite(t, `sub checkuser {
    my ($user, $pass) = @_;
    sleep 60 if $user eq "x";
    return "100 Success" if $user eq "ok";
    return "0";
}
1;
`)
	slowLogin(t, s, "x@slow", "pw")
	goodLogin(t, s)
	c := client.DialWith(t, s.Addr, 2*authTimeoutBound)
	defer c.Close()
	expect(t, c, 381, "AUTHINFO USER ok@slow")
	expect(t, c, 281, "AUTHINFO PASS pw")
	wantTimeoutLog(t, s, "Perl", 1, true)
}

// perlMultiplicity skips the test unless perl can run more than one interpreter (a perl
// without MULTIPLICITY, as on OpenBSD, runs one script per DNS worker only).
func perlMultiplicity(t *testing.T) {
	t.Helper()
	out, err := exec.Command("perl", "-MConfig", "-e", "print $Config{usemultiplicity}").Output()
	if err != nil || strings.TrimSpace(string(out)) != "define" {
		t.Skip("perl is built without MULTIPLICITY: one perl script per worker")
	}
}

// twoPerlAccess: x@slow and ok@slow go to perl script a, x@slow2 and ok@slow2 to b.
func twoPerlAccess(a, b string) string {
	access := strings.Replace(slowAuthAccess("perl "+a), "access 127.0.0.1 rslow\n",
		"access 127.0.0.1 rslow\naccess 127.0.0.1 rslow2\n", 1)
	return strings.Replace(access, "readerdef rslow\n", "authdef slow2\n    realm slow2\n    perl "+b+
		"\nend\nreaderdef rslow2\n    read yes\n    auth slow2\nend\nreaderdef rslow\n", 1)
}

// backendLogin: user logs in through the backend (password pw).
func backendLogin(t *testing.T, s *Site, user string) {
	t.Helper()
	c := client.DialWith(t, s.Addr, 2*authTimeoutBound)
	defer c.Close()
	expect(t, c, 381, "AUTHINFO USER %s", user)
	expect(t, c, 281, "AUTHINFO PASS pw")
}

// TestAuthTimeoutPerlManyTimeouts: two perl authdefs and more timeouts than a worker keeps
// interpreters (16). Each timeout costs the worker, not the scripts: the logins of both
// scripts keep working. Before the worker kept going after a cut-off and abandoned an
// interpreter each time; after 16 every perl login was refused.
func TestAuthTimeoutPerlManyTimeouts(t *testing.T) {
	builtWith(t, "PERL auth request", "Perl support (PERL_ENABLED)")
	perlMultiplicity(t)
	dir := t.TempDir()
	sc := "sub checkuser { sleep 60 if $_[0] eq 'x'; return \"100 Success\" if $_[0] eq 'ok'; return '0'; }\n1;\n"
	a, b := filepath.Join(dir, "a.pl"), filepath.Join(dir, "b.pl")
	for _, f := range []string{a, b} {
		if err := os.WriteFile(f, []byte(sc), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s := authTimeoutSiteN(t, twoPerlAccess(a, b), 1)
	const n = 17
	for i := 0; i < n; i++ {
		backendLogin(t, s, "ok@slow")
		backendLogin(t, s, "ok@slow2")
		if i%2 == 0 {
			slowLogin(t, s, "x@slow", "pw")
		} else {
			slowLogin(t, s, "x@slow2", "pw")
		}
	}
	backendLogin(t, s, "ok@slow")
	backendLogin(t, s, "ok@slow2")
	goodLogin(t, s)
	l := generalLog(s)
	if got := strings.Count(l, "Perl authentication from 127.0.0.1 timed out after 1 seconds"); got != n {
		t.Errorf("%d Perl timeouts logged, want %d", got, n)
	}
	if got := strings.Count(l, "replaced after an auth timeout"); got != n {
		t.Errorf("%d workers replaced, want %d", got, n)
	}
	if strings.Contains(l, "Lost dns resolver") || strings.Contains(l, "more than 16") {
		t.Errorf("general.log:\n%s", l)
	}
}

// TestAuthTimeoutPerlSlowLoad: a script whose loading hangs (here a BEGIN block, the first
// time only) is cut off; the next login loads it again in the new worker.
func TestAuthTimeoutPerlSlowLoad(t *testing.T) {
	builtWith(t, "PERL auth request", "Perl support (PERL_ENABLED)")
	perlMultiplicity(t)
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.pl"), filepath.Join(dir, "b.pl")
	mark := filepath.Join(dir, "loaded")
	if err := os.WriteFile(a, []byte("sub checkuser { return \"100 Success\" if $_[0] eq 'ok'; return '0'; }\n1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("BEGIN { unless (-e '"+mark+"') { open(F, '>"+mark+"'); close F; sleep 60 } }\n"+
		"sub checkuser { return \"100 Success\" if $_[0] eq 'ok'; return '0'; }\n1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := authTimeoutSite(t, twoPerlAccess(a, b))
	backendLogin(t, s, "ok@slow")
	slowLogin(t, s, "ok@slow2", "pw")
	backendLogin(t, s, "ok@slow2")
	backendLogin(t, s, "ok@slow")
	wantTimeoutLog(t, s, "Perl", 1, true)
}

// pamShimSource replaces the PAM calls of dreaderd: pam_authenticate never returns for user
// "x", accepts "ok" and refuses everyone else.
const pamShimSource = `#include <string.h>
#include <unistd.h>

static char shimUser[256];

int
pam_start(const char *service, const char *user, const void *conv, void **pamh)
{
    (void)service; (void)conv;
    strncpy(shimUser, user ? user : "", sizeof(shimUser) - 1);
    *pamh = (void *)shimUser;
    return 0;
}

int
pam_authenticate(void *pamh, int flags)
{
    (void)pamh; (void)flags;
    if (strcmp(shimUser, "x") == 0)
	for (;;)
	    pause();
    return strcmp(shimUser, "ok") == 0 ? 0 : 7;
}

int
pam_end(void *pamh, int status)
{
    (void)pamh; (void)status;
    return 0;
}
`

// TestAuthTimeoutPAM: a PAM module that never returns (here a preloaded stand-in for the PAM
// library) is cut off by the worker's alarm; main replaces the worker, and the new one serves
// the next logins.
func TestAuthTimeoutPAM(t *testing.T) {
	builtWith(t, "Unable to release PAM authentication", "PAM support (PAM_ENABLED)")
	so := preloadShim(t, "pamshim", pamShimSource)
	s := authTimeoutSite(t, slowAuthAccess("pam diablo"), lockFaultEnv(so))
	slowLogin(t, s, "x@slow", "pw")
	// served by the new worker
	goodLogin(t, s)
	backendLogin(t, s, "ok@slow")
	slowLogin(t, s, "x@slow", "pw")
	goodLogin(t, s)
	wantTimeoutLog(t, s, "PAM", 2, true)
	// a new worker is a fork of main: dreader.access is not parsed again
	if n := strings.Count(generalLog(s), "Installing access cache"); n != 1 {
		t.Errorf("dreader.access parsed %d times, want 1", n)
	}
}
