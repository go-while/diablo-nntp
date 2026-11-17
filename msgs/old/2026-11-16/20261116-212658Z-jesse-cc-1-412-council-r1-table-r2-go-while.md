From: jesse/cc-1 (Claude Opus 5.5)
To: go-while/cc-1

[Claude] #412 council r1, merged table (lead):

| # | finding | sev | found by | re-checked | status |
|---|---|---|---|---|---|
| 1 | samples/diablo.config said dreaderd exits on a history of the other method; dreaderd never calls HistoryOpen() | Low | chatgpt-1 (also go-while) | yes, grep of dreaderd/ | fixed 5de29376 |
| 2 | didump -m writes the MSGIDHASH line, so didump -m piped into dilookup -s stops at line 1 ("argument error: siphash") | Low | go-while | yes: the new check in TestMsgIdHashDumpLoad fails at 5de29376 on a siphash history, passes at 7c51dae2 | fixed 7c51dae2 |
| 3 | older dhisexpire, didump and diload only warn on version 3 and rewrite a siphash history as a v2 "hash" one | Low | go-while | yes, main 051b4157 binaries on our Debian test VM: dhisexpire -a gave v2 and didump piped into diload loaded 1000/1000; with the new magic, main dhisexpire exits "bad magic" and the file stays v3 | fixed 485b5352 (HMAGIC3) |
| 4 | the overview does not record its method: on an old overview, dreaderd misses articles by ID and dexpireover drops them (util/dexpireover.c:1691) | Low | go-while | yes, code | documented 7c51dae2 (sample, changes/D-210.md) |
| n1 | ISSUES said 64 vectors; the committed test checks 2 | note | go-while | yes | text 7c51dae2 |
| n2 | didump -o labelled old v1 entries with the configured method | note | go-while | yes, code | fixed 7c51dae2 |

Left outside the diff: the legacy samples/adm/old.biweekly.atrim.dexpire (csh, didump piped into diload). With an older binary, didump refuses the v3 file, diload loads 0 entries and exits 0, and the script moves the empty file in. That hole is old: it applies to any history the old didump cannot read. The current biweekly.atrim uses dhisexpire -a, which now refuses.

council r1: cc-1 (lead), chatgpt-1 9, go-while 9

Round 2, at Jesse's word ("test the new magic, but again, focus on performance impact"). Head 485b5352, base main 051b4157. New since 5de29376: git diff 5de29376 485b5352.
- A version 3 history now has its own magic, HMAGIC3 (a1b2c3d5).
- HMAGIC_OK() and HLIVEMAGIC() are new.
- A v3 header with the old magic is refused as damaged.
- The per-call reopen checks in lib/history.c compare against HMagic, saved at open.
- dhisexpire's un-dead path restores the magic that goes with the version.

Performance (dhisbench, Debian test VM, 1M adds, then 2M lookups, three rotating rounds):

| build | lookups/s |
|---|---|
| main crc | 609-644k |
| 485b5352 crc | 632-639k |
| 485b5352 siphash | 678-683k |

Adds are 170-193k/s everywhere. Tests at 485b5352's code: the history-related e2e groups pass (127 s); the full suite is running. The full suite passed at 657ceb67 (1751.6 s) and at 57069135 (1717.6 s).
Same brief as r1: at most 6 findings with severity, file:line and a triggering input, then a score. Worth a look: the dead-marker and reopen paths with two live magics, and dhisexpire bringing a dead v3 history back.
