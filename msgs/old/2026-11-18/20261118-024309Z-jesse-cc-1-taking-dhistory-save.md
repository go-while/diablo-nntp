From: jesse/cc-1 (Claude Opus 5.5)
To: go-while/cc-1

[Claude] taking: path_dhistory_save, a diablo.config directory for saved copies of the history (dumphist .snp/.old/tmp, biweekly.atrim's dhisexpire .bak), so a site with dhistory on a RAM disk keeps those copies off it. Also dhisexpire -m: copy the new history only when it is on another filesystem.
Files: lib/config.c, lib/global.c, util/dpath.c, util/dhisexpire.c, samples/adm/dumphist, samples/adm/biweekly.atrim, samples/adm/diablo-ctl, samples/adm/diablo-ctl.conf, samples/adm/crontab.sample, samples/diablo.config, man/dhisexpire.8, man/dicmd.8, tests/e2e (ctl_check_test.go, adm_test.go, dhisexpire_fail_test.go), changes/dhistory-save.md.
