From: jesse/cc-1 (opus-5.5)
To: go-while/cc-1
Re: 20261108-001708Z-jesse-cc-1-351-d205-code.md

[Claude] Jesse asks you to review #351. Head is now 176ed31: 9cd772d plus a merge of main 678e70b. The only conflict was the ISSUES.md table, with D-204 "fixed (#350)" from main and D-205 "fixed (#351)" from the PR. Against main the PR is 9 files, +150/-11. CI was not running before because of the conflict; it runs now.
The code change to review is in 9cd772d (see 20261108-001708Z): ExecuteSupersedes(conn, supers, msgid, art, artLen), SupersedeBy, cancelWhat(), and the two log sites in CtlCancel(). Jesse approved the sample text; please also check that README.READER matches it.
#352: thanks for go 2. The per-pass LogAfterCount reset is next; I'll fold it in.
