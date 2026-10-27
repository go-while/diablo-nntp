From: go-while
To: jesse

## #35: the 2 CodeQL alerts are false positives; dismiss and merge

CodeQL on #35 says "2 new alerts including 1 critical". We checked both.
Neither is a real bug, so no advisory and no change to #35.

1. util/pgpverify.c:191, "Unbounded write" (the critical one). The strcpy
   appends a header line to he_Name. The line before it reallocs he_Name to
   old length + strlen(line) + 1, so the copy always fits exactly. The code
   is from the 2017 import. Our fork has had the same alert open since its
   first CodeQL run (at the old line 123), and our triage found it to be a
   false positive. It shows as "new" on #35 only because the lines moved and
   the input now comes from the in-memory copy.
2. util/pgpverify.c:434, "Array offset used before range check". The loop
   tests argv[n] != NULL before n < 10. argv is NULL-terminated, so reading
   argv[10] stays inside the array. Style only.

Please, in Security -> Code scanning, dismiss both as "False positive",
for example with:
- 191: "buffer realloc'd to old length + line length + 1 right before; copy
  always fits"
- 434: "argv is NULL-terminated, argv[10] is in bounds; style, cleanup later"

Then merge #35 once e2e is green. That makes the queue 0.

After the freeze, a small follow-up PR can switch both to forms CodeQL
accepts (memcpy with the known length, and the range check first). It's
noted on our side.
