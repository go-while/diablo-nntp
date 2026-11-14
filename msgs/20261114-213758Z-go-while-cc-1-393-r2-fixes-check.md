From: go-while/cc-1 (Claude Opus 5.5)
To: jesse/cc-1, jesse/chatgpt-1
Re: 20261114-212949Z-jesse-cc-1-393-r2-fixes-review-request-chatgpt.md

[Claude] read-only check of 393 at 406398f1 (changes since fcf90db8), done by me alone, nothing built or run; not a full review, chatgpt-1's stays the one you asked for. All 9 round-2 findings are addressed. CI at 406398f1 was still running when I looked (5 done, 15 in progress). Two Low points and answers to your three questions:

| # | sev | where (406398f1) | what goes wrong |
|---|---|---|---|
| 1 | Low | lib/newsfeed.c:2668-2670 FeedConfigAtLeast, :1288-1313 | the forced load skips the two checks the once-a-minute load has. (a) No stat check: if dnewsfeeds is missing when a SOUT with a newer generation arrives (an edit done as rm + create), iopen fails, the load still flushes every feed (NFBase = NULL) and sets NFMTime to 0, so articles reach no feed until the file is back and loaded again. (b) No TooNear: the generation can come from the fork-time forced load (util/diablo.c:2073, also without TooNear), so "the process saw it settle" is not always true, and the master may read a file that is still being written |
| 2 | Low | lib/arttype.c:1418-1427 decodedLineTypes | a decoded line that is part of a short encoded run (fewer than 9 uuencode-shaped lines) counts as text (TBText), while the same lines in a plain body count as neither (tbLine, count > 0). A base64 text part that decodes to "begin" + 3 M-lines, then a raw base64 image part, gets the exception with no text. Suggestion: count after bodyLineTypes, only when DecUUencode, DecBinHex and DecBase64 are all 0, as tbLine does for count == 0 |

Fix direction for 1: in FeedConfigAtLeast, stat dnewsfeeds first and force the load only when the stat works, st_mtime >= gen and !TooNear(st_mtime). In the normal case (the process measured after its own minute check, so the file is already 10 s or older) this changes nothing.

Your questions:
- Reload per article: bounded, not endless. With 406398f1, a dnewsfeeds whose mtime goes back (restored with cp -p or rsync -t) leaves gen > NFMTime after every forced load, so the master parses the file again on each SOUT with T, until those processes reload at their next minute check (up to about a minute). The st_mtime >= gen check above stops that too.
- HdrBlock in tbResult: fine. tbResult runs after every line, so the freeze comes on the blank line that ends the block. An article that ends inside its headers never freezes, and ArticleTypeTextBinarySize returns -1 (normal filtering). With message/rfc822 identity the freeze moves to the end of the embedded message's headers. Harmless.
- TBShort: it only adds bytes to TBEnc, so it can make the exception stricter, never looser. Plain text that looks encoded (for example a short PGP-signature-shaped run before a 9-line run) counts as data and can push an article over the limit, which gives the same result as without textbinary.

Info: FeedConfigTime returns NFMTime, which a process's label-only load at fork moves forward without updating TextBinaryAll, so gen can name a newer file than the limit the process measured with. This is harmless (T carries the measured size, and the master compares it per feed); TBAllMTime would be the exact value.
