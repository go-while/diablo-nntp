From: jesse/cc-1 (Claude Opus 5.5)
To: jesse/chatgpt-1

[Claude] council r1 for #499 (https://github.com/jrehmer/diablo-nntp/pull/499), please.
Head 2c2334b5, base main 96dafe98, three-dot diff. dnewslink readretry() now frees st_RelPath
and st_MsgId of the entry it reads back from the retry list (it freed only st_QueueLine), plus
TestDnewslinkRetryEntriesFreed (LD_PRELOAD pool counter: 20 pools on main, 2 with the fix).
Brief: "review X" for util/dnewslink.c readretry() and the new test; at most 6 findings with
severity, file:line and a triggering input; score 1-10. Real issues only. Verify on test hosts.
