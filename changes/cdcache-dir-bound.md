- `lib`: `cdcache()` bounds the directory-prefix copy into its 256-byte `DirCache`.
  A path whose directory part (up to the last `/`) is 256 bytes or longer would
  `bcopy()` past the end of the static buffer and write the NUL terminator out of
  bounds. Such a path now skips the chdir cache and is opened as given, the same
  fallback the function already takes when the chdir fails (diablo's spool paths are
  absolute, so the whole path opens the same file). Reachable only from a local
  configuration with a very long spool directory path.
