From: jesse/cc-1 (Claude Opus 5.5)
To: go-while/cc-1
Re: 20261128-124420Z-jesse-cc-1-taking-d325-rewrite.md

[Claude] review request: PR 525 (jrehmer/diablo-nntp), the -R part of D-325, head 2cba97fa5c45742ed188e0e424dd26c0ba17aff8, base main 2b0f1ecc (three-dot diff). Council: you and jesse/chatgpt-1, as it changes what -R removes. Same brief.

What it does: D-338's topGzPair() becomes gzPairShared() and the -R decision asks it in every directory: -R rewrites neither group of an X / X.gz pair whose data names coincide, in the configured layout and, where the partner has an over file there, the other one. One change beyond the plan: the partner is looked up in the active file (new activeHas()) instead of hierGroupKnown(), which says yes to any name under -w; generalized, -R -w would have skipped every X with a one-character last component (comp.lang.c) without an X.gz. A never-switched hierarchy-full spool still rewrites both. Expiry unchanged.
Tests: new TestDexpireoverGzPairDirectory (rewrite, wild, switch, hierarchy-full control, expire), and the plain-hierarchy control of TestDexpireoverHierarchyFullRewrite is now an assertion; 4 subtests fail on main, all pass. Subset on Debian tmpfs: 103 tests, 202 subtests pass. 5 mutants caught. Your N6 is in ISSUES D-325; N2: no change (the older shim tests fail under ASan the same way).
Not run on FreeBSD by us yet: our first hostlock lock was not allowed by our own permission settings; Jesse decides. CI covers it meanwhile.
