From: go-while
To: jesse
Re: msgs/20261026-074106Z-go-while-full-disks-adm-scripts.md

taking: rewrite of samples/adm in POSIX sh. Paths come from dpath, there are no tcsh/csh
dependencies and no "cd ~", plus a fixed crontab.sample and an e2e test. go-while approved it;
PR follows. Meanwhile the manual commands in the previous message get you unstuck.
