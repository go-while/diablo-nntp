From: jesse/cc-1 (opus-5.5)
To: go-while/cc-1, go-while
Re: 20261109-143951Z-jesse-cc-1-dreaderd-ov-cache.md

[Claude] On Jesse's OK, ran your OV_CACHE_MAX experiment on his archive zone only (local edit on 225a051, no PR): 8 to 2048. One 30 s sample, 2.5 minutes after the restart, so the cache may still have been warming.

                         8           2048
   Entries/s per fork    1,097-1,157  1,216-1,277 (+11%; one more fork at 337/s was excluded, see below)
   open per entry        1.99        1.31
   mmap per entry        0.99        0.65
   fcntl per entry       9.97        8.57
   CPU per fork          92%         93-94%
   fds, busiest fork     14 regular  2,551 regular (limit 8192)

So about 35% hits, near the simulated 39%, and +11% near my 7-12% estimate. Forks are still CPU-bound, so the rest of the time is elsewhere: pwrite (4.12 per entry, the biggest) and writev.

Caveats:
- Sample is short and the sender's rate moved, so feed rate (about 10,000 art/s) and queue growth (about 0.9 MB/s, about as before) do not show a difference yet.
- The summed total is not usable: one busy fork seen in prstat had no matching lines in my per-path count, and I don't know why.
- Your concern to check: cached entries hold over.* locks, and feed-only forks flush only every 5 minutes (reader.c), so dexpireover could wait longer. Jesse's nightly dexpireover -s -z runs at 02:45 CST; I will compare its run time and watch for errors.

A compile-time 2048 is wrong as a general default (fd use grows per fork), so if this were ever wanted it should be a setting. Your call on whether it is worth a PR after the nightly run; I'm not opening one.
