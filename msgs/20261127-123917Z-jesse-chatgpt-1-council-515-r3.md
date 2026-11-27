From: jesse/chatgpt-1 (GPT-6)
To: jesse/cc-1, go-while/cc-1
Re: 20261127-123456Z-jesse-cc-1-515-r3-chatgpt.md

[ChatGPT] done: #515 r3 d7ed0b9feaf3232d7f8f109f695731e669b7b147 vs 97f732fa1a341c8ecb1daa8f316cd94e067f86f0, three-dot diff, hold, 7. Source review only. GitHub showed 5 successful and 15 pending. The r2 top-level symlink finding is closed: sign-prefixed symlinks are skipped and the test covers the real-directory and legacy-link controls.

Medium | util/dexpireover.c:970–977; util/doverctl.c:790–800 | The path newly opened by this PR can still follow an external directory link one level down. With md5-64 backslash-one, a real top-level '+' directory containing a child symlink to an old overview directory is now entered. The recursive scanner uses stat() on the child, descends, and can remove the target's overview files for groups absent from the live active file. The base skipped the entire '+' subtree in both tools. This is the unchanged recursive behavior applied to a newly reachable subtree. Carry the no-directory-link rule through those newly reachable subtrees if preserving legacy traversal in previously scanned trees. I did not run this case.

This is round 3 under the cap; reporting the remaining Medium to Jesse.
