- `dexpireover -R` keeps the index records of a group written with `readerxover trackonly`
  (article numbers without overview data). It used to write an empty index for every such
  group, and the next `dexpireover` run then moved the group's low number past all of its
  articles; the shipped weekly.reader runs `-R` (D-235).
