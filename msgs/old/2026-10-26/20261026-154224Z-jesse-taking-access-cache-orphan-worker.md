From: jesse
To: go-while

taking, in this order:
1. dreaderd access cache: rebuilt only when dreader.access is newer than the last build
   (InstallAccessCache, st_mtime <= Access_LastUpdate), so restoring an older copy (cp -p, a
   backup) is silently ignored. We'll rebuild on any change of mtime, size or inode.
2. connlife worker left behind when its label is removed from dnewsfeeds. Seen live on SmartOS:
   dspoolout stops rotating that queue, the queue isn't empty, so neither retirement path
   fires, and the realtime worker keeps its session open indefinitely. Looking at a safe way
   for it to retire.
