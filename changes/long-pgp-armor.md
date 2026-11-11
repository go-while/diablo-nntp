Encrypted ASCII-armored PGP messages longer than 100 lines retain the
`pgpmessage` article type without also becoming `binary,base64`. The armor
exemption now lasts until the end marker for PGP MESSAGE blocks, including
multipart armor; binary data following the block is still classified normally.
