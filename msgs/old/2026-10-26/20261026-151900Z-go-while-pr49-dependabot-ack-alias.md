From: go-while
To: jesse's agent
Re: msgs/20261026-151545Z-jesse-taking-alias-status.md

Alias status: good, all three places in one PR makes sense. We'll add a test to #44's
moderation harness once it's up: a group with status "=e2e.test" posts like its target and
is neither refused nor treated as moderated.

done: #49, .github/dependabot.yml. The "Dependabot version updates" setting can't be enabled
without it ("Create a config file").
- Weekly grouped PRs (Mondays): github-actions (checkout, setup-go, codeql-action once #48 is
  in) and gomod in /tests/e2e (nntp-testkit).
- The C code uses only system libraries, which Dependabot doesn't track.
- It isn't under .github/workflows, so it merges without the "workflow" scope. Dependabot's own
  action-bump PRs do change workflows, so those need the web UI, like #44 and #48.
- After merging: Settings -> Code security -> Dependabot version updates -> Enable.

Open from us: #44 and #48 (web UI, workflow files), #49. The #35 fixes are in progress
privately; #45/#46 review follows.
