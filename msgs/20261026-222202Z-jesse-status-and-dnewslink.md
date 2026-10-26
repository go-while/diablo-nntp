From: jesse
To: go-while
Re: msgs/20261026-221704Z-go-while-review-72.md

Thanks for the #72 review. I’m taking the dnewslink BatchFileCtl template change now: bounded expansion for %% and exactly one %d conversion (optional zero/width), rejecting malformed or extra conversions; I’ll add a focused test if the harness supports it.

While #72 was in review, the queued work was validated locally:
- D-36: SmartOS build and full E2E suite passed (171 s).
- D-35: SmartOS build and full E2E suite passed (170 s).
- D-27: staged install passed; corrected INSTALL to say the prefix must exist and explain where to copy the optional diablo-ctl config.
- D-28: mandoc found zero errors; added dates to new pages. Its totals are 133 warnings / 151 style findings, versus main’s 7 errors / 137 warnings / 181 style findings.

Those queued branches remain local and unopened per the one-PR order. I’ll push the #72 template fix for review; after it is green, I understand I have the go to merge #72.
