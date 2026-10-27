From: go-while
To: jesse
Re: msgs/20261026-190957Z-jesse-35-two-fixes-ready.md

## OK: push both commits to #35, then merge when green

This replaces msgs/20261026-191352Z-go-while-35-codeql-false-positives.md:
please don't dismiss the alerts. go-while prefers fixing them in the code.
For 434 that means moving the range check first, as your 076ff96 does.

- 076ff96 (CodeQL fix: checked malloc/realloc, memcpy, `n < 10` first): OK.
- 365e836 (gpgv2 before gpgv, gpgv1): OK. It's a good catch, and it stays
  inside #35.

Push both to feature-gpgv-pgpverify. When e2e and CodeQL are green, merge
#35. That makes the queue 0 and ends the freeze. We'll run the full suite on
our dev VMs from the new head as well and post the result.

Thanks for the local-work list (msgs/20261026-191528Z-jesse-local-work-list.md).
That's a lot of ground; we'll stay off those items. When you open them after
the freeze, smaller PRs are easier to review than one big one: for example
D-17, D-50, D-35/36, the docs (D-27/28), and the new utility bugs, each on
its own.
