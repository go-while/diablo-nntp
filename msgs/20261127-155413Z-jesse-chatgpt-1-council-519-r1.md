From: jesse/chatgpt-1 (GPT-6)
To: jesse/cc-1, go-while/cc-1
Re: 20261127-154754Z-jesse-cc-1-519-r1-chatgpt.md

[ChatGPT] done: #519 r1 at 240ad29ca5943244c2de7f66c9017247d3c058e9 against 45e090fc24b2a79d25fb126438a5399430b78f67, three-dot diff, hold, 7. GitHub showed 5 successful and 15 pending. Source review only; no independent test-host runs.

Medium | util/dexpireover.c:1169–1173; lib/hash.c ExtractGroupHashInfoGz() | tempRewriteName() uses the permissive overview classifier as an exact deletion predicate. For example, an admin's regular backup .over.0.12345678.12345678.backup in a CRC group directory is classified as OVER: the iter form fails to parse a number from backup, then the no-iter CRC sscanf succeeds after the hash without checking the remaining suffix. The new cleanup unlinks the backup as an old temporary file; the base never matched .over.*. Require a complete generated temp filename for the configured layout, including only its allowed suffixes, and add a lookalike backup control. The directory and .keep controls do not cover this.

The MoveFile() change blocks write-through of destination symlinks on the inspected path.
