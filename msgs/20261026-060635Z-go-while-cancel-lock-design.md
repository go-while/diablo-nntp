From: go-while
To: jesse

# Cancel-Lock (RFC 8315) for diablo + dreaderd: design for your OK

Short version: it fits well. Compatible with INN, no new library, about 600 lines of C in 3 PRs.
References are main ed3ce67; checked by reading the code.

## Where it goes
- **Only dreaderd executes cancels** (README.READER). The feeder doesn't, so the feeder needs no
  change. dcancel is a trusted local admin tool and stays as it is.
- **Cancels and Supersedes share one function.** ExecuteSupersedes (dreaderd/control.c) builds a
  fake "cancel <id>" and calls ExecuteControl, which reaches CtlCancel (control.c:597) through
  the dcontrol.ctl cancel line. One check covers both. RFC 8315 doesn't cover Replaces, and
  Diablo doesn't handle it anywhere.
- **There's already a hook.** CtlCancel has a dead `#ifdef NOTDEF` block (control.c:640-661)
  that fetches the target's stored headers with GetOverRecord and sets isValidCancel to +1 or
  -1. The overview data file holds all headers, also with a header-only feed, so the target's
  Cancel-Lock is local and no spool fetch is needed. Exception: `readerxover trackonly` stores
  no headers, so verification can't work there.
- **Generation** goes in dreaderd/post.c NNPostBuffer, after the final Message-ID, next to
  X-Trace. The identity is co_Auth.dr_AuthUser (none when empty, "!"-prefixed or too long).

## Crypto: OpenSSL, not libcanlock
- HMAC, EVP_Digest and EVP_EncodeBlock from libcrypto, which is already linked. A new
  lib/canlock.c of about 250 lines.
- The construction is the standard one: key = b64(HMAC(secret, user+mid)),
  lock = b64(H(key)). The agent recomputed all 8 test vectors from INN's tests/lib/canlock-t.c
  with plain HMAC+SHA+base64 and they match. So with the same secrets as in your
  inn-secrets.conf, locks are byte-identical to INN's, and articles INN injected stay
  cancellable after you switch.
- Accept sha1, sha256 and sha512; generate sha256 (plus sha1 like INN, see question 3).

## Config
- diablo.config `readercanlock off|log|auth|require`:
  - off (today's behaviour, like INN "all")
  - log (verify and log, still cancel)
  - auth (a locked target needs a valid key, an unlocked one is cancelled)
  - require (like INN require-auth)
  
  Verification needs no secret. A target we can't check counts as "unavailable", never as
  "no lock".
- dreader.access vserverdef `canlockadmin <secret>` and `canlockuser <secret>` (repeatable,
  for rotation), plus readerdef `addcanlockuser username|ip|none` (default username), the same
  model as INN's readers.conf.

## PRs
1. Verification in CtlCancel, covering cancels and Supersedes: lib/canlock.c, control.c,
   lib/config.c, docs. About 350 lines.
2. Generation on POST (Cancel-Lock; Cancel-Key for an identified user's own cancel or
   Supersedes): post.c, dns.c, samples. About 150 lines.
3. A small dcanlock tool (print an admin Cancel-Key for a Message-ID, like INN's gencancel -k;
   verify a key) and a man page section.

Each comes with e2e tests (Go computes the keys):
- matching, wrong and missing key;
- locked and unlocked targets per mode;
- sha1 in a folded lock;
- Supersedes good and bad;
- a cancel arriving before its article;
- the lock added on POST, the user key, and a full round trip.

## Questions for Jesse
1. **Default mode:** off (compatible), or auth/require? Which docancels mode does your INN use?
2. **Early cancels:** under auth and require, a cancel that arrives before its article can't be
   checked, so it's dropped and the article survives (INN does the same). OK, or should we
   keep the key and check when the article arrives?
3. **sha1:** also generate it, as INN does for interop? RFC 8315 deprecates it.
4. **Secrets:** inline in dreader.access (often world-readable), or a separate 0600 secrets
   file?
5. **trackonly readers** can't verify. OK to refuse readercanlock with trackonly at startup?

Side note: POST_RESTRICTCANCEL is off by default (lib/vendor.h:63), so today any dreaderd user
can post a cancel for any Message-ID. Verification on the receiving side is what closes that.

If this looks right, go-while starts with PR 1 after your answers.
