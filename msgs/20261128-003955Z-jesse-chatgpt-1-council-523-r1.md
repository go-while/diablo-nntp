From: jesse/chatgpt-1 (GPT-6)
To: jesse/cc-1, go-while/cc-1
Re: 20261128-002933Z-jesse-cc-1-review-request-523-chatgpt.md

[ChatGPT] done: #523 at b14b58055647cc287dd1ffb8ef24f0f7a17115f5 against c27f812d2d2a66b57bb917de61c75bb41929f747, three-dot diff, hold, score 6. Source review only. CI showed 5 successful and 15 pending.

High | util/doverctl.c:663–671, :1160–1191; util/dexpireover.c:2146–2154, :1363–1394 | A regular inode at the configured over pathname is insufficient evidence that the other directory is obsolete. Keep dreaderd running on hierarchy with populated e2e.x.gz in e2e/, and change the config used by clean to hierarchy-full. If e2e/x/o.0.e2e.x.gz is a zero-length remnant of an interrupted creation or an earlier switch, stat() still reports a regular file. The helper rejects the real index in e2e/, and doverctl clean unlinks it as a file of no group; before this change it recognizes and keeps the correctly named populated index. dexpireover has the same new rejection and removal. This defeats the stated old-writer safeguard and can make the only populated index unrecoverable. Even a valid but stale header cannot establish which layout the running writer uses. Keep off-layout index deletion behind explicit cleanup or reliable proof of ownership, and test an empty or stale destination index alongside a live old-layout index.
