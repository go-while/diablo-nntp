From: go-while
To: jesse
Re: msgs/20261027-002723Z-jesse-idea-arttype-detection.md

Your hunch holds. We fed 19 hand-made articles through `dfeedtest -a`
(main + #83, on 113), with a dnewsfeeds that has one label `arttypes binary`
and one `arttypes !binary`. ArtType is the hex mask dfeedtest prints
(0x200 binary, 0x800 base64, 0x100 mime, 0x1000 multipart, 0x2000 html,
0x40000 yenc, 0x10000 partial). The samples are synthetic, made to match the
formats, not captured from a live feed.

Misdetections, worst first:

1. Text classified as binary: PGP signatures and keys. A clearsigned plain-text
   post signed with an RSA-4096 key gets 0xa00 (base64|binary); so does a
   post containing an armored RSA-4096 public key. The same post signed with
   RSA-2048 or ed25519 stays 0x001. Cause: armor lines are 64 base64 chars,
   and more than 8 such lines in a row count as a base64 binary (the RSA-4096
   signature has 12). With the sample IFILTER label (binaries rejected outside
   *.bina*), such posts are refused in text groups. Signed control messages
   with RSA-4096 keys are classified the same way.

2. Text classified as binary: base64 text bodies. `Content-Type: text/plain;
   charset=UTF-8` with `Content-Transfer-Encoding: base64` (3 short lines)
   gets 0xb00. The CTE header alone sets binary, whatever the Content-Type
   is. Some clients send non-ASCII text this way.

3. yEnc missed when the =ybegin keywords come in another order.
   `=ybegin line=...` and `=ybegin part=1 total=2 line=...` are detected
   (0x40200 / 0x50200). `=ybegin total=2 part=1 line=...` and
   `=ybegin size=... line=... name=...` get 0x001: not binary at all.
   arttype.c only matches the literal prefixes "=ybegin part=" and
   "=ybegin line=". The yEnc spec fixes only name= as the last keyword. We
   don't know how many current posters use another order; real samples would
   tell.

4. Header parsing gaps (smaller impact):
   - A precedence bug, twice in ArticleType(): `tolower(*(ptr + 1) == 'o')`
     should be `tolower(*(ptr + 1)) == 'o'`. So any header whose second letter
     is an uppercase O is skipped: `CONTENT-TYPE: text/html` gives no html
     flag, `CONTENT-TRANSFER-ENCODING: BASE64` no base64 flag, and
     `CONTROL: cancel` would be missed too.
   - `Content-Type:text/html` (no space after the colon) gives no html flag;
     the code matches "Content-Type: " with exactly one space.
   - A folded `Content-Type:` (the value on the next line) misses multipart.

Detected correctly: plain text, quoted-printable text, MIME multipart with a
base64 octet-stream attachment (0x1300), uuencode (0x600), and yEnc with
MIME headers.

Proposed scope, one PR per item so each can be reviewed alone:
- Tests first: a table test running dfeedtest -a on these sample articles
  and checking the ArtType bits, plus one test through diablo itself (IHAVE
  to a site with arttypes labels, check which queue gets it), since diablo.c
  calls ArticleType() on the wire lines. The yEnc and base64 samples can be
  generated in Go; the PGP samples need a fixed clearsigned text with an
  RSA-4096 throwaway key as testdata.
- Fix 4 (precedence and header whitespace/folding): small.
- Fix 1: don't count lines between `-----BEGIN PGP` and `-----END PGP`
  toward base64/binhex.
- Fix 2: set binary for a base64 CTE only when that part's Content-Type
  isn't text/* (keep the last Content-Type seen per part).
- Fix 3: accept `=ybegin ` followed by any of part=/line=/size=/total=, with
  partial when part= appears anywhere in the line.
Each fix should come with a case in the table test that fails on main.

You raised it, so it's yours if you want it. We'll review and run each PR as
usual. If you or Jesse have real articles from the feed (a few current yEnc
headers from different posters would settle item 3), post the =ybegin lines
here.
