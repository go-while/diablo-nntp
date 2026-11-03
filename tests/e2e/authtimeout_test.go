package e2e

// readerauthtimeout: an auth backend that does not answer in time refuses the login, and the
// DNS worker (readerdns 1 here) serves the next login. Before, a backend that hung held the
// worker for good (LDAP, Perl, PAM, the NetRemote read) or for minutes (the NetRemote
// connect), and logins stopped once every worker was held.
//
// The slow backend sits on its own access line behind "realm slow", so only "x@slow" logins
// reach it; pat logs in with user and pass on the line before. Each test needs dreaderd built
// with the backend and skips otherwise.

import (
	"bytes"
	"net"
	"os"
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
// extra environment env (e.g. an LD_PRELOAD shim).
func authTimeoutSite(t *testing.T, access string, env ...string) *Site {
	t.Helper()
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: access, NoStart: true,
		Config: map[string]string{"readerauthtimeout": strconv.Itoa(authTimeout)}})
	s.Env = append(s.Env, env...)
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

// goodLogin: pat logs in, on the same (only) DNS worker: it did not die and get replaced.
func goodLogin(t *testing.T, s *Site) {
	t.Helper()
	c := client.DialWith(t, s.Addr, 2*authTimeoutBound)
	defer c.Close()
	expect(t, c, 381, "AUTHINFO USER pat")
	expect(t, c, 281, "AUTHINFO PASS patspass")
	if strings.Contains(s.Proc.Stderr(), "Lost dns resolver") {
		t.Errorf("the DNS worker died")
	}
}

// wantTimeoutLog: dreaderd logged the timeout of backend what.
func wantTimeoutLog(t *testing.T, s *Site, what string) {
	t.Helper()
	want := what + " authentication from 127.0.0.1 timed out after " + strconv.Itoa(authTimeout) + " seconds"
	deadline := time.Now().Add(3 * time.Second)
	for !strings.Contains(s.Proc.Stderr(), want) {
		if time.Now().After(deadline) {
			t.Errorf("no %q in the log", want)
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
	wantTimeoutLog(t, s, "NetRemote")
	goodLogin(t, s)
	slowLogin(t, s, "y@slow", "pw")
	goodLogin(t, s)
}

// TestAuthTimeoutNetRemoteNoAccept: a NetRemote server whose connects hang (about 127 s per
// address before, the kernel's SYN retries).
func TestAuthTimeoutNetRemoteNoAccept(t *testing.T) {
	builtWith(t, "NetRemote socket connect", "NetRemote support (NETREMOTE_ENABLED in lib/vendor.h)")
	port := deafServer(t)
	s := authTimeoutSite(t, slowAuthAccess("netremote secret@127.0.0.1:"+strconv.Itoa(port)))
	slowLogin(t, s, "x@slow", "pw")
	wantTimeoutLog(t, s, "NetRemote")
	goodLogin(t, s)
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
	wantTimeoutLog(t, s, "LDAP")
	goodLogin(t, s)
	slowLogin(t, s, "y@slow", "pw")
	goodLogin(t, s)
}

// TestAuthTimeoutLDAPNoAccept: an LDAP server whose connects hang.
func TestAuthTimeoutLDAPNoAccept(t *testing.T) {
	builtWith(t, "LDAP bind failed", "LDAP support (LDAP_ENABLED)")
	port := deafServer(t)
	s := authTimeoutSite(t, slowAuthAccess(ldapURL(port)))
	slowLogin(t, s, "x@slow", "pw")
	goodLogin(t, s)
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

// TestAuthTimeoutPerlSleep: a checkuser that sleeps dies at the limit, and the script keeps
// serving.
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
	wantTimeoutLog(t, s, "Perl")
	goodLogin(t, s)
	// the same interpreter still answers
	c := client.DialWith(t, s.Addr, 2*authTimeoutBound)
	defer c.Close()
	expect(t, c, 381, "AUTHINFO USER ok@slow")
	expect(t, c, 281, "AUTHINFO PASS pw")
}

// TestAuthTimeoutPerlBackstop: a checkuser that catches the timeout die and goes on sleeping
// is cut off by the worker's alarm; the next login loads the script anew.
func TestAuthTimeoutPerlBackstop(t *testing.T) {
	s := perlSite(t, `sub checkuser {
    my ($user, $pass) = @_;
    if ($user eq "x") {
        eval { sleep 60 };
        sleep 60;
    }
    return "100 Success" if $user eq "ok";
    return "0";
}
1;
`)
	slowLogin(t, s, "x@slow", "pw")
	wantTimeoutLog(t, s, "Perl")
	goodLogin(t, s)
	c := client.DialWith(t, s.Addr, 2*authTimeoutBound)
	defer c.Close()
	expect(t, c, 381, "AUTHINFO USER ok@slow")
	expect(t, c, 281, "AUTHINFO PASS pw")
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
// library) is cut off by the worker's alarm.
func TestAuthTimeoutPAM(t *testing.T) {
	builtWith(t, "Unable to release PAM authentication", "PAM support (PAM_ENABLED)")
	so := preloadShim(t, "pamshim", pamShimSource)
	s := authTimeoutSite(t, slowAuthAccess("pam diablo"), lockFaultEnv(so))
	slowLogin(t, s, "x@slow", "pw")
	wantTimeoutLog(t, s, "PAM")
	goodLogin(t, s)
	c := client.DialWith(t, s.Addr, 2*authTimeoutBound)
	defer c.Close()
	expect(t, c, 381, "AUTHINFO USER ok@slow")
	expect(t, c, 281, "AUTHINFO PASS pw")
	slowLogin(t, s, "x@slow", "pw")
	goodLogin(t, s)
}
