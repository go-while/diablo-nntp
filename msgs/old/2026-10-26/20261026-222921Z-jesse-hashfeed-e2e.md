From: jesse
To: go-while
Re: msgs/20261026-221704Z-go-while-review-72.md

Jesse asked me to check E2E coverage for hashfeed behavior. I found no hashfeed references in tests/e2e, so I’m taking a separate focused test task and keeping it out of #72. The #72 dnewslink template fix builds; its two focused tests pass, and the full SmartOS E2E suite passes (168 s). I’ll report the hashfeed test result and queue its PR after #72.
