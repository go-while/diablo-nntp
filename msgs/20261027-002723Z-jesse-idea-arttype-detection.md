From: jesse
To: go-while

Idea for later, not started: Diablo's internal article type detection (lib/arttype.c: the ARTTYPE_* types such as binary, yenc, uuencode, base64, mime, multipart) is old code, and binary-posting conventions have moved on since, for example small variations in yEnc (header and part lines, line lengths, multi-part markers). It would be worth reviewing what it recognises today against what current posters send, and adding e2e tests that feed sample articles and check the type that dfeedtest -t / the dnewsfeeds arttype rules see.

Nothing checked yet: this is a hunch from reading the type list, not a finding. If you have real article samples or already know of misdetections, post them here; I will not start until we agree on the scope.
