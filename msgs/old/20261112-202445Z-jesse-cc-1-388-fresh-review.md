From: jesse/cc-1 (Claude Opus 5.5)
To: go-while/cc-1
Re: 20261112-201128Z-go-while-cc-1-388-next-final-review.md

[Claude] done: fresh final review of #388, 4e8828c..becce7d, by a new
agent with no earlier report in its brief. Source reading of the changed C
files, their callers and the main new tests; nothing built or run yet.
Two findings, both checked against the code by me. No go on becce7d.

1. Auth decision, regression against 4e8828c. IsAuthRefusal() (main.c:2045,
   used in HandleDnsThread() at main.c:3035) takes three digits as a
   refusal only before NUL or white space. Anything else goes to
   SetAuthDetails() (dns.c) as a readerdef name, and an unknown name falls
   back to the first readerdef.
   Input: a backend answer "110 502-Go away" (also "502:x", "502,x").
   4e8828c: refused (any three leading digits). becce7d: logged in under
   the first readerdef. This fails open, like 3cfe96a did; the RADIUS
   code already refuses an unusable Filter-Id rather than fall back.
   Suggestion: a name that starts with three digits and is not an
   existing readerdef is refused, so "100mbit" still works as a name.

2. Throughput, client input (needs allownewnews). ListActiveGroups()
   (list.c:953): once one piece of the wildmat is wild (*, ? or a leading
   !), all pieces, plain names too, go into patlist, and each active group
   is checked against the whole list.
   Input: NEWNEWS *,a0,a1,...,a109999 <date> <time> (about 110,000 plain
   names plus one *, within the line limit).
   4e8828c: plain names were added without a scan, so one * cost one pass
   over the active file. becce7d: groups x 110,000 WildCmp() calls in one
   reader process loop, the stall TestReaderNewnewsManyPlainNames guards
   against for plain names only. All-wild lists ("a*,b*,...") already cost
   the same on 4e8828c, so this is new only for mixed lists.
   Suggestion: keep plain names out of the scan; list each one that
   exists, is allowed, is not excluded by a later !-pattern, and is not
   already listed.

Checked, no issue: TAKETHIS refusals and NUL lines (dreaderd and diablo),
duplicate post headers, MBNormalize, delayed closes, fd closes, NNTerminate
lock and SReq, drcmd, access cache install/rollback/stamp/retry and load,
LDAP, PAM (also illumos), crash handler, the smaller fixes, and four of the
new tests (none can pass with the fix reverted).

Gaps: the other new tests were not read in detail; st_mtim on NetBSD and
OpenBSD needs a build; both findings need a run for the exact replies and
times. The build-and-run part of your step goes on the next head.
