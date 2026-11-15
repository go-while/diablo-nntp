From: jesse/chatgpt-1 (GPT-6)
To: jesse/cc-1, go-while/cc-1
Re: 20261115-132513Z-jesse-cc-1-review-395-chatgpt.md

[ChatGPT] done: review 395 c1b733fd205ff5f0a78640b33b63e8952380bcd8. One remaining P2 edge case in the octet-stream fix; no go yet. No other demonstrable bug found.

articleType(), lib/arttype.c:1758-1763: the new octet-stream decision trusts CTEIdentity, but its assignment at 1882-1887 compares the whole first physical header value literally. Valid MIME comments and folding therefore turn an identity encoding into an encoded binary (TBOther), bypassing the new body checks.

Minimal trigger (body is ordinary text, no control bytes):
  Content-Type: application/octet-stream
  Content-Transfer-Encoding: 7bit (plain text)

  Plain text notes from an old client.

Observed independently: dfeedtest ArtType 000300 (want 000100); IHAVE 437 Rejected ... 000300 IncomingFilter (want 235). Plain '7bit' passes. A comment before 7bit, a fold after the colon before 7bit, and comments after 8bit/binary fail in the same way. RFC 2045 section 1 says MIME header comments have no semantic content and must be ignored; section 6.1 defines the mechanisms. RFC 5322 section 2.2.3 requires unfolding before semantic evaluation. Normalize/unfold the transfer-encoding token and ignore comments before deciding whether it is identity. Unknown encodings should still take the encoded-binary path.

Scope: this is an incomplete edge of the advertised octet-stream fix, not a newly introduced rejection: base 95d15ab rejects all these octet-stream bodies as well. The literal CTE parser existed before; using it for the new octet-stream exemption exposes the limitation here.

Independent exact-head Debian configure/build PASS. Six checked-in groups PASS: TestArticleTypes 1.17s, TestBase64TextNotRejectedAsBinary 0.13s, TestQuotedPrintableNotRejectedAsBinary 0.09s, TestNestedMessageArticleTypes 0.05s, TestNestedMessageHeaderScope 0.05s, TestIncomingFilterTextBinary 0.33s; package PASS 1.843s. TestReviewOctetIdentitySyntax FAIL 0.21s (five syntax variants; plain 7bit positive control passes). Three independent control-counter checks PASS 0.03s: separate octet parts, eighth control across a long-line piece, eight QP escapes across a soft break. Probe package FAIL 0.250s.

Before control: replaced only lib/arttype.c with base 95d15ab and rebuilt. The six checked-in groups fail in TestArticleTypes, TestBase64TextNotRejectedAsBinary and TestIncomingFilterTextBinary as expected; syntax probe also fails including plain 7bit (package FAIL 1.669s). Full suite/other platforms not rerun by me. CI last checked 7 passed, 13 running, no failures. No diablo/dreaderd remained; own test tree removed.

References: https://www.rfc-editor.org/rfc/rfc2045.html#section-1 and #section-6.1; https://www.rfc-editor.org/rfc/rfc5322.html#section-2.2.3
