From: jesse [Claude]
To: go-while
Re: msgs/20261027-023558Z-go-while-scope-d52-d64-and-88.md

[Claude] Two additions to #93 and a correction on the feeder-stats PRs.
- New #93 (feeder-stats Top1000 rank): top1000-fetch.sh gets https://top1000.anthologeek.net/top1000.txt (curl/fetch/wget, daily, keeps the old file on a bad download), diablo-stats.sh runs it and passes the new -T dir, the "Pos1000 is empty" lines only print with -v. The old column never had data because nothing fetched the list (it is read from /news/top1000/current) and the old site moved; the list's line format is what the parser already read.
- I merged #88, #90, #91, #92, #93 together on a scratch branch and ran all five test scripts on the result: all pass. Then I ran the combined script on the Ubuntu test VM's real feeder log (converted to syslog form in a temp dir; /news only read) with the real list: the real peer shows as `#3` in the Top1000 column through an alias line (temp copy of dnewsfeeds), as its label with -L, the tables balance (560 td / 560 /td), the %Acc total is right.
- Correction: I said they merge cleanly in any order. Only the FEEDER_STATS_QUEUECMD line does. The CHANGELOG entries of the five PRs sit at the same spot and conflict with each other, and #88 conflicts with #93 in the getopts string, the usage text and 00README (-L vs -T). Whichever you take second, I will merge main into the branch and post the new head (no force-push). Suggested order: #88 first, then #93, then #90, #91, #92.
- With -L the Top1000 rank is blank (the key is the label, and the aliases are stored under host names). I will fix that in a small PR once #88 and #93 are in.
