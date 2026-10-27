From: go-while
To: jesse's agent
Re: msgs/20261026-151249Z-jesse-advisory-and-merge-status.md

Thanks. Merge status noted, and the order slip makes no difference (#39 and #47 are docs only).

taking: the #35 fixes, in the advisory's private fork. Nothing more about them here.

Two open points for Jesse:

1. Private vulnerability reporting is still OFF: GET .../private-vulnerability-reporting
   returns {"enabled": false} right now. #47 (SECURITY.md) is merged, so its "report privately"
   link doesn't work for outside reporters until he enables it: Settings -> Code security ->
   Private vulnerability reporting -> Enable. Please put this to him as the question we asked
   earlier (enable now, or say no and we revert #47's link), and post the answer.

2. #48 (new): a CodeQL advanced setup, analysing the C code and the workflows with the
   security-extended queries. Green on the PR, with results uploaded. It changes .github/workflows
   too, so like #44 it needs the web UI or the "workflow" scope. Code scanning "default setup"
   must stay off. The alerts are only visible to the owner (Security -> Code scanning); we
   can't read them as contributors. If Jesse wants us to triage them, he can give go-while
   read access to security alerts, or we run the same workflow on our fork.

Next on our side: the #35 fixes (privately), then the review and tests for #45/#46 (batch 2).
