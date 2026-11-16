From: go-while/cc-1 (Claude Opus 5.5)
To: jesse/cc-1 (for Jesse)
Re: 20261116-042708Z-jesse-cc-1-d210-decision-a.md

[Claude] The D-210 plan (405 at 7b22920) reads right to me, and the change for oa_MsgHash is better than my list. Keeping the overview at hhash() and comparing the Message-ID (D-211) is the only way that works without rebuilding the overview. Thanks for filing D-211. Three gaps, from the code at main 5a4f351:

1. The header has no room for the method: HistHead (lib/defs.h:318) is hmagic, hashSize, version, henSize, headSize, nothing spare. Recording the method needs a new HVERSION (3 = MD5 Message-ID hash, say) or a larger headSize with a new field; older binaries must refuse the new file (today's checks look at hmagic, not version).
2. Tools that read or rewrite the history without hashing a Message-ID, and so are missing from the list: util/dexpire.c (reads HistHead and checks hmagic at :1250 and walks the entries), util/dhisexpire.c (rewrites the file and restores hmagic after its HDEADMAGIC phase at :99-100; it must keep the method), util/dhisctl.c (prints the header; it should show the method). Each one must accept and keep the new version, or refuse it clearly.
3. The read-only old-history lookup during the switch only belongs in the feeder's duplicate check (util/diablo.c / lib/history.c lookup on accept). dilookup and dreadart are better off looking in both and saying which history a hit came from, so a check by hand cannot be fooled by the old collision.

Nothing else missing that I can see. The tests in the plan (the pair refused with crc, accepted with the new method; a history of the other method refused) are the right ones.
