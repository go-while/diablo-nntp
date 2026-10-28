# Security Policy

## Supported Versions

Diablo 6 is developed on the `main` branch of
[jrehmer/diablo-nntp](https://github.com/jrehmer/diablo-nntp). Security fixes go
into `main` and the next build identifier (currently `6-CUR-20261024-BWH`, see
[CHANGELOG.md](CHANGELOG.md)).

| Version | Security support |
| ------- | ---------------- |
| `main` and the latest 6-CUR build | :white_check_mark: |
| Older 6-CUR builds | :x: (upgrade to the latest first) |
| Diablo 5.x and earlier, and other Diablo forks | :x: |

If you run an older build, please check whether the problem is still present
in `main` before reporting it; it may already have been fixed.

## Reporting a Vulnerability

Please do **not** report security vulnerabilities through GitHub issues, pull
requests, discussions, Usenet newsgroups or mailing lists: they are all
**public**.

Report them **privately** through
[GitHub Security Advisories](https://github.com/jrehmer/diablo-nntp/security/advisories/new)
for this repository. This opens a private report that only you and the
maintainers (and the collaborators they add) can see until it is resolved and
disclosed.

Please include as much of the following as you can:

- what the vulnerability is and its impact (for instance remote code
  execution, memory corruption, a buffer overrun, an authentication or access
  bypass, a forged or unverified control message, article loss or spool
  corruption, denial of service);
- the affected program(s) (`diablo`, `dreaderd`, `dnewslink`, `dspoolout`,
  `pgpverify`, a utility in `util/`, an `adm` script, ...) and the build or
  commit you tested, ideally the commit that introduced the problem;
- the configuration it needs (`diablo.config`, `dnewsfeeds`, `dreader.access`,
  `dcontrol.ctl`, ...);
- step-by-step instructions to reproduce it, or a minimal proof of concept;
- a patch, if you have one;
- your assessment of the severity and of what an attacker needs, for instance:
  - any host that can connect, before any authentication;
  - an authenticated reader (`dreader.access`);
  - a peer listed in `dnewsfeeds` (a trusted feed);
  - an article that reaches the server through any feed, such as a control
    message, which can come from anywhere on Usenet;
  - local access to the news user or to the news spool.

Please do not disclose the issue publicly before a fix is available. If you
have several unrelated issues, please open one advisory for each.

### Reporting from a script

Private vulnerability reporting is enabled for this repository, so a report
can also be submitted through the GitHub API instead of the web form. Anyone
can submit one; it goes to the maintainers for triage, just like the web form.
`gh` must be logged in:

```sh
gh api -X POST repos/jrehmer/diablo-nntp/security-advisories/reports \
    -f summary='dreaderd: one-line title' \
    -f description="$(cat report.md)"
```

### Collaborators and their agents

The maintainers and the collaborators they have already invited (people and
the AI agents working for them) share embargoed findings, reviews and fix
notes on a private message board, the GitHub repository
`go-while/nntp-diablo-msgboard`. It is not a reporting channel: if you are
not already a collaborator, report as described above instead of asking for
access. It follows this policy: nothing from it goes to a public place before
the advisory is published. Agents file each verified finding through the API
route above (one report per issue) instead of asking a person to copy it
into the web form; the maintainers triage the reports and decide which
become advisories.

## What to Expect

- The maintainers will acknowledge your report within a few days.
- We will investigate, keep you informed, and work on a fix in the private
  advisory (with a private fork where needed).
- Once a fix is ready, we will agree with you on a date to merge it and
  publish the advisory, and request a CVE where appropriate.
- You will be credited in the advisory and the changelog, unless you prefer to
  remain anonymous.

## Scope

This policy covers the source code in this repository: the daemons and
utilities, the libraries in `lib/`, the sample configuration and `adm`
scripts, and the build and install scripts.

Problems in third-party software that Diablo uses (OpenSSL, GnuPG, zlib,
Berkeley DB, the C library, the operating system) should be reported to those
projects. If Diablo uses such software in an unsafe way, that is in scope here.
