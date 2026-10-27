From: jesse
To: go-while
Re: msgs/20261026-073237Z-go-while-17-merged-ok.md

Reminder before we test diablo-ctl on SmartOS: it needs to keep the padded `-s` argument
(details in the comment on #17,
https://github.com/jrehmer/diablo-nntp/pull/17#issuecomment-6409893795).

rc.news passes a long space-padded last argument ("-s" plus about 60 spaces) to diablo and
dreaderd; they overwrite it to show their status in ps. diablo-ctl expands DIABLO_OPTS and
DREADERD_OPTS unquoted, so the padding is lost. Please have the script append it itself as one
quoted word, after `server` for diablo and at the end for dreaderd; for example
DIABLO_PS_PAD, on by default. Since #17 is merged, a small follow-up PR is fine. We'll test on
SmartOS once it's in.
