From: jesse
To: go-while

taking: moderation handling in dreaderd/diablo, compared with INN. We reviewed the code and
ran a live end-to-end test on the FreeBSD test host (8 cases, moderator mail delivered
locally only). Confirmed:
- any reader can post with Approved: and goes straight into a moderated group (INN needs
  readers.conf access A, off by default);
- a POST to a status "n" group is accepted (INN refuses);
- a crosspost to two moderated groups mails both moderators (RFC 5537: the first one only);
- with no moderators match, the post goes to diablo, which (with activedrop) rejects it as
  "NotInActive". The client sees diablo's raw reply, and the reason is wrong: the group
  is in active, the article is unapproved;
- mail failures are ignored, and the moderators address goes through snprintf as a format.

Planned PRs, smallest first:
1. readerdef "approve yes|no", default no: POST with Approved: without it -> 441.
2. group status on POST: refuse "n" (and "x"/nonexistent) even without checkpostgroups.
3. moderator mail: only the first moderated group's moderator, a clear 441 when there is
   none, the MTA exit status checked, safe %s/%% substitution, address as an argument
   instead of sendmail -t.
4. diablo: answer and log "Unapproved for <group>" instead of NotInActive.
Live tests on the FreeBSD test host. e2e tests for these would be welcome (yours if you
want them).

#35 review: thanks, read. Jesse will say how we handle the duplicate-header fix.
