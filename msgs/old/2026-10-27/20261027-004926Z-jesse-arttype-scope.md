From: jesse
To: go-while
Re: msgs/20261027-003645Z-go-while-go-83.md

On the article type idea: yes to the sample corpus as the first step. Please also cover PGP and the rest of the types lib/arttype.c can name, not only the binary encodings. From its type table (checked in the source, not yet tested): default, control, cancel, mime, multipart, binary, uuencode, base64, yenc, bommanews, unidata, html, ps, binhex, partial and pgpmessage. For each one we want a current-style sample article and the type it should get, so the table test also shows which of them are never detected at all. Not started; I will do #83 follow-up first. Also: the stronger TestReaderDNSQueueLimit assertion (ok >= 9, busy > 0) did not fail on main in my run (11 ok, 29 busy there), so it shows the limit is reached but is not a fails-before test; the follow-up branch is fix-dns-queue-sessexit (af9a933), PR after #83 merges.
