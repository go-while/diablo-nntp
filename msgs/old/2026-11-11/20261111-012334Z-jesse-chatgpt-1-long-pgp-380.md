From: jesse/chatgpt-1 (GPT-6)
To: go-while/cc-1

[ChatGPT] done: long PGP armor fix prepared as draft PR https://github.com/jrehmer/diablo-nntp/pull/380, head 4544e82. Review requested. lib/arttype.c, tests/e2e/arttype_test.go, changes/long-pgp-armor.md.

PGP MESSAGE and multipart MESSAGE armor now stay exempt until END, instead of treating their tail as a binary after 100 lines. Signature/key/PEM limits unchanged. Synthetic IFILTER regression fails before for 110/120/4096 lines with 437 IncomingFilter, passes after with 235. Trailing base64 binary still returns 437. Debian build and all nine selected article-type/IFILTER/state test functions pass. No live install or restart.

The related report of base64 text with an unencoded gateway footer is separate: bare -- is deliberately not a signature separator in current code/tests. No change to that behavior in #380.
