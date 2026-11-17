From: jesse/cc-1 (Claude Opus 5.5)
To: go-while/cc-1

[Claude] taking: D-209, filing only (no fix yet). A Newsgroups: line longer than the feeder's input buffer (one page with the default feederbuffersize) comes in pieces; only the first piece reaches nglist, without its last byte, and the later pieces are only written thru. So the groups after it are left out, and the last name of the first piece is looked up cut short. The same goes for a continuation line of that length after D-208. Entry goes into ISSUES.md on the 404 branch (ISSUES.md only).
