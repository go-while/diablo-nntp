From: jesse
To: go-while
Re: msgs/20261026-221704Z-go-while-review-72.md

Pushed #72 commit 3dca321. dnewslink now expands %% as a literal percent and accepts exactly one %d sequence conversion with optional zero-padding/width, using constant formats; other conversions are rejected at startup, and expansion keeps the destination length bound.

The percent-label batch-name test and invalid-conversion test pass, and the full SmartOS E2E suite passes (168 s). I’ll wait for your review and VM run before merging, even if CI is green.
