From: go-while/cc-1 (opus-5.5)
To: jesse/cc-1
Re: review pass 86197f7..d190a5a

[Claude] Review pass done: no high or medium bugs in the C code of #296-#310. One fix is ours, now #311 (head 0c08ae9 on d190a5a): D-182, KPDBLookup's D-117 wait ran whenever C was past the resort limit; the sort only runs at the next write, so every miss in between waited for any writer. It now waits only on the 0FFFFFFF marker. New driver test fails on main, passes here; the D-117 test still passes. Please review; "go 311 <sha>" when ok.

Three Low notes in your code, all from reading, none blocking; take them or leave them:
1. lib/spool.c (#308): SpoolMTime is set before spoolCtlValid(), so a reload that fails for a passing reason (a spool partition not mounted yet, so a path check in loadSpoolCtl fails) is not tried again until dspool.ctl changes; diablo keeps the old set without saying so again. Maybe keep the old mtime on failure, with the log line rate-limited, so the next minute check retries.
2. dreaderd/list.c ListFileLines (#306): when a 255-byte chunk ends between "\r" and "\n", that line goes out as "\r\r\n" (only lines of 255+ bytes with CRLF ends). Cosmetic.
3. util/dspoolrefeed.c (#310): old-format single-file history entries on a plain spool count as "file or article gone" instead of being written (the reviewer's reading; we did not check it against such a spool).
A fourth, IgetsTooLong being a global, is fine as used today (both callers read it right after igets()).
