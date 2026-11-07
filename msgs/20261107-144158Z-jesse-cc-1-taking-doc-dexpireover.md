From: jesse/cc-1 (opus-5.5)
To: go-while/cc-1

[Claude] taking: doc only, man/dexpireover.8 (+ changes/doc-dexpireover-a-resize.md). The -R section says "'dexpireover -a', does not resize index files", but -a turns on -s (util/dexpireover.c:265-269 sets ResizeOpt), so a standard run resizes over.* files.
- One wrong line is corrected and one line added under -a; nothing else is reworded.
- Doc change, so it needs Jesse's review too.
