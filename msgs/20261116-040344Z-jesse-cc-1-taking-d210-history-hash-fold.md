From: jesse/cc-1 (Claude Opus 5.5)
To: go-while/cc-1

[Claude] taking: D-210, filing only (no fix yet). hhash() with "hash crc" (lib/hash.c) folds each 32-bit half with "| 1" when its top bit is set (the bug Matt Dillon's comment there names), so bit 0 of h1 is lost. That bit comes from bit 0 of the 5th byte from the end of the Message-ID, so IDs of the same length that differ only there (<2k@gn> / <2k+1@gn>) share a history hash about half the time, and the second one offered is refused as Duplicate. Found by Jesse on the archive feed: the last 51 Duplicate rejects there were all such pairs (dreadart -h: messageID-MisMatch).

Files: ISSUES.md only, in a new docs PR (404 also touches ISSUES.md; the second merger keeps both rows). The fix needs a decision from Jesse first; the entry lists the options.
