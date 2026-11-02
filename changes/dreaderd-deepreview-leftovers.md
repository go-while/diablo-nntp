- dreaderd: three robustness fixes found in the deep read of the reader daemon.
  - `control.c` ParseControl(): a `log=` action whose file cannot be reopened no
    longer leaves `ct_LogFo` pointing at the handle it just closed. The handle is
    set to NULL after the close, so the temporary-file fallback runs, every later
    write to the control log goes to a valid file, and the handle is not closed a
    second time in FreeControl(). Reachable with two `log=` actions where the
    second file cannot be created. Test: `TestControlLogRuleReopenFailure`.
  - `group.c` GetOverInfo(): the OverInfo (and its group string) are freed on the
    early return taken when the active record cannot be read, and the `path`
    buffer is freed on the two overview-init error returns that previously skipped
    the shared free at the end of the function. `ov_OFd` is set to -1 right after
    the allocation so an early FreeOverInfo() cannot close(0). Removes a per-call
    SysMemPool leak on those error paths.
  - `spool.c` NNGetLocal1(): the filename taken from a spool server's `whereis`
    reply is name-checked before it is handed to open(). `localspool` must be an
    absolute path (an empty or relative value is refused: joining a relative reply
    under an empty root would otherwise form an absolute path); `localspool "/"`
    keeps its documented full-path mode where the backend's absolute replies are
    trusted and uncontained; under a real spool directory the reply must stay
    inside it by name (no leading "/" and no ".." component), and a path that would
    truncate is refused. This is lexical containment of the requested name, not a
    filesystem sandbox: a backend trusted to serve the spool can still open through
    a symlink planted inside the spool tree, so the spool tree stays a trust
    boundary (descriptor-relative containment with openat()/O_NOFOLLOW is a
    separate follow-up). A refused name falls through to the normal article fetch.
