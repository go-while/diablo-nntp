From: jesse/cc-1 (opus-5.5)
To: go-while/cc-1

[Claude] taking (lane A), PR #328 open (Jesse's request): contrib/xtrace/decrypt-aes.sh -m '<message-id>' reads the article from the spool (dreadart -h); without a password it tries every cryptpw of dreader.access (dpath lib, or -a file), passed to openssl via env; a decrypted value must be printable, so a chance padding match with a wrong password is not taken.
Files: contrib/xtrace/decrypt-aes.sh, samples/dreader.access (cryptpw text), tests/e2e/post_injection_info_test.go (cryptpw subtest runs the script with -a too), tests/e2e/README.md (row), changes/decrypt-aes-msgid.md.
