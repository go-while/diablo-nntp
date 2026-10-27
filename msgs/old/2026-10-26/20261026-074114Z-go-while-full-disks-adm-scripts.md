From: go-while
To: jesse

# Full disks and the /news/adm scripts

## Why hourly.expire fails
crontab.sample runs `cd adm; ./hourly.expire`, and the script does `cd ~` and then
`dbin/plock ... ~/dbin/dexpire`. That only works when the news user's home directory is
/news. On SmartOS it usually isn't, so dbin/plock isn't there ("no such file or directory").
daily.reader does the same `cd ~`. It's not SmartOS being odd; the scripts always assumed it.
(ISSUES.md D-34.)

## To unblock now, without the scripts (as root)
1. Spool expire. dexpire frees space as dspool.ctl says (minfree, maxsize, keeptime):

       su - news -c 'cd /news && dbin/dexpire -a -v'

   `dexpire -R <spool>:<target>` sets a free-space target for one spool; see `dbin/dexpire -h`.
2. Reader overview, if dreaderd runs there:

       su - news -c 'cd /news/spool/group && /news/dbin/dexpireover -a'

3. Reader article cache, files older than 3 days:

       find /news/spool/cache -type f -mtime +3 -exec rm {} +

Quick fix for the scripts as they are: `usermod -d /news news`, or replace `cd ~` with
`cd /news` in them.

## Proper fix (go-while can take it, say yes here)
Rewrite samples/adm in plain POSIX sh, with no csh/tcsh:
- each script finds its paths with `dbin/dpath home`, so it works whatever the news user's
  home is;
- only commands that exist on SmartOS, FreeBSD and Linux;
- crontab.sample with absolute paths, and without the duplicate biweekly.atrim.dexpire;
- an e2e test that runs hourly.expire against a test site, like TestDexpire.

Together with #21 (adm scripts installed executable in /news/adm), cron maintenance would then
work out of the box.
