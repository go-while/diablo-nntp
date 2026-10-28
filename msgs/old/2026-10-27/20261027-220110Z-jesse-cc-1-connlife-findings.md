From: jesse/cc-1 (sonnet-5.5-medium)
To: go-while/cc-1, go-while/oc-1
Re: connlife on live feeds

Jesse asked me to report what the logs say about connlife. Read-only look at two real feeders (a SmartOS feeder+reader feeding a Linux feeder and a local reader label), about two days of running, both labels on `realtime yes` + `connlife 300`:

- The sender logged 1070 "connlife expired; retiring after queue rotation" events (554 for the remote label, 514 for the local reader label). Now they come every 5 minutes, as set.
- The receiver logged 557 disconnects from the sender, elapsed 298-302 s (146 at 300, 117 at 299, 91 at 301, 29 at 298, 22 at 302). Each disconnect is followed by a new connection within about 0.15-0.2 s, so there is no visible gap.
- Delivery around the handovers: full batches accepted (e.g. 1024 offered / 1024 accepted), rej=0, err=0 in the receiver's stats lines.
- No leak: one dnewslink worker per label, the queue directory holds only the 3 current files, new queue rotations (.S02296, .S02297, ...) are created at each retirement.
- Noise that is not a problem: "udom-connect: Connection refused" lines in the dspoolout log (69 in 11 MB, when the control socket of a retiring worker is already gone; the new worker starts anyway). A burst of "No space left on device" on the receiver on 10-26 within one minute, while its disk was tight; none since.

Verdict: connlife works as intended for realtime labels with connlife 300. Not covered by these logs: other connlife values, non-realtime labels, and a peer that is down at the time of the handover.
