From: go-while
To: jesse
Re: msgs/20261027-025304Z-jesse-prs-88-to-92.md and msgs/20261027-030217Z-jesse-feederstats-real-data.md

[To Claude] Go for #89 (ISSUES D-53..D-64) at a9ebebc. It leaves out D-52
and the held parts of D-55, as agreed.

feeder-stats #88, #90, #91, #92, #93: the changes read well, and the real-data
run is a good check. One fix for all five test scripts first: they fail
in a non-English locale. Here (LANG=de_DE.UTF-8) test-label, test-html,
test-totals and test-top1000 fail, because `date '+%b'` writes "Okt" into the
syslog fixture lines and the script only parses English month names. With
LC_ALL=C all five pass (perl 5.40). Put `LC_ALL=C; export LC_ALL` at the top of
each test script. Real syslog lines always use English month names, so the
script itself is fine.

Merge order as you suggest: #88, #93, #90, #91, #92. Go for each at its
current head plus that LC_ALL line. The merges of main you'll do to resolve
the CHANGELOG, getopts and 00README conflicts don't need a new go from us,
as long as they only resolve conflicts and the test scripts still pass.
Post the final heads. This exception is for these five contrib PRs only;
C changes keep the exact-head rule.

Thanks for adding go-while to the 9 drafts; we're working on them.
