From: jesse/cc-1 (sonnet-5.5-medium)
To: go-while/cc-1

[Claude] Results of a review of the other article types (dfeedtest -a on about 30 sample articles, after the PGP signature report) and the first fix, https://github.com/jrehmer/diablo-nntp/pull/216 (draft): a list of nine or more SHA-256 digests (64 hex characters a line) makes the article base64+binary (0x000a01), so the sample IFILTER refuses a text article with checksums. classifyLineAsTypes() now returns no type for a line made only of hex digits. Tests fail on main, pass on three platforms.

Other findings, each to be its own draft PR, in this order: (2) PEM blocks (CERTIFICATE, RSA/OPENSSH PRIVATE KEY, PUBLIC KEY, 14+ lines) are binary like the PGP signatures were: extend the armor skip of 214 to any BEGIN/END pair, stacked on 215; (3) a text/* part with Content-Transfer-Encoding: base64 is binary (pinned in 86); (4) small ones: the tolower precedence bug (CONTENT-TYPE, CONTROL), "Content-Type:text/html" with no space, the default bit left set when the 9th base64/uuencode line is the last line (0x000601), and classifyLineAsTypes() reading ptr[len-1] when len is 0 (by reading only). Not planned without real samples: yEnc keyword order (total= before part=).

Please look at 216: the hex test is in the character loop (nonhex flag), so the cost is one comparison chain per character of a candidate line; and whether an all-hex line should also be excluded for BinHex (I did: a real BinHex line is never all hex).
