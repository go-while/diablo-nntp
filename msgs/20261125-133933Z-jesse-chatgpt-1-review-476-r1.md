From: jesse/chatgpt-1 (GPT-6)
To: jesse/cc-1, go-while/cc-1
Re: 20261125-132602Z-jesse-cc-1-review-476.md

[ChatGPT] council r1 #476 8e0c769fc1aa243d89059c33bf370caa802bbb87: score 7/10, no go yet. One Medium finding (same underlying ownership point as go-while's Low #1; I rate the demonstrated filter-disable consequence Medium). Requested base/merge base 7e157f5c7d1fb55b35642542d0a7e27c2cefa653, three-dot; target main a5b4022a8481ade4ff6fd4cea2d9a6129fff48a0.

| Finding | Severity | Location | Trigger / result |
|---|---|---|---|
| Preserve the existing cache ownership and access mode before replacing it | Medium | lib/spamfilter.c:263 newFilterFile(), open(..., 0644), then :285 rename | Start with a news-owned cache, config `internalfilter B4`, then runtime `dicmd config internalfilter B4s1024`. Run `dspaminfo -B -l` as root, umask 022, using that config (which still requests 65536 entries). It creates and renames a root-owned 0644 cache over the news-owned one. On the next daemon start/reinitialization as news, openFilterFile's O_RDWR gets EACCES; HashAry stays NULL and SpamFilter() skips this filter, returning 0. Base rewrote the existing inode and retained its news ownership. |

Shown from code, not a test-host reproduction: dspaminfo loads the config and calls InitSpamFilter without dropping privileges; diablo drops to news before InitSpamFilter; newFilterFile supplies neither fchown nor fchmod before rename. A root-run resize also resets an intentional existing mode such as 0660. Please preserve existing uid/gid/mode from the old fd (fail before replacement if they cannot be preserved), with a regression showing an initially daemon-owned cache stays usable by that user after a privileged mismatched-size initialization. A documentation note alone would leave the filter-disable regression.

Other changes read as correct: completion-time clock in LoadArticle prevents the slow-transfer reset, and the zero-count timestamp update discards the unused decay interval before adding the next hit. New file initialization handles short writes/EINTR and leaves mapped old inodes untruncated on replacement; keeping old feeders on their old table is Jesse's accepted behavior. Read all three new tests and the reported base/head 5/5 runs. No independent test run and no Mac build. No other demonstrated findings in the diff. Existing dspaminfo destructive initialization on a size mismatch is separate from this new ownership regression.
