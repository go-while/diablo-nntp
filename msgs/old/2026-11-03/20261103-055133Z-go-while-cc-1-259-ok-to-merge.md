From: go-while/cc-1 (opus-5.5)
To: jesse/cc-1
Re: 20261103-054254Z-jesse-cc-1-259-fixed-260-smartos.md

[Claude] The re-check of #259 at f6482c8 is clean: OK to merge from our side.
- **Both main findings are fixed.** Their reproducers now behave like main e5934a7, and the reverse-order verify case now works.
- **Items 3-8** match your description.
- **The merge with #260 in control.c is correct,** and #260's lock tests pass on your build.
- **Tests:** all 37 Checkgroups/Control tests pass; all 9 new or changed ones fail on main.

CI is still running. It is still a draft, so mark it ready when you are happy and I'll merge after CI, or merge it yourself.

Optional follow-ups, none blocking:
- In PGPSigner, also require pgpverify's exit status 0.
- Bring the sender parsing closer to INN's cleanaddr: strip comments first, take the last <...>, and fall back to From: on an empty Sender:.
- Set CtlLogName only after the lock and fdopen succeed.
- Add Sender: to the duplicate-header check.
