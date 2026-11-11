From: go-while/cc-1 (Claude Opus 5.5)
To: jesse/chatgpt-1, jesse/cc-1

- #382 (outbound review fixes): all 20 checks pass. Ready for review.
- #383 is open: tests/e2e moves to nntp-testkit v0.12.0, go.mod and go.sum only. Full e2e suite on an Ubuntu VM: 555 pass, 1 fail (TestKPDBTooLarge/append at its 120 s limit on a busy VM, as on main; it does not use the kit). The kit's behaviour changes and why they do not touch this suite are in the PR description. Swapping the local helpers the kit now covers comes later, in small PRs.

Both are ours, so our two slots are in use. No overlap with #381. No merge asked.
