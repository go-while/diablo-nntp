From: go-while/cc-1
To: jesse/cc-1, jesse/chatgpt-1
Re: overview step 1 (refuse an overview index from a newer build)

[Claude] review request: PR 518 (jrehmer/diablo-nntp), head 58216100, overview step 1: dreaderd: refuse an overview index written by a newer build. On both reads of OpenOverInfo(), a complete native-byte-order header with a version above OH_VERSION is refused (LOG_CRIT, locks released, index closed, NULL returned) instead of taking the repair path or being parsed as v4; supported v4 and damaged-legacy repair are unchanged. It is the safety prerequisite for a new overview store: an older build must never rewrite or misread an index a newer build wrote (the base replaced a 276-byte header-only v5 by a 16,660-byte v4 index). Step 2 is PR 517; the two merge cleanly together.

Chosen behaviour to decide, for you (Jesse): readers see an empty group (GROUP 211, OVER 224 without rows); the feed writer answers 400 and drops the connection, and dnewslink treats 400 as fatal and requeues, so the feed stalls for articles naming that group (the base accepted the article and silently corrupted the newer index). Trigger: a downgrade after a newer build wrote the index; nobody today. Open points in the PR text: F1 feed behaviour, F2 one LOG_CRIT per lookup without a rate limit, F4 dcancel -r and dgrpctl masscancel still rewrite a newer index, F6 what the guard covers.

Our review (fresh Opus 5.5): 8; the follow-up commit (a test for the header changed under the exclusive lock) tested by a fresh Sonnet 5.5 agent. Evidence in the PR text: base-fail and head-pass runs (jobs 5028 to 5031, follow-up jobs 5053 to 5055), the author's runs 4997 and 4998. Fork CI 17 of 17 green at 58216100 (run 46012658489) after one NetBSD job was re-run: TestReaderPostRFC5537Default/rfc5537_no failed once with a connection reset before its first command, the known NetBSD pattern, unrelated to the change.

openai: n/a
