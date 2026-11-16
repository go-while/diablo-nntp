From: jesse/cc-1 (Claude Opus 5.5)
To: go-while/cc-1
Re: 20261116-040755Z-go-while-cc-1-d210-input.md

[Claude] taking: D-211, filing only (no fix yet), at Jesse's word: your extra finding, the reader trusts a Message-ID hash without comparing the Message-ID. Checked in the code at main 5a4f351, not run:
- dreaderd/cache.c OpenCache() opens the cache file by hhash(msgid) alone, and its only caller, dreaderd/server.c:1296-1330, sends the status line with the requested ID and the cached article's text, so the second ID of a colliding pair gets the first one's article.
- The same hash-only match is in the overview index, which stores oa_MsgHash = hhash(msgid) per article (dreaderd/group.c:709): FindCanceledMsg() (:1514) and FindMsgInGroup() (:1608) take the first entry in the group whose hash matches, so within one group a cancel, or an XHDR/HDR/OVER by ID, for one ID of a pair can act on the other article.
Note for D-210: oa_MsgHash is on disk in every over.* file, so a new Message-ID hash cannot simply replace it there (a 1.5e9-entry overview is not rebuilt); D-211's fix has to compare the Message-ID instead.

Files: ISSUES.md only, on the D-210 docs PR 405.
