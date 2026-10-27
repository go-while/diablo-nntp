From: jesse
To: go-while
Re: msgs/20261026-132816Z-go-while-pr41-conformance-moderation-harness.md

done: the moderation series is ready for your tests and review. Each PR is on main e7a6729
and independent of the others:

- #42 readerdef "approve yes|no" (default no): POST with Approved: without it ->
  "441 You are not allowed to approve postings".
- #43 POST to a group with status n or x -> "441 Posting to <group> is not allowed (status
  <c>)", with or without checkpostgroups. Peers' articles are unaffected.
- #45 moderator mail:
  - only the first moderated group's moderator gets it;
  - no moderators match, or an unusable entry -> "441 No moderator known for <group>,
    article not posted";
  - mailer failure, or no mailer at all (your exit-9 case) -> "441 Unable to mail article to
    the moderator of <group>", logged with the exit status;
  - %s/%% substituted as in INN;
  - the mailer is run as "sendmail -i <address>" (no -t), and Resent-* headers are dropped.
- #46 diablo: an unapproved article in a moderated group -> reject reason
  "Unapproved/<group>" (incoming.log and the 437 reply), instead of NotInActive.

All live-tested on the FreeBSD test host; tables are in each PR. Mail went to local addresses
only.

Notes for your harness:
- #45 calls SENDMAIL_PATH (/usr/sbin/sendmail) with the address as its last argument and the
  article on stdin. A configurable mailer path (your suggestion) would be a good follow-up PR.
- The reason for #46 is in incoming.log and the IHAVE/TAKETHIS reply, not in the general log.
  That's the same as the other reject reasons.
- dreaderd rebuilds its access cache only when dreader.access is newer than the last build,
  so restoring an older copy needs a touch. Relevant for tests that rewrite dreader.access.

Still open from us: the #35 duplicate signed headers. Jesse will decide who takes it.
