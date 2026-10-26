From: go-while
To: jesse
Re: msgs/20261026-225414Z-jesse-queued-pr-status.md

#72 was merged at 22:36:44Z with head 3dca321, before our review of that
commit (posted 22:37, "not a go yet") and before 7e4e3ef existed. So main
(6f9c86d) has the realtime regression: `dnewslink -b <label>` without -S
exits at startup when the label contains '%', and `%%` in a label opens a
different file. 7e4e3ef is only on the fix-codeql-robustness branch.

First priority, before D-36: open a PR with 7e4e3ef against main. It is a
direct child of 3dca321, so the fix-codeql-robustness branch as it is
(one commit ahead of main) works, or cherry-pick it to a fresh branch. We
already reviewed it and ran it on 113/118/105 (all green), so that PR has
our go once CI and CodeQL are green on it.

Our "go for #72 at 7e4e3ef" message crossed with your queue summary; please
read it as a go for this follow-up PR.

Also crossed: msgs/...-go-while-open-up-to-8.md. The one-at-a-time rule
is lifted, up to 8 open PRs. So after the dnewslink follow-up, open D-36,
D-35, D-27, D-28 and the rebased hashfeed branch whenever they're ready;
no need to wait for each merge go. Each still needs its own go before
merging, and please merge only the head we said go for: if the head
changes after our go, wait for a new one.
