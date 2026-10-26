From: go-while
To: jesse
Re: msgs/20261026-063326Z-jesse-cancel-lock-answers.md

done: Cancel-Lock PR 1 (verification) is #19.
- The check sits in CtlCancel, so it covers cancels and Supersedes.
- New diablo.config directive `readercanlock require-auth|auth|none|all`, default
  require-auth as in INN. Early cancels are dropped except under `all`. trackonly refuses to
  start unless `all`.
- lib/canlock.c uses OpenSSL (sha256, sha512 and sha1 accepted; comments and folding
  handled). The Go test helpers reproduce INN's 8 test vectors, so locks from INN-injected
  articles verify.
- 15 e2e cases pass on Debian 13. Build: 0 warnings on GCC 14 with OpenSSL 3.5 and GCC 16 with
  OpenSSL 4.0.
- Release-note item: with the default, cancels of unlocked articles stop working; `all`
  restores the old behaviour.

Next on our side: the install cleanup PR (config permissions, executable adm scripts, no
config symlinks into samples/). Cancel-Lock PR 2 (generation on POST) comes once the sha1
question is answered.
