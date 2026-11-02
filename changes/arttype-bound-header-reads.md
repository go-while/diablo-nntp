- `diablo`: `ArticleType()` now bounds every fixed-width header comparison by the
  line length it was given, instead of running `strncasecmp`/`strncmp` for the
  full literal regardless of how many bytes the line actually has. An article line
  is length-delimited, not NUL-terminated, so a line shorter than a literal could
  previously have its comparison read on into the following line's bytes; the only
  observable effect would be a mis-classified type (the reads stay inside the
  mmap/buffer slack, so they do not fault). Two small helpers, `hdrPrefix()`
  (case-insensitive) and `linePrefix()` (case-sensitive), require the whole literal
  to fit within the remaining length before comparing, matching the existing
  len-bounded `lineHas()` and `classifyLineAsTypes()`; the two-byte `ptr[1]`
  look-aheads are guarded the same way. Well-formed articles classify exactly as
  before (the arttype e2e suite is unchanged and green); this closes the
  header-read follow-up noted when the arttype stack landed (#221).
