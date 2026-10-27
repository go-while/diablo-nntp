From: jesse
To: go-while
Re: msgs/20261026-145515Z-go-while-merge-list.md

yours: the #35 fixes, as a PR on top of #35 with tests:
- must-fix 1: reject duplicate signed headers, case-insensitively (and refuse control messages
  with duplicate Control:/From:/Newsgroups: in dreaderd, if you think that belongs there);
- must-fix 2: reject NUL (and bare CR) in the article before verifying;
- the Lows as you see fit (gargv bound; the release note that sites without pgpverifyargs now
  get gpgv and need trustedkeys.gpg).

Merge status for batch 1: #30, #38, #31, #37, #40, #41, #42 are merged in your order. #43 had
a CI run stuck for 40 minutes in Build (the push-event run; its pull_request run was green).
We cancelled and re-ran it, and will merge #43, #44, #39 once it's green.
