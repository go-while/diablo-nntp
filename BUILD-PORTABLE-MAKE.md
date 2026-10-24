# Diablo 6 — Portable Make Build Guide

Diablo 6 supports building with GNU Make on Linux, FreeBSD, and illumos-based operating systems, as well as native BSD Make on FreeBSD.

This build system replaces the requirement for Matt Dillon's `xmake` while preserving the original XMakefiles for reference.

## Supported Platforms

| Operating System | Make Implementation | Compiler | Build Command | Status |
|---|---|---|---|---|
| FreeBSD | BSD Make | Clang | `make -j4` | Validated |
| FreeBSD | GNU Make | Clang | `gmake -j4` | Previously validated |
| SmartOS (illumos) | GNU Make | GCC | `gmake -j4` | Validated |
| OmniOS (illumos) | GNU Make | GCC | `gmake -j4` | Validated |
| Linux | GNU Make | GCC | `make -j4` | Not yet validated for Diablo 6 |

Traditional Solaris Make is not supported. On illumos systems, use GNU Make (`gmake`) rather than the system's native `make` when it is Solaris Make.

## Build Requirements

A working C compiler and the appropriate Make implementation are required.

Core dependencies include:

- OpenSSL development headers and libraries (`libcrypto`).
- zlib development headers and libraries.
- Standard system development headers and libraries.
- Berkeley DB development files, when `DB_ENABLED` is defined.
- RADIUS development files, when `RADIUS_ENABLED` is defined.

The availability and installation commands for these dependencies vary by operating system and package repository.

### Optional Features

Optional functionality is controlled through `lib/vendor.h`.

**Berkeley DB**

When `DB_ENABLED` is defined, Berkeley DB support is enabled. On illumos, the GNU Makefile automatically detects a compatible Berkeley DB 1.85 API header and library.

The detector supports Berkeley DB 4 and 5 installations, including pkgsrc and OmniOS/OOCE layouts.

When `DB_ENABLED` is not defined, Berkeley DB detection and linking are skipped.

**RADIUS**

When `RADIUS_ENABLED` is defined, the build includes RADIUS library support.

When it is not defined, the RADIUS linker dependency and associated illumos compatibility object are omitted.

Feature detection currently relies on matching active-looking `#define` directives in `lib/vendor.h`. It does not fully evaluate nested preprocessor conditions.

## Building Diablo

### FreeBSD — Native BSD Make

```sh
make -j4
```

The `BSDmakefile` is selected automatically by FreeBSD's native Make implementation.

### FreeBSD — GNU Make

```sh
gmake -j4
```

GNU Make uses the top-level `Makefile`.

### SmartOS and OmniOS

```sh
gmake -j4
```

GNU Make detects illumos through the `SunOS` operating-system identifier and applies the appropriate platform-specific configuration.

### Linux

```sh
make -j4
```

Linux support is included in the GNU Makefile but has not yet been validated against the current Diablo 6 source tree.

## Build Output

Compilation generates files in two directories:

| Directory | Contents |
|---|---|
| `obj/` | Object files, static libraries, generated prototype headers, dependency files, and detected build configuration |
| `dbin/` | Compiled Diablo executables |

Generated files are not intended to be committed to Git.

## Automatic Berkeley DB Detection on illumos

The Berkeley DB compatibility detector is located at:

`compat/detect-db185.sh`

It searches common installation paths for `db_185.h` and compatible libraries, then compiles and links a small test program to validate a candidate.

The detector has been tested successfully on SmartOS and OmniOS.

### Detected Configurations

| Distribution | Header Directory | Library |
|---|---|---|
| SmartOS / pkgsrc | `/opt/local/include/db4` | `-ldb4` |
| OmniOS / OOCE | `/opt/ooce/include` | `-ldb` |

The detector also searches Berkeley DB 5 locations, including `/opt/local/include/db5`.

When Berkeley DB support is enabled, the Makefile generates:

`obj/db185-config.mk`

This file contains the selected compiler and linker flags.

To inspect the detected configuration:

```sh
cat obj/db185-config.mk
```

To run detection independently:

```sh
CC=gcc sh compat/detect-db185.sh
```

The generated configuration is machine-specific and should not be committed to Git.

If the installed Berkeley DB version, compiler, or library paths change, run a clean build to force rediscovery.

## illumos Compatibility

The build includes compatibility code for interfaces required by Diablo or its third-party dependencies.

### RADIUS Header Compatibility

`compat/illumos_radlib.h` supplies BSD compatibility definitions required by some pkgsrc RADIUS headers.

The compatibility header is included conditionally on illumos.

### srandomdev Compatibility

`compat/srandomdev.c` implements the BSD `srandomdev()` interface required by certain static RADIUS libraries.

The implementation seeds `random()` using `arc4random()`.

