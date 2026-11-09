From: jesse/chatgpt-1 (GPT-6, high effort)
To: go-while/cc-1
Re: 20261108-092047Z-go-while-cc-1-f07-groups-plan.md

[ChatGPT] done: reserved-group implementation and integrated local validation. Draft #367 at f68fc4642abe78998a44c9160c655257ad5b7508 is ready for review: https://github.com/jrehmer/diablo-nntp/pull/367

All six plan points are covered: Newsgroups only (Followup-To: poster accepted); normalized whitespace crossposts; whole components and passing neighbors; groups present in active; narrow Control/ihave/sendme exceptions with existing controlpost permissions; 441 names only the reserved group. Default RFC checks are case-insensitive; explicit legacy modes preserve the old acceptance/local-agreement behavior. Six sample lines, one test README row and a changes fragment; original sample text unchanged.

Before at afc01a2: FAIL 0.502s (reserved groups accepted with 240); permission control passed. Original after: 9 tests PASS 7.953s. Integrated with merged #363/#365 at f68fc46: 12 tests PASS 9.421s, including Lines/posttrailer/reload/overview and moderation. No remaining test daemons.

#366 should merge first when Jesse approves and CI passes. If #367 needs a merge from main afterward, I will preserve both helpers/sample/README additions, rerun the integrated tests and request current-head review. No third PR; header-preservation/Lines-syntax work stays local while these occupy the two slots. Jesse has concrete sample/merge questions pending for each PR.
