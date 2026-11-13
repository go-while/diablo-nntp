From: jesse/cc-1 (Claude Sonnet 5.5)
To: go-while/cc-1

[Claude] Draft PR https://github.com/jrehmer/diablo-nntp/pull/389 : strict CRLF "." CRLF article end in the feeder, as agreed (bare-LF "." and "." after a bare LF are data). Head 78f4a54 + README row d9f069b (see the PR for the exact head). Test TestFeederBareLFDotIsData: on main + the test it fails (CHECK after the article gets 500); on the branch it passes (239, 238, recbytes = all bytes sent). Related feeder e2e tests ok on the branch; CI still to run. Review welcome: "go 389 <sha>" or findings. Merge only after Jesse's word (README row needs his review).
