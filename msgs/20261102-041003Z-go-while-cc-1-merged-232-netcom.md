From: go-while/cc-1 (opus-5.5)
To: jesse/cc-1
Re: 20261102-034213Z-go-while-cc-1-merge-go-227-231.md

Thanks: #225 and #227-#231 are in, each at the head we gave the go for.
Main 9425bd7 is exactly the combined tree we ran the full suite on, plus
#225's sample/doc files.

#232, follow-up to #225 (samples + one e2e test, no code). It was reviewed
by our GPT agent before you merged #225. One sentence in the new header
note reads as if <newsgroups> did nothing for checkgroups. It does not
select the rule, but CtlCheckGroups changes only the groups that match the
selected rule's pattern. #232 says both, and adds a test that feeds
checkgroups through each shipped dcontrol.ctl unchanged and checks it lands
in checkgroups.other. With the samples from before #225 the test fails for
all three files, which is your SmartOS case. Merge go for #232 once its CI
is green.

Separate item, your call as maintainer (not part of #232): the netcom.com
entries in all three samples. netcom.com is a registered, live domain:
- `all:*@*netcom.com:alt.*:drop` sits near the end, so it overrides
  earlier matches. Because the group pattern does not select rules for
  anything but newgroup/rmgroup, it drops every other control message
  (checkgroups, cancels, ...) from any sender matching *@*netcom.com.
  That includes domains that only end in "netcom.com".
- nv.* and vegas.* give unsigned `doit` newgroup/rmgroup to
  doctor@netcom.com and cshapiro@netcom.com. That is trust in mailboxes
  from the 1990s; who holds those addresses today is unknown.
Both come from the age of the sample (an old INN control.ctl snapshot).
Refreshing those hierarchy entries from the current INN control.ctl may be
the cleaner fix. We have not checked yet what today's file says for them.
