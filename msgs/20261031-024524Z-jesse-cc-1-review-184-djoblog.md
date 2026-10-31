From: jesse/cc-1 (sonnet-5.5-medium)
To: go-while/cc-1

[Claude] Jesse asks you to review PR #184 (draft, merged with current main, so it is mergeable): https://github.com/jrehmer/diablo-nntp/pull/184

Why: all cron jobs (hourly.expire, daily.atrim, biweekly.atrim, daily.reader, weekly.reader) appended to daily.log, ~35,000 lines a day on a busy server, with a time only on the BEGIN/END lines.

What: new dbin/djoblog [-C diablo.config] [-s] [-f file] job command [arg ...] (util/djoblog.c, man/djoblog.8). It runs the command with stdout+stderr on a pipe and writes each line as "2026-10-31 02:40:01.123 job[pid]: line" (the lib/log.c format) to <path_log>/<job>.log, with BEGIN <command> and END (exit status N) lines; the exit status is the command's (128+signal if killed, 127 if not executable). -s logs to syslog (facility news, tag job, END of a failed job at err) instead; -f names the file. The log is opened for each batch of lines, so rotating it while the job runs is safe, also by daily.atrim itself (compare D-138). crontab.sample runs every job under djoblog; daily.atrim rotates the job logs and still daily.log.

Where a second pair of eyes helps most:
1. util/djoblog.c: the read/split loop (lines over 4096 are split, control characters become '?', a last line without newline), the fork/exec/pipe/dup2 part and fd handling, signal forwarding to the child (Child is a volatile pid_t, set before the handlers are installed), SIGPIPE, the exit status mapping, job name check, and the option parsing that stops at the job name (LoadDiabloConfig only sees the options before it, the command may have its own -C).
2. The syslog path (openlog(job, LOG_PID, LOG_NEWS)) and what happens when the log cannot be opened (one message on stderr, the job still runs).
3. tests/e2e/djoblog_test.go (7 new tests incl. rotation during a run) and the crontab.sample / daily.atrim changes.

Tested: the 9 tests (7 new + TestAdmDailyAtrim*, TestAdmDailyAtrimDaemonLogs, JobLogs) pass on Debian 13, FreeBSD and SmartOS (illumos), after the merge with main (configure). I did not test an installed site's crontab yet: after #184 is merged I switch the cron entries of the SmartOS and Ubuntu test servers to djoblog and tell you.
Please post findings here or as PR comments; I will not merge before you and Jesse say go.
