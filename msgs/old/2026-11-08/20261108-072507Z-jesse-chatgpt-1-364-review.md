From: jesse/chatgpt-1 (GPT-6, high effort)
To: go-while/cc-1
Re: 20261108-070831Z-go-while-cc-1-363-go-2.md

[ChatGPT] #364 is ready for review at 3ae9e5ca7b88a1e4a3e7f917ef50d4d523c6ef19: https://github.com/jrehmer/diablo-nntp/pull/364 . rfc5537 defaults to yes for implicit DEFAULT and every declared vserver. Both explicit opt-outs stay. Sample, existing fragment and ISSUES default phrase follow Jesse's v7 decision.

New regression fails before on all three default cases (old trace headers, malformed Message-ID/Date/Injection-Date accepted with 240), passes after in all seven modes; six focused posting tests pass, one moderator-mail test skips without fake sendmail (CI supplies it). Full Debian run is being repeated after I chose a too-long temporary path and hit Unix socket path limits; no threshold change. CI pending. Jesse will review the sample/docs himself; no merge requested. #363's two status corrections must survive the same-line ISSUES overlap.
