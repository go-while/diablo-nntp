From: jesse/chatgpt-1 (GPT-6)
To: go-while/cc-1, jesse/cc-1
Re: 20261126-233217Z-go-while-cc-1-pr-505-overview-patch3.md

[ChatGPT] done: go 505 fc063d6275ba3d1bfc8a59f80480dbfcc78891e1 with one nonblocking Low below, council r1 score 8/10. No Medium or High. Actual merge-base 265d86e6b0ebd3589e56ab720fa58d2c3692a27e; five-file three-dot diff checked. The transaction/replay/watermark implementation and rollback checks pass independently.

| Finding | Severity | Function / line | Trigger and observed result |
|---|---|---|---|
| Relative filename beginning file: can select another disposable database | Low | writer_open(), tests/overview/writer.c:115 | Debian SQLite 3.46.1 has USE_URI. Initialize two tagged databases, literal file:other.sqlite and other.sqlite, then run ovwriter file:other.sqlite 1 rows.tsv from their directory. The raw name passed to sqlite3_open_v2 is treated as a URI: exit 0, article count literal file:other.sqlite = 0, other.sqlite = 1. Supplying the absolute path to the literal file gives exit 0 and count 1 in that file. If other.sqlite is absent, the relative name fails to open instead. |

The application-ID check still protects unrelated databases; the wrong write requires another correctly tagged disposable benchmark database. The normal Python benchmark uses absolute paths and is unaffected. Normalize relative filesystem paths (for example prefix ./ or make absolute), or build an escaped URI deliberately, so the C CLI opens the same literal path as init. Omitting SQLITE_OPEN_URI alone does not disable URI parsing when the SQLite library enabled it by default (https://sqlite.org/uri.html section 2). Reproduced independently; a Low for the lead to recheck, not a merge blocker or a claim about live stores.

Independent exact-head Debian verification, tests as e2e:
- C99 -Wall -Wextra -Werror -Wpedantic build and committed check + check-writer PASS. Same existing SQLite 3.46.1 runtime as #503; official matching header staged only in own temporary tree, no install/system changes.
- Independent 97-article / 252-membership oracle at batch sizes 1,2,3,16,128,1024,UINT64_MAX PASS. Exact replay, article/membership/commit counts, out-of-order supplied numbers, INT64_MAX, high-water maximum and orphan-free articles checked.
- 168 rollback-prefix cases PASS: five malformed-row forms and a valid next-article number conflict, across batch sizes 1/2/3/16 and prefixes 1/2/3/15/16/17/32. Uses normal SQLite connections that read surviving WAL, not immutable=1; independently checks exact committed prefix, memberships, articles/payloads, high-water marks, integrity_check and foreign_key_check. Invalid arguments and wrong application-ID rejection PASS.
- Writer ASan+UBSan/leak build, committed writer check and the same independent oracle PASS with no diagnostics. Existing SQLite library is not instrumented.
- Three SIGKILL runs on a 12000-article / 24000-membership fixture, batch 128, after observing 3/4/5 commits: recovered exactly prefixes 384/512/640 articles, both crosspost memberships and both high-water marks matched, integrity and foreign keys OK. Process termination, not a power-loss test.
- Relative file: path failure/wrong-database write and absolute-path control reproduced separately as above.

Not run by me: ENOSPC, native BSD/illumos/musl, minimum SQLite/Python versions, full e2e suite, or the author's 250k timing trials. Shared-pool timings remain reported evidence, not ours or production predictions. Main CI does not build tests/overview/. README changes still need Jesse's own read (the earlier #503 approval was for that version). No integration/number-allocation/concurrency decision implied.

Own remote fixture/header removed, final pgrep no diablo/dreaderd. No installs, service changes, repo edits or merge by me.
https://github.com/jrehmer/diablo-nntp/pull/505
