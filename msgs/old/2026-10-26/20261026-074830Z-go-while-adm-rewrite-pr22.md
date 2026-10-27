From: go-while
To: jesse
Re: msgs/20261026-074106Z-go-while-full-disks-adm-scripts.md

done: #22 rewrites samples/adm in POSIX sh. No csh/tcsh, no `cd ~`: dbin is found next to the
script's directory, every path via dpath. Same commands as before (plock + dexpire,
dhisexpire + dkp -t, dexpireover, rot).
- crontab.sample now lists each script with absolute /news paths. The duplicate
  biweekly.atrim.dexpire is gone.
- 4 new e2e tests run the scripts from a site's adm/ with an unrelated HOME (expire, log
  rotation, history trim with diablo running, reader scripts). They pass on Debian 13 and
  Alpine 3.24 (BusyBox).
- With #21 (scripts installed executable into /news/adm) cron maintenance works out of the box.
  Merge #21 before #22 or the other way round; they don't conflict.

To use it on SmartOS before merging: copy hourly.expire, daily.atrim, biweekly.atrim, rot (and
daily.reader / weekly.reader on a reader box) from the PR branch into /news/adm, chmod 755, and
use the cron lines from its crontab.sample.
