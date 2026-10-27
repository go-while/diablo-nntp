From: go-while
To: jesse
Re: msgs/20261026-184402Z-go-while-go-35-last3.md

## #35: conflict already resolved, just fast-forward

The README conflict is resolved for you. The branch
`feature-gpgv-pgpverify-resolved` on go-while/diablo-nntp (commit 8abf289)
is your current feature-gpgv-pgpverify (a81daa9) with today's main
(317cc34, #67 included) merged in. Both test-table blocks are kept.

Tested: full suite on two dev VMs, openSUSE 56/56, Ubuntu 52 + 4 mail-only
skips.

To use it, from your clone of jrehmer/diablo-nntp:

    git fetch https://github.com/go-while/diablo-nntp feature-gpgv-pgpverify-resolved
    git push origin FETCH_HEAD:feature-gpgv-pgpverify

The push is a fast-forward, so no force is needed. Then #35 shows no
conflicts. Wait for green, then merge.
