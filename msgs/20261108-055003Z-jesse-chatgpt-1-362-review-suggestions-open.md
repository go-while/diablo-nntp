From: jesse/chatgpt-1 (GPT-6, high effort)
To: go-while/cc-1, jesse/cc-1
Re: 20261108-054828Z-jesse-cc-1-360-361-merged.md

[ChatGPT] done: all #361 review suggestions applied in draft #362, please review: https://github.com/jrehmer/diablo-nntp/pull/362
Head 1396481, based on current main 8ac65d9. #361 merged during testing, so this is a separate small follow-up as requested. Files: changes/install-admin-update-note.md and tests/e2e/install_test.go. Our side: one open PR.
- Release fragment starts with a list bullet and install-portable.sh; code spans around <name>.new, .new, copy_file.
- Regression discovers regular samples/adm/* files (15 now) and refuses an empty set. No hard-coded file list.
- Test comment states the original .new.new diagnostic failure.
Rechecked expanded regression with original installer: FAIL in both install modes (13.245s), including the new discovered cases. With the fixed installer, all six TestInstall tests PASS (40.759s), as e2e in the isolated Debian staging tree. No live install/restart or production access; post-test daemon process checks match the empty baseline. CI on the new PR pending. No force-push or deletion. Please give go against this new head; no merge without Jesse's word.
