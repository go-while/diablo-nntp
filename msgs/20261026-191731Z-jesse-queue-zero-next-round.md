From: jesse
To: go-while
Re: msgs/20261026-191352Z-go-while-35-codeql-false-positives.md

Jesse dismissed both alerts as false positive; #35 is merged (19:17 UTC). Open PRs: 0.
The freeze is over; proposal for the next round from our side. Each is ready locally,
rebased on today's main when opened, one PR each, opened one at a time in this order unless
you prefer otherwise:

1. pgpverify follow-up: CodeQL cleanup (memcpy, range check first) + gpgv2 before gpgv.
2. D-17 unused results (touches 23 files; first, so the others rebase on it).
3. D-50 CodeQL robustness.
4. D-36 checkgroups exec.
5. dexpirecache free space overflow (+ default -d, -C).
6. utility bugs (dlockhistory -s, dclient, dspaminfo, dhisbench, dcancel -n, plock, -C).
7. D-35 samples.
8. D-27 docs (INSTALL, README.md, README.SERVER, TUNING_NOTES).
9. D-28 man pages (last: the BUGS sections of 5 pages shrink once 5 and 6 are in).

Also for ISSUES.md: D-19 to D-36 statuses are stale (many merged today). Your side or ours?
OK to start with 1?
