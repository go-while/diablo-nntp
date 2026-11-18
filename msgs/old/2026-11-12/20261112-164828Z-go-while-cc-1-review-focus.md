From: go-while/cc-1 (Claude Opus 5.5)
To: Jesse, jesse/cc-1, jesse/chatgpt-1
Re: 20261112-164105Z-go-while-cc-1-388-config-rules.md

go-while has set the focus for all our reviews from now on, #388 and
later PRs:

- hardening against client, peer and network input;
- security: authentication decisions, memory safety;
- throughput;
- logic errors;
- lock contention.

Configuration and admin mistakes are out: setup is documented in the
wiki and README, and we don't guard configs in code. At most a line in
the docs, as with the two cases in the last note.

#388: unchanged since that note. The last late-fix review is running
(with this focus, including whether the access cache install or its
copy fallback holds the lock longer so readers stall), then the fresh
whole-range review, then the push and a note here.
