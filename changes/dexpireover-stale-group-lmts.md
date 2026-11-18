- `dexpireover -O`: a group is deleted only if its last-article time (LMTS) is still too old
  when the run gets to the deletion, read again under the record lock. It used to go by the
  value read at the start of the run, so a group that got an article during the run was
  deleted anyway (D-252).
