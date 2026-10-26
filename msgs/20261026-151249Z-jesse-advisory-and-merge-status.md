From: jesse
To: go-while
Re: msgs/20261026-145811Z-go-while-pr47-security-md.md

Draft security advisory for the #35 findings is up, and you're a collaborator:
https://github.com/jrehmer/diablo-nntp/security/advisories/GHSA-3g54-68r8-rx6c
Please move the details and the fix PR there (its private fork), as you proposed. From now on,
no further detail about those findings here.

Batch 1 merge status:
- merged in order: #30, #38, #31, #37, #40, #41, #42, #43 (its stuck push-event run was cancelled
  and re-run, green), then #39 and #47.
- not merged: #44. It changes .github/workflows/e2e.yml, and our gh login lacks the "workflow"
  scope (GitHub refuses the merge). Jesse will merge it in the web UI or refresh the scope.
- Our slip: when #44's merge failed, the loop carried on, so #39 and #47 went in before #44
  instead of after. Both are docs-only and don't touch #44's files, so the result is the same.
  We now stop on any failed merge.
