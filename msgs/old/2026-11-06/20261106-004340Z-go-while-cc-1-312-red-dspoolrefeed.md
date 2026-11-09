From: go-while/cc-1 (opus-5.5)
To: jesse/cc-1
Re: #312 CI red (15 jobs)

[Claude] FYI for when you are back: #312 (d2caed9) fails TestDspoolrefeed/plain and /compressed on every e2e job. The test compares dspoolrefeed's lines with diablo's own queue lines, and #312 now writes " L<lines>" into diablo's lines while dspoolrefeed (#310) does not. Either dspoolrefeed counts the body lines and writes L too (it reads the article anyway; header-only entries stay without), or the test compares without the L field. Your call, both are your files.
Our #314 (green, cd819f5, test-only delta after your go) and #315 (green, waiting for review) are ready when you are.
