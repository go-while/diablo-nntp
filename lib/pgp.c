
/*
 * PGP.C	- run pgpverify
 *
 * (c)Copyright 1998, Matthew Dillon, All Rights Reserved.  Refer to
 *    the COPYRIGHT file in the base directory of this distribution
 *    for specific rights granted.
 *
 */

#include "defs.h"

Prototype const char *PGPVerify(Control *ctl, const char *art, int artLen);
Prototype const char *PGPSigner(const char *art, int artLen, char *signer, int signerSize, int *pcannot);

/*
 * PGPVerify()
 *
 * cat article | pgpverify.  If the output of the program is non-null it
 * contains the verified pgp user id which we compare against the
 * control verification string.
 *
 * Return:
 *	NULL	success
 *	string	failure (return error template)
 */

#if DIABLO_PGP_SUPPORT

const char *
PGPVerify(Control *ctl, const char *art, int artLen)
{
    char signer[256];
    const char *badCmd = PGPSigner(art, artLen, signer, sizeof(signer), NULL);

    if (badCmd == NULL && strcmp(signer, ctl->ct_Verify) != 0) {
	if (DebugOpt)
	    printf("pgp label mismatch:%s:%s:\n", signer, ctl->ct_Verify);
	badCmd = "pgp parsed ok, but label did not match\n";
    }
    return(badCmd);
}

/*
 * PGPSigner() - run pgpverify on the article once: NULL and the user id of the
 * key that made a good signature in signer, or the failure.  *pcannot (if not
 * NULL) is set when the signature could not be checked at all (pgpverify not
 * runnable, or its exit status 2), as opposed to a missing or bad signature.
 */
const char *
PGPSigner(const char *art, int artLen, char *signer, int signerSize, int *pcannot)
{
    const char *badCmd = "Unable to exec pgp verification program\n";
    int cannot = 1;
    int fds[3];
    pid_t pid;

    if (DebugOpt) {
	int i = 0;
	while (i < 10 && DOpts.PgpVerifyArgs[i] != NULL) {
	    printf("PgpverifyArg[%d]=%s\n", i, DOpts.PgpVerifyArgs[i]);
	    i++;
	}
    }

    if ((pid = RunProgramPipe(fds, RPF_STDIN|RPF_STDOUT,
					DOpts.PgpVerifyArgs, NULL)) > 0) {
	/*
	 * write article to pgpverify, result answer
	 */
	char buf[256];
	int n;
	int lcr = 0;
	FILE *fo = fdopen(fds[1], "w");

	for (n = 0; n < artLen; ++n) {
	    if (lcr == 0 && art[n] == '\r') {
		lcr = 1;
		continue;
	    }
	    if (art[n] == '\n')
		lcr = 0;
	    if (lcr) {
		fputc('\r', fo);
	    }
	    fputc((int)(uint8)art[n], fo);
	    lcr = 0;
	}
	fflush(fo);
	if (ferror(fo))
	    logit(LOG_ERR, "write error sending article to pgpverify");
	fclose(fo);

	{
	    int v = 0;
	    for (n = 0; n < sizeof(buf); n += v) {
		v = read(fds[0], buf + n, sizeof(buf) - n);
		if (v <= 0)
		    break;
	    }
	    if (n > 0 && buf[n-1] == '\n')
		--n;
	    if (n > 0 && buf[n-1] == '\r')
		--n;
	    if (n == sizeof(buf))
		--n;
	    buf[n] = 0;

	    /*
	     * pgpverify's exit status says whether the answer is a signer,
	     * or why there is none.
	     */
	    close(fds[0]);
	    fds[0] = -1;
	    {
		int status = 0;

		waitpid(pid, &status, 0);
		pid = -1;
		if (!WIFEXITED(status)) {
		    ;			/* killed: "Unable to exec" stands */
		} else if (WEXITSTATUS(status) == 0 && n) {
		    /*
		     * a good signature: the answer is the signer
		     */
		    snprintf(signer, signerSize, "%s", buf);
		    badCmd = NULL;
		    cannot = 0;
		} else {
		    switch (WEXITSTATUS(status)) {
		    case 0:
			badCmd = "pgp verification gave no signer\n";
			break;
		    case 1:
			badCmd = "pgp signature header (X-PGP-Sig) missing\n";
			cannot = 0;
			break;
		    case 3:
			badCmd = "pgp signature bad, or signer key not in keyring\n";
			cannot = 0;
			break;
		    default:
			badCmd = "pgp header parsing failed, or verifier not runnable\n";
			break;
		    }
		}
	    }
	}
	if (fds[0] >= 0)
	    close(fds[0]);
	if (pid > 0)
	    waitpid(pid, NULL, 0);
    } else {
	logit(LOG_ERR, "Unable to run %s", DOpts.PgpVerifyArgs[0]);
    }
    if (pcannot)
	*pcannot = (badCmd != NULL && cannot);
    return(badCmd);
}

#endif

