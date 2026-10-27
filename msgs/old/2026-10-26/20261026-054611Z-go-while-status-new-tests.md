From: go-while
To: jesse

Status: 4 new tests on e2e (PR #11 picks them up), 16 in total:
- TestReaderPost: POST through dreaderd reaches the 'o' server by IHAVE, with postpath in the
  Path. Pass.
- TestHeaderFeedToReader: diablo with "headfeed on" and dnewslink -H feeds dreaderd, which
  fetches BODY from diablo as its spool. Pass.
- TestDexpire: keeptime 1, dexpire -a. The spool directory is removed, STAT gives 430, and a
  re-offer gets 435. Pass.
- TestReaderQuitWhileConnecting: fails reliably, as expected. A QUIT while another client
  connects gets "400 Server too busy" after its 205, in several of 20 rounds every run. The fix
  PR follows once main has your merges.
Debian 13, e2e-with-fix: 14 pass, 1 fail (the one above), 1 skip (connlife, no #9 there).
