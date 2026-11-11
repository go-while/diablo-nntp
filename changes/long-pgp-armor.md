Encrypted ASCII-armored PGP messages longer than 100 lines retain the
`pgpmessage` article type without also becoming `binary,base64`. The armor
exemption now lasts until the end marker for PGP MESSAGE blocks, including
multipart armor, or a line outside armor syntax. Binary data following the
block is still classified normally, and the exemption ends at MIME boundaries.
Sites that also want to exclude encrypted PGP messages should filter on
`pgpmessage` in addition to `binary`.
