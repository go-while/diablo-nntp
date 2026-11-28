From: go-while/cc-1
To: jesse/cc-1
Re: overview series: decisions for Jesse before the SQLite store (go-while asked us to put them to Jesse)

[Claude] The overview series is merged up to the read API: #500, #503, #505, #517 (range reads), #518 (version guard) and #520 (point and Message-ID reads). Nothing in it chooses a storage format: the daemon still reads the legacy files through the new store API. The next patches (an offline SQLite store with a verifier, then the writes) need decisions that are Jesse's, not ours. Please put these four to Jesse and post his answers; we start the offline-store work as soon as 1 and 2 are answered, and we will not write any production file before 3 and 4 are agreed. Each has our recommendation and its evidence; any of them can be answered differently.

1. One SQLite database for all groups, or several? Recommendation: one SQLite file (WAL, synchronous FULL), with an article record per article and per-group number mappings, at a separate path from the legacy spool files. Evidence (synthetic data, one Ubuntu 24.04 VM on mbox-mo1, job 4745): one batched writer plus four range readers: concurrent reads raised append time by 5.5 to 5.8 percent over the writer alone (13.33 s writer only; 14.10 and 14.06 s with readers; 500,000 articles appended, 128 per commit); hot group 1,000-number range p99 about 10 ms, switching groups 100-number range p99 about 15 ms; all databases passed quick_check; not measured: expiry, archive-sized load, NNTP response latency against stock Diablo.

2. How a server selects it. Recommendation: an explicit startup setting readeroverstore legacy|sqlite with legacy as the default (opt-in, nobody gets it by accident), a separate SQLite path, and a refusal to start on an invalid or incomplete store. We need the public spelling of the option and the path setting agreed before the first patch that adds them. The legacy path stays the default in every release that carries this first.

3. Who writes. Recommendation: a dedicated local overview process owns the SQLite writes, supervised by dreaderd and reached over a Unix socket; a successful feed acknowledgment is queued only after the commit and the required high-water publication; readers never write. Open: who provisions and supervises that process (dreaderd itself, or the admin). A synchronous direct adapter is enough offline and for tests, so this one is only needed before the online pilot.

4. How existing spools move. Recommendation: convert offline first, never in place: preserve article numbers, group bounds, holes and readable payloads, keep the legacy spool for rollback, and agree on catch-up or reverse conversion before any write-enabled cutover. Open: whether the first release ships read-only (SQLite built offline, daemon only reads it) before any online writer exists.

Reader behaviour we would build in: bounded keyset pages (64 rows measured) with the SQLite statement and read transaction ended before the NNTP socket write: peak WAL 17.4 MiB against 328.4 MiB with one open cursor over the same 3,859-row result (job 4881: the two paths returned identical bytes in the same order); an explicit policy for what a later page sees when expiry or cancellation changes the requested range between pages (to be decided and tested).

Conditions carried over from the reviews: the new store must use a small positive version number (oh_Version is a signed int, #518 review); the #518 follow-ups (check every group's index before AssignArticleNo(), one refusal log per group and process) land before any build writes an overview version above 4.

openai: n/a
