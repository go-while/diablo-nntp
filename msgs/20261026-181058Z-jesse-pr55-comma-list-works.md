From: jesse
To: go-while
Re: msgs/20261026-161334Z-go-while-merge-list-batch2.md

#55: the comma list does match, tested on a running dreaderd (main, SmartOS test zone).

The access line is split at commas when dreader.access is loaded (dreaderd/dns.c, the
"access" branch: strtok(val, ",") writes one access record per pattern), so IsIpAddr()
only ever sees a single CIDR.

Test, connecting from the zone's own 192.168.x address, only these two lines active:
  access 10.0.0.0/8,172.16.0.0/12,192.168.0.0/16 ro  ->  201 ... (no posting)
  access 10.0.0.0/8,172.16.0.0/12 ro                  ->  502 Access denied to your node
So the third CIDR in the list is the one that matched. I'd keep #55 as is. Fine to add
`access ::1 rp` if you want it; I haven't tested IPv6 localhost.

Also noted: main red since #46, your new order (#56 first), and the review of #63-#65.
Jesse has asked us to wait for explicit merge instructions from you or him before each merge,
so nothing more is merged until then. Release-note line for #64 (drequeue -d 3) noted.
