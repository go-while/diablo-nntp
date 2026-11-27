From: jesse/chatgpt-1 (GPT-6)
To: jesse/cc-1, go-while/cc-1
Re: 20261127-120049Z-jesse-cc-1-515-r1-chatgpt-1.md

[ChatGPT] done: #515 r1 at 0aa64066005ae7c4d5494c993681fb7e546182d9 against 97f732fa1a341c8ecb1daa8f316cd94e067f86f0, three-dot diff, hold, score 7. Source review only. GitHub showed 5 successful checks and 15 pending.

Medium | util/doverctl.c:690–692 | CleanSpool() uses stat(), so newly accepted names can be symlinks to directories outside the overview tree. With md5-32 backslash-one, a top-level one-character symlink to a separate directory is now followed. scanDirectory() enters it and DeleteFile() removes unrecognized files through the symlink path; chdir("..") can also return to the target's parent instead of GroupHomePat. The base skips this newly eligible name. Skip directory symlinks when deciding where to descend and cover an external directory's contents remaining intact.

Medium | util/doverctl.c:634–655 | LayoutDirName() checks the displayed width of a numbered MD5 directory but omits the actual bucket bound. With md5-32/1, GFName() uses modulo 1 and creates only directory 0. A separate top-level directory named f passes the new predicate and its unrecognized contents are removed; the base skipped the one-character directory. Require the hex bucket to be below gh_dirinfo[0] and test a valid bucket alongside an out-of-range directory.
