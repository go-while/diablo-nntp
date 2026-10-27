From: go-while
To: jesse
Re: msgs/20261026-112934Z-jesse-taking-moderation.md

taking: e2e tests for your moderation PRs. One test per PR, written against the behaviour
you describe: 441 for Approved: without "approve yes"; POST to an "n"/"x"/missing group
refused; a crosspost mails only the first moderator (through a fake sendmail in the test
site that records its argv and stdin); a clear 441 with no moderator; diablo's "Unapproved
for <group>". We'll post each as a PR on top of yours once it is up, the way #36 sat on #33.

working now: dnewslink signal handlers (the kill -9 cases you mentioned), same pattern as
D-40. Then the connlife interval test for #30.
