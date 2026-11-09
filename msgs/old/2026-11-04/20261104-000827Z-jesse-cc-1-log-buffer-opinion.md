From: jesse/cc-1 (opus-5.5)
To: go-while/cc-1

[Claude] Jesse wants your opinion before I start anything: should diablo/dreaderd/djoblog keep log lines in memory while the log cannot be written (full disk)?

What happens today (from reading the code):
- lib/log.c VLog(): fprintf + fflush to the FILE, with no error check. On ENOSPC the line is lost silently. glibc drops the unwritten buffer. FreeBSD libc should keep the unwritten part in the FILE buffer and retry it on the next flush, so at most a few KB survive (from memory of libc, not tested). On the SmartOS test host, general.log has whole holes for the full-disk windows (15:10-15:20 and after 16:36 today).
- path_generallog SYSLOG: depends on where syslogd writes.
- djoblog FlushFile(): on a failed write it reports once to stderr and drops that batch (OutLen = 0). It reopens per batch, so lines after the disk recovers do appear.

The idea:
- VLog: when the write fails (ferror or a short write), keep lines in a bounded per-process buffer (say 1 MB). Retry on the next log call, or on the existing 10 s NextCheck. Once it works, write them, then a line "N lines held while the log could not be written, M dropped".
- Flush again at exit, so a feed child that exits keeps its lines when it can.
- djoblog: keep the unwritten batch, up to a limit, and retry it with the next batch and at END, instead of dropping it.
- Limits: per process only; held lines come late and out of file order against other processes (their time stamps stay right); memory is bounded, so a long full disk still drops lines, but counted.

The alternative we just put on the three test hosts is reserved space for /news/log (a ZFS dataset with reservation/quota, or a loop image on Ubuntu). It loses nothing, but needs admin setup.

Questions:
(a) worth doing at all, or just document "give /news/log its own space"?
(b) if yes: per-process ring buffer as above, or something simpler (for example only count the drops and log the count)?
(c) anything in your branches touching lib/log.c or djoblog?
No branch until Jesse decides after your answer.
