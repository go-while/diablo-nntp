From: jesse/cc-1 (opus-5.5)
To: go-while/cc-1

[Claude] taking: D-200 (new): checkgroups creates and removes groups that the newgroup and rmgroup lines do not allow. Jesse wants checkgroups to follow those lines: no group created or removed outside them.
- dreaderd/control.c: CgCheck() makes an addition or moderated-flag change only where a newgroup of that group from the sender would be executed, and a removal only where an rmgroup would. "Would be executed" means the last newgroup/rmgroup/"all" line matching the sender and group (break ends the scan) is doit, or verify-<key> with the key the checkgroups line was verified with. Changes not made are logged as dgrpctl commands; descriptions are updated as before. New: CgReadGrpRules(), CgGrpAllowed().
- INN's controlchan does not make this check; this is a deliberate difference, noted in the docs.
- All three dcontrol.ctl samples already pair every checkgroups doit/verify line with allowing newgroup/rmgroup lines (225 each, checked by script), so they are unaffected.
- Files: dreaderd/control.c, samples/dcontrol.ctl (7 lines added to the checkgroups notes, nothing reworded), tests/e2e/d200_checkgroups_newgroup_rules_test.go (new), tests/e2e/checkgroups_{docheck,rules,ctl}_test.go (dcontrol.ctl fixtures get newgroup/rmgroup doit lines), tests/e2e/README.md, ISSUES.md, changes/d200-checkgroups-newgroup-rmgroup.md.
- Touches samples/, so it needs Jesse's review as well.
