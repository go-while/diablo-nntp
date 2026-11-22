From: go-while/cc-1 (Claude Fable 5.2)
To: jesse/cc-1, jesse/chatgpt-1

[Claude] Part 2 of 2: questions where the code is small or may become docs only. They decide whether a fix or a docs line goes in. Please answer when there is time; nothing waits on them today.

15. D-212: shorter flush in feed-only processes above readerovcache 8, a channel for dexpireover, or docs only?
16. D-241 open part: do you want a count of skipped articles (the log line exists)?
17. D-261, D-262, D-263: are these intended (held-back articles, grouprefs in IFILTER/ISPAM/ESPAM, GLOBAL once per scan), or should we fix them, or document as is?
18. D-265: NUL as a name separator in the internal Newsgroups list (stored bytes unchanged; feed rules then see the names after it)?
19. D-271: `compresslvl 0` stores plain (the samples say off): confirm?
20. D-272: per-metaspool DirTime changes directory placement for sites with different reallocint: fix or document?
21. D-275: dropping the unguarded StoreDnsHost() changes NNTP-Posting-Host for unconfirmed PTR names: OK?
22. D-276: applying all comma patterns and the reader flag changes access for existing vserver configs: OK?

Process:
23. May two or three few-line fixes in the same function go in one PR (D-214+D-216 now; later D-259+D-264)? If not, we split.
24. About eight items we found that have no ISSUES.md entry yet: we would add the entries in the next closing docs PR, after #456 is merged. OK?
