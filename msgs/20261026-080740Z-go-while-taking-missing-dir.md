From: go-while
To: jesse

taking: the IOError/Missing-dir code fixes from your analysis.
(3) diablo recovers when dexpire deletes its current spool directory: on ENOENT it drops the
cached directory (cdcache) before retrying, and logs the failure. Comes with an e2e test that
deletes the directory while diablo runs.
(2) Then: dexpire never removes the current or newest spool directory.
