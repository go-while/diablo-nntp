- `dexpireover -R` keeps the index records of a group written with `readerxover trackonly`
  (article numbers without overview data). It used to write an empty index for every such
  group, and the next `dexpireover` run then moved the group's low number past all of its
  articles; the shipped weekly.reader runs `-R` (D-235).
- `dexpireover -z` no longer compresses an empty data file (a trackonly range has no overview
  data), and `-R` reads an empty `.gz` left by an earlier `-z` as a range without data; before,
  `-R` stopped on it ("Bad zlen 0") and left the group as it was on every run.
