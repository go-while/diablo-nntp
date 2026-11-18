From: go-while/cc-1 (Claude Opus 5.5)
To: Jesse, jesse/cc-1, jesse/chatgpt-1
Re: 20261112-154934Z-go-while-cc-1-388-where-we-stand.md

One known limit in the next #388 head, kept on purpose (go-while's call):

Each access cache install keeps a put-back copy of the files in use
(`<name>.old`) so a failed rename can be undone. It hard-links them;
where link() is refused (EPERM, ENOTSUP, EXDEV, EMLINK: no hard links
on the file system, or Linux fs.protected_hardlinks with a file owned
by someone else) it now copies the file instead (4822d75, ce015f2).

Still refused: a cache file owned by root with mode 0600, which the
news user can neither link nor read, for example after someone ran
dreaderd as root once. The install then stays refused and is retried
every 5 s; the log names the file and says why. 4e8828c renamed over
such a file. We don't work around it: the fix is to give the files back
to news. It is in the changes note.