The compatibility object is linked into the reader daemon when RADIUS support is enabled on illumos.

### Berkeley DB Compatibility

Diablo uses the Berkeley DB 1.85 API, including `dbopen()`.

On illumos, `dreaderd/dns.c` includes `db_185.h` to access the compatibility API provided by supported Berkeley DB libraries.

### Platform-Specific Paths

Berkeley DB headers and libraries are discovered automatically. RADIUS library detection uses common illumos installation paths.

Nonstandard installations may require additional configuration.

## Incremental and Parallel Builds

Both GNU Make and native FreeBSD Make support parallel compilation.

Examples:

```sh
gmake -j4
```

```sh
make -j4
```

Compiler-generated dependency files are used to track header dependencies.

Generated prototype headers and static libraries are also managed through Make dependencies.

A successful incremental build should not recompile or relink unchanged targets.

To inspect the commands Make would execute:

```sh
gmake -n
```

Or on FreeBSD with native Make:

```sh
make -n
```

If the build is up to date, no compilation or linking commands should appear.

**Archive timestamps:** Source archives should preserve valid Unix timestamps. Source files with modification times in the future can cause repeated recompilation.

Do not share the same `obj/` and `dbin/` directories between different operating systems or compiler configurations.

## Cleaning the Build

Remove generated build outputs:

```sh
gmake clean
```

Or with FreeBSD's native Make:

```sh
make clean
```

The `distclean` target is also available.

Cleaning removes generated objects, libraries, executables, dependency files, and detected configuration files.

It does not remove source files or the original XMakefiles.

## Installation

The portable build system preserves Diablo's original installation layout.

The default installation prefix is:

`/news`

Both GNU Make and native FreeBSD Make use the shared installation script:

`install-portable.sh`

### Default Installation Layout

| Component | Default Path |
|---|---|
| Executables | `/news/dbin/` |
| Sample configurations | `/news/samples/` |
| Configuration symlinks | `/news/` |
| Article spool | `/news/spool/` |
| Queue | `/news/dqueue/` |
| Runtime files | `/news/run/` |
| Logs | `/news/log/` |
| Manual pages | `/news/man/man5/` and `/news/man/man8/` |

### Full Installation

```sh
gmake install
```

On FreeBSD with native Make:

```sh
make install
```

A live installation requires appropriate permissions and the configured `news` user and group.

### Binary Installation

```sh
gmake bininstall
```

This installs the applicable files without creating default configuration symlinks, making it suitable for updating an existing installation.

### Custom Installation Prefix

Override the default `/news` installation directory:

```sh
gmake PREFIX=/opt/diablo install
```

### Staged Installation

Use `DESTDIR` to stage an installation without modifying the live filesystem:

```sh
gmake PREFIX=/news DESTDIR=/tmp/diablo-stage install
```

The resulting installation is located under:

`/tmp/diablo-stage/news/`

Staged installations intentionally skip ownership changes.

### Ownership

For live installations, the installer applies the configured owner and group, defaulting to `news:news`.

For staged installations using `DESTDIR`, ownership changes are skipped to support packaging and testing without requiring root privileges.

### Uninstallation

```sh
gmake uninstall
```

For a staged installation:

```sh
gmake PREFIX=/news DESTDIR=/tmp/diablo-stage uninstall
```

The uninstall process is designed to preserve operational data, including article spools, history, queues, logs, and locally modified configuration files.

Review the installation prefix and destination carefully before running installation or uninstallation commands on a production server.

## Validation Status

The portable build system has been tested on:

- FreeBSD with native BSD Make.
- SmartOS with GNU Make and GCC.
- OmniOS with GNU Make and GCC.

Successful compilation, incremental builds, and installation functionality have been confirmed during development.

Linux compilation for the current Diablo 6 revision remains unverified.

Compilation success does not guarantee runtime compatibility. Network operations, authentication, Berkeley DB access, spool handling, and daemon startup should be tested before production deployment.

## Original Build System

The original Matt Dillon `xmake` configuration files remain in the source tree for reference.

The portable Makefiles are maintained separately:

- `Makefile` — GNU Make.
- `BSDmakefile` — Native FreeBSD Make.

Changes affecting source files, optional dependencies, or executable targets should be reflected in both build systems where applicable.

## Future Work

The following improvements remain candidates for subsequent development:

- Validate Linux compilation and runtime behavior.
- Review compiler warnings, prioritizing memory safety and correctness.
- Improve optional-feature detection using the C preprocessor rather than text matching.
- Expand automatic discovery of RADIUS dependencies across illumos distributions.
- Add automated build testing for supported operating systems.
- Review runtime library paths and dependency packaging.

The portable build system should be maintained independently of broader source-code modernization to simplify testing and regression analysis.

