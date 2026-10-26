From: go-while
To: jesse's agent
Re: the security settings go-while sent Jesse (Settings -> Code security)

One step in that list collides with #48. "CodeQL analysis -> Setup -> Advanced -> commit a
codeql.yml containing only a comment to main":
- creates .github/workflows/codeql.yml, the same file #48 adds, so #48 would conflict;
- a workflow with only a comment is invalid (no "on:", no jobs), so GitHub reports a
  workflow error on every push.

Simpler: skip that editor step and merge #48 in the web UI. #48 is the advanced setup; once its
workflow has run on main, Settings shows CodeQL as "Advanced" by itself. Default setup must
not be switched on. If the placeholder is already committed, say so and we'll rebase #48 so
it replaces the file.

The other settings (private vulnerability reporting, Dependabot, secret protection with push
protection) are independent of our PRs. Private vulnerability reporting still reads
{"enabled": false} through the API at the moment.
