From: go-while
To: jesse

Status:
- PRs opened on your repo: #10, the host cache fix (fix-hostcache-fresh-start), and #11, a draft
  with tests/e2e and the workflow (branch e2e).
- All five PRs merged plus a geteuid() == 0 guard around the three user switches (local test
  branch only): 11/11 on AlmaLinux 9/10, Arch, Debian 13, openSUSE 15.6, Ubuntu 24.04/26.04 and
  Fedora 46. So #6 needs only that guard; #5, #7, #8 and #9 work together.
- Your integration branch is the same code as my "all five merged" run (without #10).

connlife e2e test (#9) is added: TestDnewslinkConnlife, on e2e, skipped without -K. It passes
3 of 3 runs on Ubuntu 22.04 with feature-dnewslink-connlife, with connlife 8, realtime on,
rtflush.
- Session: the realtime worker keeps the same socket across a rotation.
- Retirement: about 9 s in, "connlife expired; retiring after queue rotation", and the
  dspoolout -J replacement connects about 5-10 ms later. Three generations checked.
- Descriptors don't grow: each replacement has stdio, the syslog socket, the active queue, the
  peer socket and spool article files. The spool files are the article cache; their number
  varies, so they're not counted.
- Every article reaches the peer once, in order.

Two behaviour notes, not bugs:
1. After a worker has delivered records, it retires only at the next rotation, and it notices
   that rotation only when diablo has created the new active queue file, i.e. with the next
   article. The empty-queue path needs st_size == 0. On a quiet feed the worker therefore
   keeps its session past connlife until the next article after a rotation. samples/dnewsfeeds
   says "or when the active queue is empty"; maybe "when no record has been queued since it
   started on that queue file".
2. A realtime feed needs rtflush; without it, entries wait in diablo's buffer until the next
   flush or rotation. That was my test setup, not your code.

Next: running TestDnewslinkConnlife on #9 on Debian 13, Ubuntu 24.04 and Fedora 46.
