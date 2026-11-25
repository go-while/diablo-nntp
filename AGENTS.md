# Guidance for AI agents working on Diablo

Issues, pull requests and comments are read by volunteers.
Your work should save them time, not cost it.
If in doubt, send less, later, and better checked.

The maintainer may decline AI-prepared work at any time; this file does
not ask them to accept it. How the AI-assisted work behind some pull
requests is done is described in [AI-TEAM-PROCESS.md](AI-TEAM-PROCESS.md).

## A person is responsible for everything you send

The person submitting the work must verify its claims, read the final
text and any diff, and take responsibility for the submission. Pull
requests and comments upstream are not sent without a person having read
the exact text. Notes on the agent-chatroom board follow the board's own
rules (REVIEW.md); this paragraph does not change them.

## Before you send anything

- Search first: [ISSUES.md](ISSUES.md) (open and fixed entries), open and
  closed pull requests, `changes/` and recent commits on `main`.
  Do not report what is known or fixed.
- Read the code you talk about, at the commit you name, and point to it
  (file and line).
- Check the documentation and the sample configuration first: many
  problems are configuration.
- Give the configuration, the commands and what happened. Say what you
  ran and what you only read. "By reading" is a fine answer; a claim
  that was neither run nor read is not.
- "Nothing found" is a good result.

## What to report

A defect someone can trigger in normal operation, under a configuration
the documentation calls valid, with a concrete effect: wrong, lost or
duplicated data, a wrong access decision, a crash, a hang, a stuck
connection, memory or CPU without bound, a build failure, or wrong
documentation. Say who triggers it, with what input, and what goes wrong.
Hardening ideas, style and invalid configurations are notes, marked as
such; the maintainer decides.

## Security

Do not report vulnerabilities in issues, pull requests or comments.
Follow [SECURITY.md](SECURITY.md).

## Pull requests

- One concern per pull request. No unrelated reformatting or cleanups.
  Do not send a batch; ask what the maintainer wants and when.
- Start from current `main`.
- A fix comes with a test in `tests/e2e` where possible (say why when
  there is none) that fails without the fix and passes with it (see [tests/e2e/README.md](tests/e2e/README.md): a row
  for the test, and `ci/durations` for a slow one). Say which hosts and
  systems it ran on. Do not lower a threshold to make a flaky test pass.
- Add a short file in `changes/` for a user-visible change.
- Behaviour and defaults are the maintainer's decision; say in the pull
  request which default you chose and why.
- The maintainer asks that the "Last updated" line of ISSUES.md is not
  edited in these pull requests and that each issue gets its D-number;
  credit the reviewer who raised a finding first.
- Do not run builds or tests on a machine you care about; use test
  machines. Keep output and run time of a test bounded.
- Commit message: one `Co-Authored-By` line naming the model that did
  the work (for Claude agents `Co-Authored-By: <model and version>
  <noreply@anthropic.com>`; other agents use their vendor's no-reply
  address). No session links or session ids in commits, pull requests
  or comments.
- No host addresses, local paths or private details in public text.
  Name a test machine by a public, non-sensitive label, not its address
  or private host name, and add the job id of the run. For a hosted CI
  runner use the run id and the runner OS. One example for a virtual
  machine: `(kvm@<host>/<id>-<name>, job N)`.
- Never post under a person's account below that person's own post to
  answer or add to it. A comment answers someone else; new facts go into
  the commit message and the pull request text.

## Writing

Short, plain words, facts the reader can check. One claim per sentence;
every sentence about code or a test has its evidence (file and line, a
test name, a log). If a text was edited after its review, have the
changed part read again.

## If a tool or model refuses

Stop that item and tell the person responsible. Do not reword or split
the task to get past a safety check.
