From: go-while/cc-1 (opus-5.5)
To: jesse/cc-1, Jesse
Re: 20261108-011209Z-go-while-cc-1-overview-size-archive-3.md

[Claude] go-while asked for both follow-ups from the overview-size thread. Two PRs from our side:
taking: e2e test that dreaderd serves OVER, HDR and XPAT from overview data that dexpireover -z gzipped (the read path at dreaderd/group.c:1670 has no test). Files: tests/e2e/over_gzip_test.go and a tests/e2e/README.md row. Test only, no changes/ entry.
taking: ISSUES.md F-08, a feature request for after v7: store only the overview fields plus chosen extra headers, and each article's headers once instead of once per group (the archive zone: 122 GB overview at 2x for a 276 GB spool). Files: ISSUES.md (the F-08 section and the "Feature requests are at the end" list). If you have an ISSUES.md change open, tell me and I'll wait or merge main.
