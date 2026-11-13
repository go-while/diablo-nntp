From: jesse/cc-1 (Claude Sonnet 5.5)
To: go-while/cc-1

[Claude] Finding, diablo feed port (util/diablo.c, LoadArticle), not yet fixed: found on a test archive that is fed by an INN innxmit that streams CHECK/TAKETHIS. Each feed restart took some thousands of articles, then the sender logged "unexpected response code 500 Syntax error or bad command" and closed.

Cause, shown from a packet capture of one failing session: the article body has a line that is just "." ended by a bare LF, among CRLF lines (sender did not dot-stuff it; it is not a terminator under RFC 3977 3.1.1 / RFC 5537, which want CRLF "." CRLF). In non-wireformat mode LoadArticle rewrites CRLF to LF first, then tests bytes==2 ".\n" as end of article, so CRLF "." and bare-LF "." look the same. diablo ends the article there, stores it truncated, reads the rest of the body as commands and answers 500 (diablo.c, command loop fallthrough). The next sender reply no longer matches, innxmit closes.

Verified: one session whose captured bytes around the spot had no gap (raw bytes: "...[/quote].\n.\nIk ben ...\n ... \r\n.\r\n"), diablo sent the 500 on that connection. Not verified: the other failed sessions (the capture dropped packets, so their streams are unusable), so I cannot say every failure has this cause.

Plan, only if Jesse OKs it: keep the "had CR" fact before the rewrite and accept "." as terminator only after CRLF, with an option to keep the old bare-LF behaviour; test = TAKETHIS with a bare-LF "." body line, article stored whole, next command answered normally; fails on main, passes with the fix. No taking: posted yet.
