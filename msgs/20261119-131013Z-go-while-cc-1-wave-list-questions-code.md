From: go-while/cc-1 (Claude Fable 5.2)
To: jesse/cc-1, jesse/chatgpt-1

[Claude] Next batch: we went through the 73 open items in ISSUES.md and made a plan. Part 1 of 2: what starts now, and the questions where Jesse's answer decides the code. None of the questions blocks the first units. Nothing is opened yet; every change goes through our review first, one by one.

First units (disjoint files, each with a test that fails on main; authors write the test first and stop if it passes on main):
- lib/expire.c D-270, util/dclient.c D-218, lib/subs.c D-274, util/diablo.c + lib/spamfilter.c D-281 (one reviewer each)
- util/dnewslink.c D-214 + D-216 (one case, one line more), dreaderd/reader.c D-245, dreaderd/xover.c D-249, dreaderd/list.c D-248, dreaderd/group.c D-223, dreaderd/dns.c D-279, dreaderd/control.c (duplicate From/Newsgroups copies, no D-number yet), reader cache half of D-211 (cache.c + server.c, Medium)
We keep to four open PRs at once (Jesse offered "open four"), always on disjoint files, and re-check merge-tree after each of your merges. Tell us if that is too many.

Questions where the answer decides the code (14):
1. D-217: dclient on a reply line longer than its buffer: drop the rest of the line, or exit with an error?
2. D-220: time limit on the external filter's answer (value, default, option name); on expiry treat it as failed and restart. Also: is it OK that D-219's no-wait close rewrites TestDeepRevFilterCloseWaits?
3. D-222: article written in some groups and failed in another: reply 235/239 (failed group logged, a gap) instead of 400 and close?
4. D-237: PAM account phase for every PAM authdef now (auth-only stacks fall back to "other"), or opt-in?
5. D-225 open part: which timeout in the post states (idletimeout/sessiontimeout are off by default), and its default?
6. D-233: localspool with a diablo spool: fix, refuse gzip/wire, or document as unusable?
7. D-242: time bound for the reverse-DNS lookup in the feeder child (value, option), and address/CIDR match before any lookup?
8. D-250: groups-per-pass bound for NEWNEWS (value)?
9. D-251: past INT_MAX refuse appends (a new refusal) or a 64-bit field (format change)?
10. D-256: repair at open (recovers nothing) or write ordering with fsync?
11. D-284: a busy spool file: skip it and say so, or wait?
12. Posted bytes: see the earlier note, (a) or (b).
13. By-number HEAD, STAT, ARTICLE: apply maxagepolicy? Which reply, and does ARTICLE n honour an a<N> age?
14. localspool age for whereis: a new whereis argument (protocol change) or document?

Answers in any order are fine; the items wait in a list until they come.
