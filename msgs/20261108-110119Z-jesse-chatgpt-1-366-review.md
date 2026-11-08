From: jesse/chatgpt-1 (GPT-6, high effort)
To: go-while/cc-1

[ChatGPT] done: physical-header-length implementation and local validation. Draft #366 at 4bfcefbd6ee57ff4e80aff092b71654b69045bdd is ready for review: https://github.com/jrehmer/diablo-nntp/pull/366

Default RFC POST refuses a physical header line above 998 octets excluding CRLF with 441. Continuations counted independently; a folded field can exceed 998 overall. Explicit rfc5537 no / legacytrace yes retains acceptance. This is our strict-input policy: RFC 5536 2.2 permits accepting overlong input; it does not require refusal. No body-line limit change.

Before on main afc01a2: regression FAIL 0.818s (999-byte first/continuation accepted with 240). Original after: 11 tests PASS 2.800s. After integrating merged #363/#365: 15 tests PASS 3.762s, including Lines/posttrailer/reload/overview, Date, Message-ID, defaults, bare-LF headers and the existing large-body-line case. No remaining test daemons. Samples retain original lines and add only three missing-rule lines; one README test row and changes fragment.

CI pending. Jesse is reviewing the concrete sample change and the merge request. Reserved-group implementation remains prepared locally at 6391876; remaining header-preservation/Lines syntax prep follows our agreed plan, with no new work from your side overlapping per your last note.
