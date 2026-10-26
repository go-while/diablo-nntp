From: go-while
To: jesse
Re: msgs/20261026-085038Z-jesse-merged-whats-next.md

Great, and CI on main is green after all of them (d7bc2c6). The UDP find (#27) is a good one:
glibc returns the TCP result first even without a socktype hint, so our Linux e2e runs never
saw it. Only a real FreeBSD host shows it. #27 looks right; we won't duplicate it.

# Next on go-while's side
1. **Docs PR:** the e2e README test table (the rows taken out of #21, #22 and #25), plus
   ISSUES.md and REVIEW.md statuses.
   - Newly fixed: D-23 (#26), D-25/D-34 (#21, #22), the Cancel-Lock verification (#19).
   - New entries: the silent article loss with a removed spool directory (#24), dexpire and
     the newest directory (#25), and the getaddrinfo socktype bug (#27).
2. **A full run of main** (after #27) on all 20 Linux VMs and Alpine/musl, as promised after
   the merges, with a per-distro table here.
3. **Next review area** from REVIEW.md: area 7, the reader protocol (dreaderd nntp.c, reader.c,
   post.c, ...), never read in full so far, plus CAPABILITIES for dreaderd (D-15).
   CAPABILITIES lets clients and nntp-testkit's 30 RFC 3977 checks see what dreaderd supports.
4. **Cancel-Lock PR 2 (generation on POST)** once the sha1 question is answered. It could also
   start with sha256 only, plus a switch for sha1.

# Suggested for Jesse, in this order
1. **Merge #27** (the UDP fix), after it passes CI.
2. **#7:** fix D-11 (kpdb "-" at the stale offset) and D-12 (dead DNS worker selectable) first,
   ideally also the FFFFFFFF marker and cache.hits; go-while re-reviews. This is the biggest
   remaining risk of data damage once #7 is merged.
3. **#9:** run the 24 h connlife test on the SmartOS or FreeBSD test box (connlife 300, then
   86400), then decide.
4. **Release tag:** main now has a lot of fixes since 6-CUR-20261024-BWH. A tag now (before #7
   and #9) gives admins a tested point. For CHANGELOG.md and the release notes:
   - **Behaviour change:** readercanlock defaults to require-auth (#19).
   - **Install changes:** real config files, an empty dactive.kp, /news/adm (#21).
   - **New:** POSIX adm scripts and crontab (#22), diablo-ctl.
   - **Fixes:** #10 to #16, #24 to #27.
   - **BUILD-PORTABLE-MAKE.md (D-19):** add it, or drop the link.
5. **Decisions still open:** D-16 (RADIUS/DB on by default in vendor.h) and D-21 (exit on a
   dactive.kp write error in #7).
6. **Ask Julien or the INN admins about sha1** generation for Cancel-Lock PR 2.

Starting with 1 and 2 on our side now.
