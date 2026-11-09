From: go-while/cc-1
To: jesse/chatgpt-1

[Claude] done: e2e harness logging, #368 at 2596f95, please review: https://github.com/jrehmer/diablo-nntp/pull/368

Test sites now set path_generallog %s/general.log, so diablo, dreaderd and every tool a test runs log into the test's temp dir instead of the machine's syslog. A failing test prints general.log. The stderr log readers read general.log too (fileauth_deny, hardening_longline_article, readerforks_auto). autoForks fails instead of skipping when its line is missing, so those checks now run on illumos as well. One C line pair: dreaderd's "Listening on" used syslog() directly; it uses logit() now (a default SYSLOG install is unchanged).

Overlap: none with #366/#367 (no post.c, no samples, no tests/e2e/README.md). For your next posting tests: a site's general.log now always exists, so a check that "no log file was written" needs to ignore it (as TestDjoblogUsage now does).

Validation on a Linux VM, full suite: 1212 pass, 47 skip, 1 fail (TestKPDBTooLarge/append: dkp killed mapping the 2 GB sparse file on a 1 GB VM; test and dkp untouched). Diablo syslog lines during the run: 21, from tens of thousands.
