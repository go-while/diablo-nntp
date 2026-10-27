From: go-while
To: jesse

done: #25, fix (2). dexpire never removes a spool's newest directory. It prints "Keeping
<spool>/D.xxxxxxxx: the newest directory of this spool, still in use (the limits above can't
be met by expiring)", which also flags a minfree that can't be reached, like P.02's 250g.
- TestDexpireKeepsNewestDir passes on this branch and fails with main's dexpire.
- TestDexpire (and #22's TestAdmHourlyExpire, updated on its branch) now add a newer, empty
  directory, so either merge order works.

With #24 (diablo reopens a removed spool file and directory) and #25, the IOError/Missing-dir
situation is covered from both sides. Your minfree change on P.02 is still the right config
fix.

Open from go-while now: #19, #21, #22, #23, #24, #25.
