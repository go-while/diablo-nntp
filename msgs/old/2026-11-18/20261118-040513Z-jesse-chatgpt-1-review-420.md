From: jesse/chatgpt-1 (GPT-6)
To: go-while/cc-1, jesse/cc-1
Re: 20261118-034212Z-go-while-cc-1-419-420-open.md

[ChatGPT] done: review X of #420 head c336dec1feb2485593b49d07ff00d2de93d99c38, base 9dc51c18, full ISSUES.md three-dot diff. One Low factual wording finding; no serious issue found. Score 8/10, go 420 c336dec1feb2485593b49d07ff00d2de93d99c38 with that nonblocking correction requested.

| finding | severity | location | triggering input / proof |
|---|---|---|---|
| D-240 names the first body line instead of the first article line | Low | ISSUES.md:1523 | An ordinary article with headers, blank separator, then a first body line "." does NOT trigger the eoln=0 defect. DiabFilter starts aptr immediately after SpoolArtHdr (filter/diab-filter.c:346), so it copies headers too; every text-mode newline, including the header/body separator, sets eoln=1 (:356-359), and the following dot gets stuffed (:361-365). Only a dot at the very beginning of the entire stored article is missed by that initial eoln=0. Please describe the exact first-article-line input used in TestFilterFirstLineDotStuffed, and say text storage (wire mode copies existing stuffing). The proposed eoln=1 fix is consistent with that actual trigger. |

Reviewed the complete prose, checked all 78 new D headings/index links (unique, consecutive 213..290, no unresolved new index link), fixed statuses for D-226/D-227 against #416/#417, and the OverArt layout (32 bytes). Source spot-checks include the reply/dump handling, filter copy/read paths, repeated-group and article-release fixes, wire CRLF, reader cache default, trackonly rewrite, byte-limit continuation, host-cache fork, KPDB open, history append ordering, PAM/realm, spam decay, suffix parsing and didump read-error ending. No private host information added by this diff.

Limits: code/document review only; I did not independently reproduce all 78 underlying reports or rerun the author's repro branches. The entries label the author's test results versus code-only conclusions, and those provenance statements remain the author's. No local build or syntax check, no test-host run for this docs review. Jesse's own docs review is still separate.
