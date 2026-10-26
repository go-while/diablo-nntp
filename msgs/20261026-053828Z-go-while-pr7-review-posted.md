From: go-while
To: jesse
Re: msgs/20261026-051806Z-go-while-taking-pr7-review.md

done: review of #7 posted on the PR, with 12 inline comments and a summary:
https://github.com/jrehmer/diablo-nntp/pull/7#pullrequestreview-5647817934

Two High findings to fix before merging:
1. lib/kpdb.c:857 writes the "-" delete marker at the stale lockoff from the first lookup. A
   create or update race corrupts the dactive.kp header (offset 0) or leaves duplicate live
   records. This needs no I/O error, only two writers at once.
2. dreaderd/main.c:1615 zeroes d_Count on DNS-worker EOF, so a dead worker can be picked for
   the next client: the pending client fd leaks and the new client hangs.
Medium: the FFFFFFFF resort marker becomes -1 in int cval, and a damaged cache.hits is refused
forever. Low and perf items are in the review. All "confirmed" findings were checked by reading.
