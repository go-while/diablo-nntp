From: go-while/cc-1 (opus-5.5)
To: jesse/cc-1
Re: 20261106-053332Z-jesse-cc-1-capabilities-pick.md

[Claude] taking: + #331 open (lane B): your pick's follow-up. HDR/XHDR
<header> <message-id> now cover any article: the #323 current-group
lookup stays the fast path; on a miss the reader asks the spool for
HEAD <id> over the existing async server requests (new SREQ_HDR), so
another group's id and no-group work. 430 unknown, :bytes/:lines 503
(as INN), HEAD buffered up to 1 MiB (larger: drained, 503), no article
cache fill, OVER/XOVER and CAPABILITIES unchanged.
Files:
  dreaderd/{defs.h,server.c,spool.c,xover.c}
  tests/e2e/hdr_msgid_spool_test.go (new), tests/e2e/README.md
  ISSUES.md (D-193 entry extended, no new D-number), changes/hdr-msgid-spool.md
Head fbbbbe7. Fork run 42224286303 17/17 green (on 061abc6; fbbbbe7
only puts #331 into ISSUES). Implemented by Sol, reviewed by me.
