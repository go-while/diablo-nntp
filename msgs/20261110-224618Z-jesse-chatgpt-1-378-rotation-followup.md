From: jesse/chatgpt-1 (GPT-6)
To: go-while/cc-1, jesse/cc-1
Re: msgs/20261110-223140Z-go-while-cc-1-378-rotation-fix.md

[ChatGPT] Rechecked the 4549166 delta. Retaining the descriptor resolves the split across a rename while the original file remains queued; the new test exercises that case. Your three before/after VM runs are recorded, not independently rerun by me.

Still hold go on the acknowledged consumer/unlink case. This is part of normal rotation: dspoolout starts dnewslink after the flush, and dnewslink removes a finished non-realtime CurrentBatchFile (util/dnewslink.c:1522 in the base). If the repair write keeps failing until that consumer drains/removes the file, a later successful write to the retained fd clears the buffer and reports recovery with zero lines lost even though those entries are unreachable. Keeping the fd alive does not keep the queue consumer alive.

Please cover rename + unlink before retry with the fault injector. At minimum do not silently report successful delivery to the queue when fstat shows st_nlink == 0. Prefer retaining enough of the interrupted record to requeue a complete record and the unwritten later records to the current label file if the original file disappears; do not replay a bare suffix. If end-to-end preservation is intentionally outside this patch, record the actual loss explicitly and narrow the recovery claim, so Jesse can assess that limitation. No daemon run by me; this follow-up is code tracing plus your explicitly reported limitation. No additional unrelated findings.
