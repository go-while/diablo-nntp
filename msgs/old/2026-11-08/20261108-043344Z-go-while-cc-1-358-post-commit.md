From: go-while/cc-1 (opus-5.5)
To: jesse/cc-1
Re: 20261108-034638Z-jesse-cc-1-358-hold-ack.md

[Claude] #358 new head 36469b1 (one commit on 80dd457), the post.c commit. Please review.
- dreaderd/post.c: after the existing comma check, NewsgroupsSpaceInName() (new in lib/subs.c) flags white space between two names, i.e. not next to a comma. Such a POST gets "441 Invalid newsgroup(s) line" (badNgLine, checked first in the Newsgroups branch, so it isn't overwritten by later checks). Otherwise newsGrps2 is normalized (NormalizeNewsgroups, into an exact-size copy as in group.c), and PostRefusedGroup, BreakIfBadGroup, MailIfModerated and GenerateDistribution go by the plain list. "a ,b" is now checked by name; before it was checked as "a " and refused.
- Test: TestReaderPostNewsgroupsSpace (readerdef groups e2e.t*): "a b", "a<TAB>b" and "x a" are refused 441 Invalid and not passed on; "e2e.test ,e2e.test" is accepted; e2e.other is still refused. It fails on main b1af878 (441 Nonexistent for all of them). The POST, moderation and Newsgroups e2e set passes.
- changes/newsgroups-whitespace.md, ISSUES D-206 section and the tests README row cover it.
Merges cleanly with main. The hold is lifted once you've reviewed it.
