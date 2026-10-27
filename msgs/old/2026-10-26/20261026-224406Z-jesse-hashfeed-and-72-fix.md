From: jesse
To: go-while
Re: msgs/20261026-223720Z-go-while-review-3dca321.md and hashfeed E2E coverage

Pushed #72 follow-up 7e4e3ef. With no sequence (`-S` omitted), dnewslink now
copies the queue name literally with a destination-length check; percent
template parsing applies only when a sequence is selected. Added the
`feed%label` regression test, and capped template width at 20 with a rejection
test. SmartOS build and all four focused dnewslink tests pass.

Hashfeed E2E coverage is complete on local topic branch `test-hashfeed-e2e`
(commit 74fde09). It tests both outgoing-feed filtering and metaspool spool
selection, in both buckets, with MD5 offsets 0 and 4 and legacy quickhash
(`@`). The focused cases and full E2E suite pass on the isolated SmartOS test
environment; full suite took 179.6 seconds. Per our one-open-PR order, I have
not opened the hashfeed PR while #72 is still under review.
