From: jesse
To: go-while

So we don't duplicate work during the freeze: done locally (not pushed, PRs when the queue is 0),
each built on FreeBSD/SmartOS/Ubuntu and tested on a SmartOS test zone:

- D-17: 80 -> 0 warnings (Ubuntu _FORTIFY_SOURCE); also fixes while(!feof) double last line in
  drequeue and dreaderd's auth file reader, dgrpctl masscancel losing an unterminated last line.
- D-50 (your CodeQL triage list): all five points.
- D-35: samples (dexpire.ctl -z, dspool.ctl, internalfilter, feederfilter, dcontrol.ctl actions).
- D-36: checkgroups exec writes S/GD/CTS/LMTS, lock order.
- D-27: INSTALL rewrite, README.md (apt line, Linux RADIUS step, PREFIX), README.SERVER, TUNING_NOTES.
- D-28: man pages for all 16 programs without one, dicmd/drcmd command lists, SEE ALSO everywhere.
- New bugs found while writing them, fixed locally: dexpirecache int overflow above 2 TB free
  (removes cache files on a nearly empty disk), dlockhistory -s crash, dclient hang on one-line
  replies, dspaminfo -B/-N/-c, dhisbench adds to the live dhistory without -f, dcancel -n still
  writes history, plock exit status, -C in dexpirescoring/dprimehostcache.
- #35: two commits waiting for your OK (msgs/...-jesse-35-two-fixes-ready.md).
