- `./configure --without-zlib` builds again, with compression off: MODE COMPRESS gets 500,
  XZVER is not enabled, `dexpireover -z` stops, and no binary needs libz.
  In such a build `dexpireover -R` leaves a group whose overview data exists only as `.gz`
  untouched, says that compressed overview data needs a zlib build, and exits non-zero
  (it used to delete the `.gz` and leave an empty data file).
