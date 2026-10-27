From: go-while
To: jesse

Build and e2e results on 20 Linux VMs (1 core each).
- Code: 6-CUR-20261024-BWH plus the host cache fix plus tests/e2e (branch e2e-with-fix).
- Build: tests/e2e/ci/build-on-host, RADIUS off. Berkeley DB was available everywhere.
- Tests: 11 per VM.

| distro | gcc | build | warnings | tests |
|---|---|---|---|---|
| AlmaLinux 9.4 / Rocky 9.4 / CentOS Stream 9 | 11.5 | ok, 39 programs | 0 | 11/11 |
| AlmaLinux 10.0 / CentOS Stream 10 | 14.3 / 14.4 | ok | 0 | 11/11 |
| Rocky 10.0 | 14.3 | ok | 0 | 10/11 (see 2) |
| Debian 13 | 14.2 | ok | 0 | 11/11 |
| openEuler 25.03 | 12.3 | ok | 0 | 11/11 |
| openSUSE Leap 15.6 / 16.0 | 7.5 / 15.1 | ok | 0 | 11/11 |
| Ubuntu 22.04 / 24.04 / 25.04 | 11.4 / 13.3 / 14.2 | ok | 122-123 | 11/11 |
| Ubuntu 26.04 | 15.2 | ok | 133 | 11/11 |
| Gentoo | 14.2 | ok | 122 | 11/11 |
| Fedora 46 / Arch | 16.2 | ok | 11 | 11/11 |
| Alpine 3.22 / 3.23 / 3.24 (musl) | 14.2 / 15.2 | dreaderd fails | 0 | 9/11 (see 1) |

Warnings:
- Ubuntu and Gentoo: 122-123 are -Wunused-result from _FORTIFY_SOURCE (write, ftruncate,
  chdir, setuid, ...).
- Fedora and Arch: 11 are glibc 2.43+ const strchr().
- Ubuntu 26.04 has both kinds: 133.

New findings:
1. musl has no loff_t. dreaderd/filealloc.c:61 and :78 cast to (loff_t) for
   syscall(SYS_fallocate), so dreaderd doesn't build on Alpine. With _FILE_OFFSET_BITS=64,
   off_t is the same size. The 2 failing tests are the reader tests (no dreaderd binary).
2. Rocky 10 hit reader finding 9 (400 after 205) with connections one after another. The
   dreaderd log shows it: connection 1 QUITs (205), and at session end its fd goes back to the
   master (SendDns). The single DNS thread is busy with connection 2, so the master logs
   "Unable to find spare DNS thread - rejecting connection" and writes "400 Server too busy"
   to connection 1, which already got its 205. Config: readerdns 1. This confirms the path I
   suspected.

Debian Hurd: not run yet (1 GB disk free on that VM).
