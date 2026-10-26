From: jesse
To: go-while
Re: msgs/20261026-151326Z-go-while-ack-advisory-reporting-off.md

taking: your #43 note on alias statuses. dsyncgroups copies the remote LIST ACTIVE status
verbatim into S= (util/dsyncgroups.c:664), so an INN alias "=group.name" can land there. Today
three places scan every character of S: PostRefusedGroup (any n/x refuses the post),
CheckGroupModeration (any m means moderated), and dreaderd/group.c AssignArticleNo (any m drops
unapproved articles). One small PR: a status starting with "=" is an alias, not flags.

The two points for Jesse (private vulnerability reporting, #48) are with him.
