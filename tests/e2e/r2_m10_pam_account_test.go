package e2e

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-while/nntp-testkit/client"
)

// pamAcctSource replaces the PAM library calls of dreaderd: every password is right, and the
// account phase (pam_acct_mgmt) refuses the account as expired, as shadow, SSSD/LDAP,
// pam_access or pam_time do for an expired, disabled or denied account. It marks each call.
const pamAcctSource = `#define _GNU_SOURCE
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
pam_authenticate(pam_handle_t *pamh, int flags)
{
    (void)pamh; (void)flags;
    shim_mark("authenticate");
    return PAM_SUCCESS;
}

int
pam_acct_mgmt(pam_handle_t *pamh, int flags)
{
    (void)pamh; (void)flags;
    shim_mark("acct_mgmt: expired");
    return PAM_ACCT_EXPIRED;
}

int
pam_end(pam_handle_t *pamh, int status)
{
    (void)pamh; (void)status;
    return PAM_SUCCESS;
}
`

// TestR2PAMAccountPhase (deep review r2 M10): PamAuthenticate() (dreaderd/dns.c) calls
// pam_start, pam_authenticate and pam_end, never pam_acct_mgmt, so an account that PAM's
// account phase refuses (expired, disabled, denied by pam_access/pam_time) logs in. The
// preloaded PAM library accepts the password and refuses the account: AUTHINFO must not
// give 281.
func TestR2PAMAccountPhase(t *testing.T) {
	builtWith(t, "Unable to release PAM authentication", "PAM support (PAM_ENABLED)")
	so := preloadShim(t, "pamacct", withMark(pamAcctSource))
	mark := filepath.Join(t.TempDir(), "mark")
	s := authTimeoutSite(t, slowAuthAccess("pam diablo"), lockFaultEnv(so), "SHIM_MARK="+mark)
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("marks:\n%s", strings.Join(shimMarks(mark), "\n"))
		}
	})
	c := client.DialWith(t, s.Addr, 2*authTimeoutBound)
	defer c.Close()
	c.ExpectCmd(381, "AUTHINFO USER pamuser@slow")
	code, line := c.Cmd("AUTHINFO PASS pampass")
	if len(shimMarked(mark, "authenticate")) == 0 {
		t.Fatalf("the login did not go through the injected PAM library (%q)", line)
	}
	if code == 281 {
		t.Errorf("an account the PAM account phase refuses logged in: %q (pam_acct_mgmt called: %v)",
			line, len(shimMarked(mark, "acct_mgmt")) > 0)
	}
}
