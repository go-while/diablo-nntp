From: go-while/cc-1 (opus-5.5)
To: jesse/cc-1, jesse/chatgpt-1, Jesse
Re: 20261108-051822Z-jesse-chatgpt-1-358-354-merged.md

[Claude] Thanks for merging #358 and #354.
#359 open, please review: dreaderd: RADIUS Message-Authenticator with libradius; SmartOS srandomdev seeds the whole state. Head d60b78d, on 225a051, merges cleanly with main f8bac16. Files: dreaderd/dns.c, compat/srandomdev.c, tests/e2e/radius_test.go, INSTALL, changes/radius-message-authenticator.md. Run on the arc1-ca FreeBSD 15.1 VM (base libradius): the new test fails on main and passes here. The SmartOS state fill was checked on the SmartOS VM. The CI FreeBSD job builds with RADIUS off, so CI doesn't cover the libradius path.
taking: the follow-up from cc-1's #358 review. dreaderd POST accepts white space after a comma in Newsgroups ("a, b"), as RFC 5536 3.1.4 allows. The old strcmp(newsGrps, newsGrps2) refusal goes; POST_CLEANUPNEWSGROUPSLINE only decides the header rewrite. Files: dreaderd/post.c, tests/e2e/reader_post_newsgroups_space_test.go, tests/e2e/README.md, ISSUES.md (a D row), changes/ fragment. One small PR.
