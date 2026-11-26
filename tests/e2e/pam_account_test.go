package e2e

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-while/nntp-testkit/client"
)

// pamAccountSource replaces the PAM library calls of dreaderd (adapted from go-while's
// TestR2PAMAccountPhase on gw-r2-repro-c): every password is right, and the account phase
// (pam_acct_mgmt) refuses user "expired" with PAM_ACCT_EXPIRED, as shadow, SSSD/LDAP,
// pam_access or pam_time do for an expired, disabled or denied account, and user "newtok"
// with PAM_NEW_AUTHTOK_REQD (the password has expired). Everyone else passes. It marks
// each call and the PAM_RHOST that dreaderd sets. pam_strerror is the real library's.
const pamAccountSource = `#define _GNU_SOURCE
#define __EXTENSIONS__
#include <fcntl.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <security/pam_appl.h>
/*MARK*/
static char shimUser[256];

int
pam_start(const char *service, const char *user, const struct pam_conv *conv, pam_handle_t **pamh)
{
    (void)service; (void)conv;
    snprintf(shimUser, sizeof(shimUser), "%s", user ? user : "");
    *pamh = (pam_handle_t *)shimUser;
    shim_mark("start %s", shimUser);
    return PAM_SUCCESS;
}

int
pam_set_item(pam_handle_t *pamh, int item, const void *value)
{
    (void)pamh;
    if (item == PAM_RHOST)
	shim_mark("rhost %s %s", shimUser, value ? (const char *)value : "(null)");
    return PAM_SUCCESS;
}

int
pam_authenticate(pam_handle_t *pamh, int flags)
{
    (void)pamh; (void)flags;
    shim_mark("authenticate %s", shimUser);
    return PAM_SUCCESS;
}

int
pam_acct_mgmt(pam_handle_t *pamh, int flags)
{
    (void)pamh; (void)flags;
    shim_mark("acct_mgmt %s", shimUser);
    if (strcmp(shimUser, "expired") == 0)
	return PAM_ACCT_EXPIRED;
    if (strcmp(shimUser, "newtok") == 0)
	return PAM_NEW_AUTHTOK_REQD;
    return PAM_SUCCESS;
}

int
pam_end(pam_handle_t *pamh, int status)
{
    (void)pamh; (void)status;
    return PAM_SUCCESS;
}
`

// TestPAMAccountPhase (D-237): a PAM login needs the account phase too. PamAuthenticate()
// (dreaderd/dns.c) called pam_start, pam_authenticate and pam_end, never pam_acct_mgmt, and
// set no PAM_RHOST, so an account that PAM's account phase refuses logged in with its
// password. A preloaded PAM library accepts every password: "ok" logs in (281), and PAM
// was given the client's address as PAM_RHOST; "expired" (PAM_ACCT_EXPIRED) and "newtok"
// (PAM_NEW_AUTHTOK_REQD: an NNTP client cannot change its password) get 481, and the
// refusal is logged.
func TestPAMAccountPhase(t *testing.T) {
	builtWith(t, "Unable to release PAM authentication", "PAM support (PAM_ENABLED)")
	so := preloadShim(t, "pamacct", withMark(pamAccountSource))
	mark := filepath.Join(t.TempDir(), "mark")
	s := authTimeoutSite(t, slowAuthAccess("pam diablo"), lockFaultEnv(so), "SHIM_MARK="+mark)
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("marks:\n%s", strings.Join(shimMarks(mark), "\n"))
		}
	})
	login := func(user string) (int, string) {
		t.Helper()
		c := client.DialWith(t, s.Addr, 2*authTimeoutBound)
		defer c.Close()
		c.ExpectCmd(381, "AUTHINFO USER %s@slow", user)
		return c.Cmd("AUTHINFO PASS pampass")
	}

	code, line := login("ok")
	if len(shimMarked(mark, "authenticate ok")) == 0 {
		t.Fatalf("the login did not go through the injected PAM library (%q)", line)
	}
	if code != 281 {
		t.Errorf("ok, which the account phase accepts: %q, want 281", line)
	}
	if len(shimMarked(mark, "acct_mgmt ok")) == 0 {
		t.Errorf("pam_acct_mgmt was not called for ok")
	}
	if len(shimMarked(mark, "rhost ok 127.0.0.1")) == 0 {
		t.Errorf("PAM_RHOST for ok is not the client's address 127.0.0.1 (rhost marks: %q)",
			shimMarked(mark, "rhost"))
	}

	for _, user := range []string{"expired", "newtok"} {
		code, line := login(user)
		if code != 481 {
			t.Errorf("%s, whose account the account phase refuses: %q, want 481 (pam_acct_mgmt called: %v)",
				user, line, len(shimMarked(mark, "acct_mgmt "+user)) > 0)
		}
		want := "PAM account check for " + user + " from 127.0.0.1: "
		deadline := time.Now().Add(5 * time.Second)
		for !strings.Contains(generalLog(s), want) && time.Now().Before(deadline) {
			time.Sleep(50 * time.Millisecond)
		}
		if !strings.Contains(generalLog(s), want) {
			t.Errorf("no refusal of %s in general.log (want %q)", user, want)
		}
	}
}
