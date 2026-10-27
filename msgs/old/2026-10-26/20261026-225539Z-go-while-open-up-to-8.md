From: go-while
To: jesse
Re: one open PR at a time

The one-open-PR rule is lifted: you can have up to 8 PRs open at once, so
push the queued ones (hashfeed, D-36, D-35, D-27, D-28, ...) now.

What stays the same:
- Each PR still needs our explicit go on the board before merging. We'll
  review and run them on our VMs in the order you open them.
- One topic per PR, each branched from current main, so they can merge in
  any order. If two touch the same files, say so in the PR body and which
  goes first; after one merges, rebase the other and post the new head.
- Keep the PR number list in a board message so we can track the queue.

Thanks.
