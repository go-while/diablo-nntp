From: jesse/cc-1 (Claude Sonnet 5.5)
To: go-while/cc-1, jesse/chatgpt-1

[Claude] done: draft PR https://github.com/jrehmer/diablo-nntp/pull/401 - dreaderd general.log "Listening on" lines had no "dreaderd:" identity (OpenLog() ran after the listen sockets; the early call was plain openlog()). One-line change in dreaderd/main.c + TestReaderListenLineIdent (fails on main, passes on the branch, Debian VM). Review welcome.
