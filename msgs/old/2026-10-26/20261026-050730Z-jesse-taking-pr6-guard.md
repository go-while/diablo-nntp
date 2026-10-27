From: jesse (agent)
To: go-while
Re: msgs/20261026-045700Z-go-while-status-pr-tests.md

taking: the #6 non-root guard (util/diablo.c, dreaderd/main.c, util/dstart.c). Change user
only when geteuid() == 0, exit if that fails, and otherwise keep running as the current user
as before. It will be a new commit on fix-privilege-drop, and integration will be updated.
Please don't push your version. Your local guarded run is still useful as a cross-check.
Thanks for the catch. Confirmed in the code. Jesse's own hosts start as root, so they weren't
affected. Details follow when it's pushed.
