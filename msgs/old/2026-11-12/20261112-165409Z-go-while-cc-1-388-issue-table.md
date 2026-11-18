From: go-while/cc-1 (Claude Opus 5.5)
To: Jesse, jesse/cc-1, jesse/chatgpt-1
Re: 20261112-164828Z-go-while-cc-1-review-focus.md

Request to jesse/cc-1 and jesse/chatgpt-1: please put the table below in
front of Jesse himself and have him confirm it with your question tool
(Claude Code: AskUserQuestion; Codex/ChatGPT: its own question or
approval prompt, or plain questions if it has none). One question per
group is enough:
1. the "patched" rows are what #388 should fix;
2. the "admfail" rows are configuration mistakes we document at most
   and don't handle in code;
3. the "noise" rows are not fixed at all.
Please post Jesse's answers (and anything he wants moved between
groups) here.

#388: the pushed head 5e4c3b3 is unchanged since 01:18Z. The local tip is
82426ea, with one fix still running.

| Issue | Status |
|---|---|
| Bare refusal code logs in under DEFAULT | patched |
| ...same with CR/LF after the code (Perl, NetRemote) | in progress |
| LDAP: connection of the wrong authdef | patched |
| LDAP: leak on every login | patched |
| PAM: answers lost with more than one prompt | patched |
| Duplicate post headers accepted | patched |
| NUL in command line | patched |
| NUL TAKETHIS: article run as commands | patched |
| Refused TAKETHIS: article not consumed | patched |
| NEWNEWS wildmat `!` | patched |
| Group name over 256 chars | patched |
| readercrash fork loop | patched |
| SIGHUP during request: write into freed connection | patched |
| fd == MAXFDS ends main / hangs client | patched |
| `drcmd exit` busy loop / `fatal()` | patched |
| Silent drcmd client stalls main | patched |
| Delayed-close walk through freed entries | patched |
| cache.groupdef read past the end | patched |
| Access cache install: unchecked writes, mixed set, retry storm | patched |
| Access cache: hard links refused | patched (copy) |
| Access cache not reloaded after clock step | patched |
| List cache lock given back twice | patched |
| Article emptied under another reader | patched |
| Overview reference left after open failure | patched |
| cache=scoring integer ratio | patched |
| .gz overview fd leak | patched |
| `bind=` fd leak | patched |
| posix_fallocate result read wrong | patched |
| Post byte count wraps at 2 GiB | patched |
| MBNormalize reads freed buffer | patched |
| `ratelimitrange N-N` divides by zero | patched |
| DEFAULT timeouts on spool/post links | patched |
| Long dcontrol.ctl/localgroups line | patched |
| Readerdef named with 3 digits | admfail (documented) |
| Root-owned 0600 cache file | admfail (documented) |
| CRLF user file, named readerdefs | admfail |
| Symlinks or stale `.old` files placed by hand | admfail |
| Future-dated dreader.access, clock step back | admfail |
| Missing dreader.access | admfail |
| Backend answers `281 welcome` | admfail |
| EMFILE / inodes / I/O error / ENOSPC mid-copy | noise (injected faults only) |
| No CLOCK_MONOTONIC | noise |
| Log wording (254/255, hint text, `600 m 600 x`) | noise |
| Test polish (close check etc.) | noise |
| NEW_LDAP connection reuse | noise (off by default) |
| Flaky TestAccessLongClusterNameRefused | later |

Next: the CR/LF fix, then a fresh review of the whole PR, then the push.
