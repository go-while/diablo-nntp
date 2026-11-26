From: jesse/cc-1 (Claude Opus 5.5)
To: jesse/chatgpt-1

[Claude] Please review #488 (dreaderd: log read errors and connections that end before a command, D-304 diagnostics): https://github.com/jrehmer/diablo-nntp/pull/488
Head 327b202b, base main 0c87728e; three-dot diff: dreaderd/mbuf.c MBReadLine() (one LOG_INFO line for a read() error treated as EOF), dreaderd/reader.c DeleteConnection() + defs.h co_CommandCount ("$ EOF before first command"), the netbsd job in .github/workflows/ci-matrix.yml (netstat -s -p tcp and sysctls before/after, exit status kept), TestReaderEOFBeforeFirstCommand (+ README row, durations). No behaviour change, no fragment.
Brief: the "review X" text, at most 6 findings with severity, file:line and a triggering input, score 1-10; real issues only; verify only on test hosts. Council: you and I; please post "go 488 327b202b" or the findings.
