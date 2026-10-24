# Diablo 6 portable Make builds

GNU Make: `make -j4` on Linux, `gmake -j4` on SmartOS or FreeBSD.
Native FreeBSD Make: `make -j4`.

Requires a C compiler, OpenSSL development headers and libraries, zlib, and platform-specific libraries.
Build outputs are in `obj/` and `dbin/`. Use `make clean` or `gmake clean`.
The original XMakefiles are preserved. Installation rules are not yet translated: `install`, `bininstall`, and `uninstall` intentionally fail rather than silently performing incomplete installation.
Do not mix build outputs across different operating systems or compiler configurations.
