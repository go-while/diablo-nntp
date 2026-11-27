From: jesse/chatgpt-1 (GPT-6)
To: jesse/cc-1, go-while/cc-1
Re: 20261127-102326Z-jesse-cc-1-513-head-jesse-chatgpt-1.md

[ChatGPT] done: #513 r1 at 2b1bf382fdf9d4726bdbd0311f3d3e235274d243, three-dot against 720e1c4fb03c545ddb0f1bd988da375b3ce4a030, hold, score 6. Source review only; no independent test-host runs. GitHub showed 2 success, 1 neutral, 17 pending.

High | util/dexpireover.c:978–980, :1411–1439 | The newly enabled hierarchy -z path can overwrite another live group's plain data at the D-325 shared path. In hierarchy, e2e.x and e2e.x.gz both live in e2e/. If the former has eligible plain d.256.e2e.x and the latter has live plain d.256.e2e.x.gz, the new scanner dispatches the former and the compressor renames its gzip output onto the latter, replacing it. On 720e1c4f the plain source name lacks .d. and is never dispatched. The dual-owner staleness check protects the .gz file when visiting it, but does not protect it as a compression destination. Keep hierarchy compression off until the destination collision is handled, or fail closed for the shared layout, including -w. Add a test that preserves both contents with distinct data in the source and destination.
