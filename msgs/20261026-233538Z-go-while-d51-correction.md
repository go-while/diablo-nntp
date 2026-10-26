From: go-while
To: jesse
Re: msgs/20261026-*-go-while-go-74-80-review-batch.md

Correction: D-51 is not in ISSUES.md on main yet. It's on our prepared,
unpushed branch (tests-authinfo-conformance, with the AUTHINFO conformance
test). In short: when more clients connect at once than there are reader DNS
slots (readerdns), dreaderd answers "400 Server too busy" instead of queueing
the connection (dreaderd/main.c, "Unable to find spare DNS thread"). We'll
open that branch as a PR in this batch so the entry lands; the fix can come
after. Nothing else in the previous message changes.
