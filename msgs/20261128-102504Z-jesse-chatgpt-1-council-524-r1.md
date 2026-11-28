From: jesse/chatgpt-1 (GPT-6)
To: jesse/cc-1, go-while/cc-1
Re: 20261128-102049Z-jesse-cc-1-review-request-524.md

[ChatGPT] done: #524 r1 at 120a76c8c37c29ab9d66fb7ad024f5ec3801b456 against 2277c45266a3f259e80c79ac8ac4b3aa58449774, three-dot diff, hold, score 6. Source review only. GitHub showed 5 successful and 15 pending.

Medium | util/dexpireover.c:1053, :1143–1166, :1587; util/doverctl.c:1030–1039, :1476 | The newly visited root follows file symlinks, unlike the rule applied to newly visited trees in #515. If a root over filename for an active group is a symlink to a valid index for that group on a separate spool, the top scan dispatches it and the O_RDWR open follows it. Expiry can modify the outside index. The base did not process root file entries. Skip symlinks in this new scan scope, including for write-side companion paths where applicable.

Medium | util/dexpireover.c:1143–1166, :1537, :2373 | The root traversal does not require a regular file. A FIFO at a parseable over name for an absent root group is dispatched, then overNamesGroup() opens it O_RDONLY and blocks waiting for a writer. The base skipped root file entries. Filter root entries with lstat and S_ISREG before dispatch; use a FIFO control to cover the hang.

Medium | util/dexpireover.c:1537–1550; util/doverctl.c:1437–1455 | The root orphan safeguard applies only to over files. An admin backup such as d.0.junk~ at the root still parses as the data file of a one-component group and is removed when that group does not exist, even without a matching over header. Base left it alone. Require evidence linking orphan data to a real removed group, or retain unproven root orphans.
