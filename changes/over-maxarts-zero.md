- `diablo`: no divide-by-zero on a header-only overview file, across every caller.
  `maxarts`, computed as `(filesize - headsize) / sizeof(OverArt)`, is 0 for an over
  file that is only a header (no article slots), and the next `artno % maxarts` was a
  SIGFPE. dexpireover already guarded this (#162); this extends the same guard to the
  paths that were missed:
  - `GetOverInfo()` in the reader daemon (`dreaderd/group.c`), in `dgrpctl`
    (`util/dgrpctl.c`) and in `dcancel` (`util/dcancel.c`) now treat a file with no
    room for even one `OverArt` as new/empty and re-initialise it, so `ov_MaxArts` is
    positive before any `GetOverArt()` (a header-only file holds no articles, so the
    group self-heals rather than being rejected);
  - the inline over-file scans in `dgrpctl groupinfo` and `dreadover` refuse such a
    file with a message instead of dividing by it.
  Two robustness points found in review and folded in: the re-initialisation decision
  now uses the file size `fstat`'d after taking the exclusive lock, not the stale size
  from before it (so a concurrent `WriteOverview` that added the first record is not
  truncated away); and an `initarts` of 0 **or less** from `dexpire.ctl` falls back to
  the default instead of producing a negative slot count.
  Tests: `TestGroupinfoHeaderOnlyOver`, `TestMasscancelHeaderOnlyOver` (pins the
  re-init), `TestDcancelHeaderOnlyOver`, `TestDreadoverHeaderOnlyOver` — each reproduced
  the SIGFPE before the fix.
