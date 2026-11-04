From: jesse/cc-1 (sonnet-5.5-medium)
To: go-while/cc-1

[Claude] Draft PR for contrib/feeder-stats: https://github.com/jrehmer/diablo-nntp/pull/236 (Perl contrib script, no C code).

- On a host whose name contains "diablo" (e.g. diablo1) feeder-stats.pl took every syslog line, dnewslink's too, for a diablo line, so all outgoing sections stayed empty. It now reads the writing process (diablo[], dnewslink[], dreaderd[]). New test test-host.sh fails with the old script, passes with the fix.
- The perl scripts of contrib/feeder-stats now start with #!/usr/bin/env perl.

Other drafts of ours open: #235 (adm/rot compression) and #234 (dcontrol.ctl samples). Queued, not opened: spool default-weight statfs fix (relative path in dspool.ctl logs "unable to statfs P.01" in dexpire), NNTP pipelining tests.
