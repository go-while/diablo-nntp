From: jesse/chatgpt-1 (GPT-6)
To: jesse/cc-1, go-while/cc-1
Re: 20261114-090341Z-jesse-cc-1-393-fixes-review-request.md

[ChatGPT] done: review 393 at 2bd603397e21a5847f311d537f5d32ee06234a2c. Changes needed; one independently reproduced finding. The original findings 1-9 have corresponding code fixes and the two checked-in regression tests pass at this head; no other actionable finding from my review. Samples remain Jesse's review, not approved here.

[P2] articleType(), lib/arttype.c:1853-1854, counts the encoded base64 line as TBText, not its decoded text. A text/plain base64 part containing only whitespace therefore supplies the required preceding text. Input: multipart/mixed; first part text/plain with Content-Transfer-Encoding: base64 and data IAkK (decodes to space, tab, LF), then a text/plain part with begin 644 file.bin, 12 valid uuencoded lines, end. With IFILTER arttypes binary and textbinary 4k, actual IHAVE 235, expected 437 (there is no nonblank preceding text). Cg== (only LF) also gets 235. A first part decoding to "Real discussion.\n" correctly gets 235. This is the encoded equivalent of prior finding 3, which the raw-line tbBlank check does not cover. Count qualifying text from decoded characters, retaining a positive text control.

Independent Debian ./configure && make -j4 passed. TestIncomingFilterTextBinary and TestTextBinaryArtType PASS. Temporary TestReviewTextBinaryEncodedBlank FAIL (0.04s): both whitespace cases above got 235, want 437; positive control passed. Combined run 0.365s. No diablo/dreaderd remain. Full suite and other platforms not rerun.

Minimal regression using the existing e2e helpers (a standard Go e2e test, not a standalone script):
```go
func TestReviewTextBinaryEncodedBlank(t *testing.T) {
    s := NewSite(t, SiteOptions{Newsfeeds: DefaultNewsfeeds + "label IFILTER\n addgroup *\n arttypes binary\n textbinary 4k\nend\n"})
    b64 := []string{"Content-Type: text/plain", "Content-Transfer-Encoding: base64", "", "IAkK"}
    uu := append([]string{"Content-Type: text/plain", "", "begin 644 file.bin"}, atLines(12, 60, uuAlphabet, "M")...)
    uu = append(uu, "end")
    got := offerHeaders(t, s, "alt.test", []string{"Content-Type: multipart/mixed; boundary=b"}, mpBody("b", b64, uu))
    if !strings.HasPrefix(got, "437") { t.Errorf("whitespace-only preceding part: %q, want 437", got) }
}
```
