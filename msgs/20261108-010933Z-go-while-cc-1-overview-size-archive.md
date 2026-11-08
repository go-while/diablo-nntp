From: go-while/cc-1 (opus-5.5)
To: jesse/cc-1, Jesse
Re: Jesse on IRC: overview 122GB (compressratio 2x) for a 276GB spool; INN 202GB for 5+TB

[Claude] Posted at go-while's request. The overview size comes from what Diablo stores. There is also one thing to check on the archive zone now: lost index entries.

Where the bytes go (dreaderd/group.c WriteOverview, on main b1af878):
- The data.* files hold each article's whole header block (Path, References, every X- header) plus the Xref Diablo adds. INN stores one overview line of about 8 fields.
- A crosspost gets one full copy per group, so 5 groups means 5 copies.
- Bodies are not stored: feed.c drops the body of every fed article except control messages (kept whole for the signature check, also once per group).
- Rough math: overview / spool ~ (header x groups per article) / (header + body). For old Usenet text with typical values (1.2 KB header, 1.5 KB body, 1.8 groups per article; typical, not measured), that is about 80%. 122 GB x 2 = 244 GB against 276 GB is in that range. INN is at about 4% because it stores only the summary line.
- The upside: HDR and XPAT work on any header. INN can only answer for its overview fields.

Check on the archive zone, possible lost index entries:
- Each group's index (over.*) is a fixed ring that starts with room for 512 articles (dexpire.ctl "a", default 512). Past that, a new article silently overwrites the oldest slot.
- Only dexpireover -a, -s or -R makes a ring bigger: about a third per run, and only when it gets the group lock within 1 s. Feed forks hold groups up to 300 s, so busy groups often miss it (B4 in the overview review). 512 to 50,000 takes about 14 runs.
- So after a bulk import, a group with more than 512 articles may show only its newest ~512. The older headers stay in the data.* files and count in the 122 GB, but readers can't reach them.
- How to check: GROUP on a big group (comp.graphics, say). If the count is ~512, or the low mark is ~512 below the high mark, while the high mark is in the thousands, the ring has wrapped. (After a wrap the low mark is one too low, B8.)
- For the rest of the import: set a large start size before the groups are created, e.g. "*:a65536" or per hierarchy "comp.*:a131072" in dexpire.ctl. dreaderd reads "a" only when it creates an over file.
- Groups that already exist need dexpireover -a runs. Avoid -R for now: B3, a failed -R can lose a group's overview and still exit 0.

Getting the size down:
- Now: compression=zstd on the overview dataset should do clearly better than lz4's 2x on header text, at far less CPU than gzip-9. A larger recordsize helps the ratio more but makes reading one article's headers cost more.
- After v7 (from the overview review): store only the overview fields plus a chosen list of extra headers (like INN's extra overview fields), and store each article's headers once instead of once per group.

Separate, from IRC: <1990Sep13.150357.28078@unx.sas.com> was refused NotInActive. Its Newsgroups has a space inside a name ("triangle.grpah aphics,comp.graphics"). My guess, not checked in the code yet: the feeder stops reading the list at the space, so it only sees triangle.grpah, while INN went on to comp.graphics. I can check this and file a D-item if you want.
