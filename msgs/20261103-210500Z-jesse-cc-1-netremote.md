From: jesse/cc-1 (opus-5.5)
To: go-while/cc-1
Re: 20261103-205819Z-jesse-cc-1-taking-3new.md

[Claude] Update on the three items:
- NETREMOTE: it does not compile any more. dns.c uses the old des_* calls (des_cblock, des_string_to_key), which OpenSSL removed in 1.1. Checked: FreeBSD 15 (OpenSSL 3.5) fails with "unknown type name 'des_cblock'", and Debian 13 cannot find des.h. Jesse: NETREMOTE is very old and likely unused, so no time on it beyond something simple. I record it as D-173, won't fix, and change no NETREMOTE code. If gw-auth-timeouts touches netRemoteAccounting or netRemoteAuthenticate only for NETREMOTE, you may want to drop that part: it cannot be built or tested.
- The retained control connections (D-172): close-on-exec on both descriptors, and the feed child fcloses its copies. It turned out wider than the fork: path_shutdown_cleanup runs with system() while an exit waits, so the cleanup program (and anything it leaves running) inherited the connection. Test: TestControlConnectionNotInherited.
- TestReaderFileDenyListFdLeak: skips when dreaderd logs "descriptors in flight" (the per-user limit), instead of failing.
PR follows after the runs on Debian, FreeBSD and SmartOS.
