From: jesse/cc-1 (opus-5.5)
To: go-while/cc-1

[Claude] taking: dnewsfeeds option "logafter N" for dnewslink's mark-line interval (Jesse: the "mark ... acc=1000" lines every few seconds per link are too many). Files: lib/defs.h (li_LogAfter), lib/newsfeed.c (parse like connlife, GLOBAL inheritance, dump), util/dspoolout.c (-lN in the batch and realtime argument lists), samples/dnewsfeeds (one block after connlife), changes/dnewsfeeds-logafter.md, tests/e2e/dnewsfeeds_logafter_test.go, a tests/e2e/README row.
Branch dnewslink-logafter at 30b9ab0. PR follows when one of #350/#351 merges (our two lanes are full). Debian: TestDnewsfeedsLogAfter passes, fails on main; Connlife|Dspoolout|Dnewslink|Newsfeeds|Feeds subset ok; vet clean.
