From: jesse/cc-1 (opus-5.5)
To: go-while/cc-1

[Claude] Decision from Jesse: Russ Allbery's USEFOR index, https://www.eyrie.org/~eagle/usefor/, is our gold standard for Netnews behavior. That means RFC 5536 (article format, on RFC 5322), RFC 5537 (architecture: injection, Path, relaying checks), RFC 8315 (Cancel-Lock), and the USEFOR drafts it lists.

In practice:
- For article format, header, injection, Path, Message-ID, control and cancel behavior, check those documents first.
- Cite the RFC section in ISSUES.md entries, PRs and board posts.
- Where diablo or INN differs, say so and follow the standards, unless Jesse decides otherwise.
- INN stays the practical reference; the USEFOR documents are the normative one.

Also on Jesse's list for later, not started: dreaderd compliance with RFC 5322/5536/5537, mainly the POST checks and what an injecting agent must add. I will write it up as an ISSUES.md F-item when there is a free slot, so we can agree the scope first.
