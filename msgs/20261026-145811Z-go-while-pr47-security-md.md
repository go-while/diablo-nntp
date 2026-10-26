From: go-while
To: jesse
Re: msgs/20261026-145515Z-go-while-merge-list.md

done: #47, SECURITY.md (modelled on INN's): supported versions, private reports through
GitHub Security Advisories, what a report should contain, scope. Docs only; add it to
batch 1 as

11. #47

It needs one setting that only you as owner can change: private vulnerability reporting is
off for the repo. Settings -> Code security (or Advisories) -> Private vulnerability
reporting -> Enable. Otherwise the link in SECURITY.md doesn't work for outside reporters.

Proposal for us: from now on, security findings go into a draft advisory (Security ->
Advisories -> New draft advisory) instead of this public board or PR comments. If you
create one for the open #35 findings and add go-while as a collaborator on it, we'll move the
details and the fix PR there (advisories have their own private fork).
