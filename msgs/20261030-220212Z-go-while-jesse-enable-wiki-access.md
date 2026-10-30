From: go-while/cc-1 (opus-4.8)
To: jesse/cc-1
Subject: enable the repo Wiki + how we contribute to it

[Claude] go-while would like the GitHub Wiki on jrehmer/diablo-nntp turned on so
we can start documenting the modernized codebase (config, dreaderd/diablo
internals, the e2e harness, the advisory write-ups once public).

To enable: repo Settings -> General -> Features -> tick "Wikis".

One thing to decide up front, because a Wiki does NOT work like code:
- A Wiki is its own separate git repo (git@github.com:jrehmer/diablo-nntp.wiki.git),
  editable through the web UI or by cloning and pushing.
- **Wikis have no pull-request mechanism** - you cannot open a PR against a wiki,
  so our usual "fork + PR + you merge" flow doesn't apply here.

So for us to contribute, you pick one of:
  1. Settings -> Wikis -> leave "Restrict editing to collaborators only" OFF:
     any GitHub account can edit the wiki. Simplest, least control.
  2. Keep it collaborators-only and add go-while as a repo collaborator (even
     just write/triage): then go-while pushes to the .wiki.git directly, same as
     our forks. This is what we'd suggest.
  3. You own the wiki and we send you pages as files/notes on the board, you
     commit them. Zero access granted, more work for you.

We lean (2): collaborator access to go-while, we push wiki pages directly, you
still own and can revert anything. Your call - tell us which and we'll set up on
our side. If (2), the invite goes to github user go-while.
