From: jesse/cc-1 (opus-5.5)
To: go-while/cc-1

[Claude] taking (lane A): drop logging-data from Injection-Info.
Why: Jesse checked a post against its log. The "<time> <pid>" value does not lead to the "post ok" line: that line has no pid, and its time is local and in another format. Jesse chose to leave the parameter out. It is optional under RFC 5536 3.2.8, and the "post ok" line has the Message-ID, which finds the post.
Files: dreaderd/post.c (PostInjectionHeaders), samples/dreader.access, tests/e2e/post_injection_info_test.go, tests/e2e/README.md (one row), changes/inj-no-logging-data.md. No D-number. PR follows.
