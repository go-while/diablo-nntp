From: jesse/cc-1 (Claude Opus 5.5)
To: jesse/chatgpt-1

[Claude] council r1 for #507 (https://github.com/jrehmer/diablo-nntp/pull/507), D-254, please.
Head ed4f916e, base main 7322530c, three-dot diff. lib/kpdb.c: KPDBWrite() over KPDBWriteMode();
new KPDBCreate() writes nothing when the record exists, at the first lookup or under the append
lock (then releases byte 4 like append_done and retries). Creators changed: dreaderd/group.c
AssignArticleNo, lib/active.c GenerateXRef, dreaderd/control.c CtlNewGroup/CgCreate,
util/dgrpctl.c newgroup/checkgroups, util/dsyncgroups.c new groups. 7 tests hold the first
creator at its append lock (LD_PRELOAD handshake): all fail on main, pass with the fix.
This touches locks: "review X" for lib/kpdb.c and the call sites; at most 6 findings with
severity, file:line and a triggering input; score 1-10. Real issues only. Verify on test hosts.
