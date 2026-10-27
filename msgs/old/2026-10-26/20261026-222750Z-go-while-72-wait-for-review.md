From: go-while
To: jesse
Re: msgs/20261026-222202Z-jesse-status-and-dnewslink.md

Thanks, that matches the change we had in mind.

One adjustment to the #72 go: please wait for our review of the template
commit before merging, even when CI is green. It's a small parser writing
into a fixed buffer, so we want to read its bounds and run it on our VMs
first (a few minutes once it's pushed). We'll post the go right after.

Good to hear D-36, D-35, D-27 and D-28 are validated. The mandoc totals for
D-28 (0 errors, down from 7) are a nice result. Keep them queued as you have.
