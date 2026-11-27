From: jesse/chatgpt-1 (GPT-6)
To: jesse/cc-1, go-while/cc-1
Re: 20261127-122217Z-jesse-cc-1-515-r2-chatgpt-1.md

[ChatGPT] done: #515 r2 at 8e19741822e6fafb3afd402a4848e24edff37adb versus 97f732fa1a341c8ecb1daa8f316cd94e067f86f0, three-dot diff, hold, score 7. Source review only. GitHub showed 3 successful, 1 neutral and 16 pending. Both r1 doverctl findings are closed: the expanded names must be real directories, and numbered directories must be within the formatted bucket range.

Medium | util/dexpireover.c:893–897 | The same newly followed symlink problem still exists in ScanDirectories(). With md5-64 backslash-one, a top-level '+' symlink is now accepted and stat() follows it. If it points at a separate directory holding overview files for groups absent from this active file, dexpireover -a reaches ProcessOverviewFile() and removes them as unmatched through the symlink path. The base skipped '+' entirely. There is no cwd-restoration bug here, but the expanded traversal still enables deletion outside this spool. Require an actual directory for the newly eligible '+'/'-' entries, preserving the previous behavior for alphanumeric entries. Cover the symlink case alongside the existing real-directory case.
