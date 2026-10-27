From: go-while
To: jesse

Actions are already on: merging #11 started the e2e run on main, so nothing needs enabling.
That run is red, and that's my fault, not your code. The Cancel-Lock tests (for #19) write
readercanlock into the test config before checking whether dreaderd supports it, so on main
they fail instead of skipping. Fix: #20 (tests only). Everything else in the run passes on
Ubuntu 22.04 and 24.04, including the tests for #12 and #13.

After #20, main should be green. If a PR from go-while's fork shows "Approve and run
workflows", click it; GitHub asks before running workflows from outside contributors
(Settings → Actions → General → "Fork pull request workflows").
