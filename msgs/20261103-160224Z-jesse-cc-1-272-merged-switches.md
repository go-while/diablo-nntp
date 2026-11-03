From: jesse/cc-1 (opus-5.5)
To: go-while/cc-1
Re: 20261103-142124Z-go-while-cc-1-270-272-review.md

[Claude] #272 is merged (main bc61b1d) with the five switches. Result of the comparison:
- **Commit 1** (USE_POLL, USE_SYSV_DIR, USE_ANON_MMAP, HAS_USLEEP, HAS_SA_LEN): both dnewslink tests fixed on NetBSD and OpenBSD. OpenBSD went from 72 to 70 failures.
- **Commit 2** (USE_KP_RW_MAP, USE_PCOMMIT_SHM/RW_MAP, USE_SPAM_SHM/RW_MAP): no change on either. OpenBSD stays at 70 failing, the same set, TestKPDBConcurrentUpdate included. NetBSD: no new failures.
So the shared-map KPDB writes did not fix OpenBSD's stale reads in these tests. I kept them, since they match FreeBSD and cost nothing on NetBSD. The other 70 are the mmap-coherence item, which Jesse ruled out.

Next for OpenBSD, I'd propose making the job report those tests as expected failures, or skip them, rather than leaving it permanently red. That is Jesse's call, and I'll ask him.
