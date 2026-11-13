package e2e

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-while/nntp-testkit/client"
)

// pamConvSource replaces the PAM library calls of dreaderd with a module that asks its
// questions the way a PAM stack may: one conversation call with a text message, a prompt
// for the user name and one for the password. It accepts the login only when the answer
// array has an entry for each message and each answer is at the index of its message, and
// marks what it got.
const pamConvSource = `#define _GNU_SOURCE
#define __EXTENSIONS__
#include <fcntl.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <security/pam_appl.h>
#ifdef __GLIBC__
#include <malloc.h>
#endif
/*MARK*/
static char shimUser[256];
static const struct pam_conv *shimConv;

int
pam_start(const char *service, const char *user, const struct pam_conv *conv, pam_handle_t **pamh)
{
    (void)service;
    snprintf(shimUser, sizeof(shimUser), "%s", user ? user : "");
    shimConv = conv;
    *pamh = (pam_handle_t *)shimUser;
    return PAM_SUCCESS;
}

int
pam_authenticate(pam_handle_t *pamh, int flags)
{
    struct pam_message m[3] = {
	{ PAM_TEXT_INFO, "welcome" },
	{ PAM_PROMPT_ECHO_ON, "login: " },
	{ PAM_PROMPT_ECHO_OFF, "Password: " },
    };
#ifdef __sun
    /* Solaris/illumos: a pointer to one array of the messages */
    const struct pam_message *mp[3] = { &m[0], NULL, NULL };
#else
    /* Linux-PAM, OpenPAM: one pointer per message */
    const struct pam_message *mp[3] = { &m[0], &m[1], &m[2] };
#endif
    struct pam_response *resp = NULL;
    int ok = 1;
    int i;
    int r;

    (void)pamh; (void)flags;
    r = shimConv->conv(3, (void *)mp, &resp, shimConv->appdata_ptr);
    if (r != PAM_SUCCESS || resp == NULL) {
	shim_mark("conversation failed: %d", r);
	return PAM_CONV_ERR;
    }
#ifdef __GLIBC__
    /* 0 when a write behind the array cleared the in-use bit of the next chunk */
    if (malloc_usable_size(resp) < 3 * sizeof(*resp)) {
	shim_mark("answer array of %ld bytes for 3 messages", (long)malloc_usable_size(resp));
	return PAM_AUTH_ERR;	/* reading on would be behind the array */
    }
#endif
    if (resp[0].resp != NULL) {
	shim_mark("answer to the text message: %s", resp[0].resp);
	ok = 0;
    }
    if (resp[1].resp == NULL || strcmp(resp[1].resp, shimUser) != 0) {
	shim_mark("answer to the user prompt: %s", resp[1].resp ? resp[1].resp : "(none)");
	ok = 0;
    }
    if (resp[2].resp == NULL || strcmp(resp[2].resp, "pampass") != 0) {
	shim_mark("answer to the password prompt: %s", resp[2].resp ? resp[2].resp : "(none)");
	ok = 0;
    }
    for (i = 0; i < 3; i++)
	free(resp[i].resp);
    free(resp);
    shim_mark("answers %s", ok ? "right" : "wrong");
    return ok ? PAM_SUCCESS : PAM_AUTH_ERR;
}

int
pam_end(pam_handle_t *pamh, int status)
{
    (void)pamh; (void)status;
    return PAM_SUCCESS;
}
`

// TestDeepRevPAMConversation (L6-1): the PAM conversation function of dreaderd (dns.c,
// PAM_conv()) gave PAM an answer array of one entry, whatever the number of messages, and
// wrote the answers one after the other: a second prompt in one call was written behind the
// array, and after a text message the answer sat at the index of that message while PAM
// reads the one of its prompt.
//
// A preloaded PAM library asks a text message, the user name and the password in one call,
// in the layout of the platform's PAM (a vector of message pointers; on Solaris/illumos one
// array of messages): the login with the right password must be accepted. A wrong password
// is still refused.
func TestDeepRevPAMConversation(t *testing.T) {
	builtWith(t, "Unable to release PAM authentication", "PAM support (PAM_ENABLED)")
	so := preloadShim(t, "pamconv", withMark(pamConvSource))
	mark := filepath.Join(t.TempDir(), "mark")
	s := authTimeoutSite(t, slowAuthAccess("pam diablo"), lockFaultEnv(so), "SHIM_MARK="+mark)
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("marks:\n%s", strings.Join(shimMarks(mark), "\n"))
		}
	})
	login := func(pass string) (int, string) {
		t.Helper()
		c := client.DialWith(t, s.Addr, 2*authTimeoutBound)
		defer c.Close()
		c.ExpectCmd(381, "AUTHINFO USER pamuser@slow")
		return c.Cmd("AUTHINFO PASS %s", pass)
	}
	code, line := login("pampass")
	if len(shimMarked(mark, "answer")) == 0 && len(shimMarked(mark, "conversation")) == 0 {
		t.Fatalf("the login did not go through the injected PAM library (%q)", line)
	}
	if code != 281 {
		t.Errorf("pamuser with the right password: %q, want 281; the conversation answers: %q",
			line, shimMarks(mark))
	}
	if code, line := login("wrong"); code == 281 {
		t.Errorf("pamuser with a wrong password logged in: %q", line)
	}
}
