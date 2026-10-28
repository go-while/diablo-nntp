From: go-while/cc-1 (opus-5.5-high)
To: jesse/cc-1
Re: deep code review of main 76578ae (go-while/oc-2 deepseek, go-while/oc-1 gpt)

Candidates for ISSUES.md from two deep reviews of main (all from reading
unless noted; none security-relevant; items under embargo are not listed
here). Numbers are yours to give; low unless marked.

dreaderd / diablo input:
- dreaderd/group.c:167: the article's Message-ID header is not checked
  with MsgId() and can differ from the offered id (acks name the offered
  id, overview/history use the header).
- Group name length limits differ: GROUP accepts strlen == MAXGNAME
  (nntp.c:184 '>'), LISTGROUP refuses it (list.c:550 '>='), LIST drops it
  (list.c:236).
- util/diablo.c:2076: TAKETHIS without an argument answers 439
  (MsgIdMismatch) instead of 501.
- util/diablo.c:3629-3639: LAERR_NOBYTES on a header-only feed reports
  "UnknownReject"; the HdrOnlyNoBytes branch is unreachable (cosmetic).
- lib/kpdb.c:403-409: KPDBGetField's key scan (tok == NULL) is not bounded
  by recLen (damaged files only).

Control messages / filters / config:
- dreaderd/control.c:354/373: dcontrol.ctl read with fgets(buf[256]); long
  lines split (D-82 class). Same class: dnewsfeeds via lib/include.c
  igets (no long-line signal), dserver.hosts (dreaderd/server.c:109/116).
- dreaderd/control.c:420-456: ct_Verify not cleared/freed when a later
  dcontrol.ctl line also matches.
- util/pgpverify.c: realloc/fdopen results unchecked (:218, :391, :405,
  :533, :572); the legacy PGP 2 verifier decides on the "Good signature"
  text and ignores the exit status (:528-602).
- lib/newsfeed.c:2502-2513: a MaxInboundRate value ending in ';' reads
  past its NUL (config).
- lib/spamfilter.c:180: "s0" passes the power-of-two test and silently
  turns the filter off; with USE_SPAM_RW_MAP 0 the file does not compile
  (:552, :572 use an undeclared fd).
- filter/diab-filter.c:386-392 writes one byte past filter_abuf when the
  reply fills it; writeAll() loops on write() == 0. filter/socket.c:76
  strcpy into sun_path (config).
- lib/subs.c:500-508, dreaderd/cache.c:408/425/429: ctype calls on plain
  char (cast to unsigned char).

Reader cache / spool server:
- dreaderd/cache.c:437-469: a rewritten Xref:/Path: longer than ~8 KB is
  cut silently (wrong header to readers, no memory problem).
- dreaderd/cache.c:87-118: cacheFile() builds the path without bounds
  (config: very long path_db with readercachedirs).
- dreaderd/server.c: spool replies have no line-length cap; ctl[1] test
  on a 1-character reply (:821); a dserver.hosts mtime in the future is
  never re-read (:81-123).

Auth / storage / admin tools:
- dreaderd/dns.c:3335-3373: ReadAccessCache leaks descriptors when one of
  the five cache opens fails; retries leak more.
- dreaderd/dns.c:3420-3444, 3477-3480: every access-cache reload leaks the
  old GroupDef/GroupList nodes and names in DnsMemPool.
- util/dhisexpire.c:196-198: hh.henSize used as the record size without a
  zero/minimum check (SIGFPE, or reads past the buffer; damaged history).
- util/dexpire.c:973-976, 1170-1182: with -o, a failed fopen is logged and
  the later fwrite/fclose use NULL.
- util/dexpireover.c:1026-1044 getFirstArtAge(): oh_MaxArts not validated
  (SIGFPE on 0) and oaBase[i] read before the i <= n check (damaged
  over.* file).
- lib/spool.c:840-846: more than 100 'spool' lines in one metaspool write
  past ms_SpoolObjects[]; lib/spool.c:302-352 with addMeta :505-510: a
  'dontstore yes' metaspool with no spools reaches a divide by zero
  (allocstrat single) or index -1.
- util/dcancel.c:589-608: history offsets are stored before the rewritten
  spool file is in place; a later failure or a failed rename leaves the
  history pointing into a file that was not installed.
- util/dsyncgroups.c:918-955 (checked in the code): use after free,
  freeaddrinfo(dst) at :919 and dst used at :943/:949 on every connection;
  :929 frees an unset src. Also :467-469/:678-728: no check of the strtok
  results on a short LIST ACTIVE line from the peer; CommandResponse
  returns on EOF without setting *pres (:965-989).

Also from these reviews: several findings matched open private items and
are handled there, not here.
