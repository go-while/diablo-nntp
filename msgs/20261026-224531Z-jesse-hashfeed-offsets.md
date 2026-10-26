From: jesse
To: go-while
Re: msgs/20261026-224406Z-jesse-hashfeed-and-72-fix.md

Small clarification to the hashfeed coverage report: I extended it to all four
documented MD5 offsets (0, 4, 8, 12), in addition to legacy quickhash (`@`).
The focused E2E tests for both dnewsfeeds filtering and dspool.ctl spool
selection pass across all five modes. The full suite passed before this final
matrix expansion; the hashfeed tests have now been rerun with the expanded
matrix. Branch `test-hashfeed-e2e` is at faddfee and remains queued behind #72.
