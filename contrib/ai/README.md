# contrib/ai: helpers for AI-assisted maintenance

Small POSIX sh scripts for the repetitive parts of reviewing and testing
Diablo with AI coding agents: steps that are always the same and are cheaper
and more reliable as a script than as an agent's work. They only read
(git, `gh`, logs) and print a short summary, so an agent spends a few lines
of context instead of a full log or listing.

| Script | What it prints |
|---|---|
| `suite-summary LOG...` | one line per e2e suite log (`tests/e2e/ci/build-on-host` plus `go test -v`): system, build, warnings/errors, top-level pass/fail/skip, exit status, failing tests. Exit 1 if any log is not clean |
| `pr-status [-a] OWNER/REPO` | one line per open pull request: head commit, mergeable, check counts and the names of failing checks |
| `board-new [-n] [-p PATH] DIR` | the messages added to a git-branch message board (one file per message) since the last run, with their From/To/Re lines |
| `adv-forks [-f] OWNER/REPO [GITDIR]` | every open security advisory with its private fork's branches, and whether each commit is in the local clone; `-f` fetches the missing ones |

Each script prints its usage with no arguments. They need `git`; `pr-status`
and `adv-forks` also need an authenticated `gh` and `jq`.

`adv-forks` prints advisory IDs that may still be embargoed: keep its output
out of public places (issues, pull requests, public chat).

## Splitting the work

What worked for us, as a guide rather than a rule:

- **Scripts** (this directory): fixed sequences: what is new, what is
  missing, one line per log.
- **A cheaper sub-agent** for collecting and comparing: "is finding X
  already fixed or tracked" across branches and notes; reading large
  outputs and returning a few lines; comparing CodeQL alerts or test counts
  between two heads; fact sheets with `file:line` and the quoted code line.
  Check its lists before relying on them.
- **The main model or a human** for judgment: security impact, severity,
  final reviews, anything posted, merge decisions.
