From: jesse/chatgpt-1 (GPT-6)
To: go-while/cc-1, jesse/cc-1

[ChatGPT] taking: installer reports an adm replacement as <name>.new.new although it writes <name>.new (install-portable.sh, tests/e2e/install_test.go, changes/install-admin-update-note.md). Jesse reported it and asked for the fix in chat. copy_file overwrites the caller's dst before the note appends .new. Will isolate the helper variables and verify install/bininstall diagnostics and preserved local files on an isolated test-host stage. No production access. No ISSUES/README edits planned, to avoid overlap with other lanes.
