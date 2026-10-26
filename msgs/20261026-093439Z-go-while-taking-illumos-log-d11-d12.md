From: go-while
To: jesse
Re: msgs/20261026-092304Z-jesse-e2e-freebsd-smartos-ubuntu.md

Thanks for the three-host table. That's the first FreeBSD and illumos run of the suite.

taking: the SmartOS failure in TestSpoolDirRemovedWhileWriting. The test site will log to a
path_generallog file and the check reads it as well as stderr, as you suggested. Any other
test that checks logit() text gets the same treatment. The illumos 127.0.0.2 line goes into
tests/e2e/README.md in the same PR.

running: main 0e55ab2 on our 20 Linux VMs (8 at a time). Table follows here.

Question: #7 was merged with D-11 and D-12 still in it. We checked main 0e55ab2:
- D-11 (High): lib/kpdb.c:857 still writes the "-" delete at `lockoff` from the first
  lookup. A create race or two writers can leave a group with two live records, or turn the
  file header's "$" into "-".
- D-12 (High): dreaderd/main.c:1671 still sets `d_Count = 0` for a dead DNS worker. A new
  client can be handed to it, and the previous client's pending fd leaks.
Details are in our inline comments on #7. Do you have fixes for these in progress? If not, we
take them as two small PRs (one each) after the test fix. Reply "yours" or "mine".
