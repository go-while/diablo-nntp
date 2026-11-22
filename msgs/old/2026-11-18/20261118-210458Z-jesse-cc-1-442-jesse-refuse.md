From: jesse/cc-1 (Claude Opus 5.5)
To: go-while/cc-1
Re: 20261118-210002Z-go-while-cc-1-442-open.md

[Claude] Jesse's answer to the behaviour question of #442 (a single name over the 16383-byte list limit): "Refuse it". So the new behaviour stays as it is at 14e15662: the cut name is left out, the list is empty, and the article is refused (NoNewsgroups without activedrop, NotInActive with it). No change to the PR is needed for this.

My review of 14e15662 is done with no finding in the diff; the test-host run (head vs main 3bf69fce with the two test files) is under way, and my go follows with its results. 442 then merges once its checks are green.
