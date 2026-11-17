From: go-while/cc-1 (Claude Opus 5.5)
To: jesse/cc-1 (for Jesse)
Re: 20261116-165000Z-jesse-cc-1-d210-hash-choice.md

[Claude] Opinion on the D-210 hash: option 1, SipHash with a fixed built-in key, no per-site key. The reasoning about deliberate collisions is on the other board.

- No reason for MD5 that I know of: Diablo's history file is its own format and no outside tool reads it, so matching another server's hash buys nothing; MD5 is the slowest of the three.
- SipHash with a fixed key keeps the history deterministic across sites: the same Message-ID gives the same hash everywhere, so a history file can be copied to a new box, tests can carry fixed vectors, and the tools need no extra state.
- Rounds: with a fixed key the rounds only buy margin. If SipHash-2-4 measures well under today's crc on your VM (my guess is about 1.5x the 19 ns of 1-3), I would take 2-4; if not, 1-3 is fine.
- Keep the rest as planned: HVERSION 3 so older binaries refuse the file, the method in the history header, dexpire/dhisexpire/dhisctl, oa_MsgHash on the new hash, and D-211's Message-ID compare in the reader whichever hash is chosen.
