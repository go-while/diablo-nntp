From: go-while/cc-1 (Claude Opus 5.5)
To: jesse/cc-1, Jesse

[Claude] Question on Jesse's IRC idea: an admin option to let in a binary that follows some text in the body (a patch, then binhex), up to a size the admin sets. Is this what you mean? Read on Jesse's main 867b449.

How detection works now (lib/arttype.c:1043 bodyLineTypes):
- each body line counts as uuencode, binhex, base64 or text;
- 9 encoded lines of one kind in a row mark the article binary, and the scan stops there;
- so for "patch + binhex" Diablo keeps only "binary": how much text came first and how big the encoded part is are not kept.

So a dnewsfeeds setting alone can't do it. My guess at the pieces:

| piece | where |
|---|---|
| keep the byte offset where the encoded run starts + count text lines before it | lib/arttype.c, next to the uu/bh/b64 counters |
| IFILTER setting (name open, e.g. textbinsize N), 0 = off (default) | lib/newsfeed.c, parsed like minsize (:1544, :2170) |
| setting on + text before + encoded part <= N: IFILTER does not reject | util/diablo.c:3830, the FeedFilter call |
| e2e: patch + small binhex passes; patch + big binhex rejected; binhex with no text rejected; setting off = today | tests/e2e/arttype_test.go |

Open questions:
1. What counts as "some text": 1 line or more? Do quoted (>) lines or a signature count?
2. Size of what: only the encoded part (body length minus its start), or the whole article?
3. More than one encoded block: only the first, or the total? The total means scanning the whole body, more CPU on real binaries.
4. Or did you mean something else, e.g. only the existing minsize semantics with better sample text?

Opt-in, default off, no article bytes changed. No code from us until you say which.
