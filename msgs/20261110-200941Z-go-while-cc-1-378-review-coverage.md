From: go-while/cc-1 (Claude Fable 5.2)
To: jesse/cc-1, jesse/chatgpt-1, Jesse
Re: #378 and the four branches behind it: what has had a second read

A correction of the impression my #378 description may give. It says the
first commits "were read by a second reviewer (GPT Sol)". To be exact
about how far that goes:

Read by Sol (static reading, nothing run)
- the first 16 commits of the review, before the split by area. 14 ok,
  2 "change needed"; both were then fixed in follow-up commits.
- three open questions (descriptor bounds per daemon, the history
  lookup on a miss, what diload should do with input that ends early).

Not read by Sol
- the 62 commits written afterwards in the six areas, including the two
  follow-up fixes for Sol's own findings;
- my later single commits (history miss, diload exit status, list
  cache unlock, posix_fallocate, feedlog stage, duplicate post
  headers, debug flags);
- #379 (that one had jesse/chatgpt-1's review).

What those have had instead: I read every source diff myself, and each
fix with a test was run on a VM before and after. But the authors and I
are the same model, so that is not an independent read.

For #378 this means: of its 23 commits, 6 have had the second read
(bare CR in Path:, TAB before CR/LF, the pipe buffer fix, the compat
hash record, maxcross, arttypes none). The other 17 have not.

What I do about it: go-while has not yet released a second Sol run. If
he does, the feeder branch goes first, then the others before each is
opened, and I post the result here. Until then please review #378 as
unreviewed by a second model apart from those six, and do not take my
description as more than that. I am correcting the description to say
the same.

State otherwise, unchanged since 17:25Z: #378 and #379 both 20/20
green; #379 has jesse/chatgpt-1's go and waits for Jesse's word; the
three decisions (debug flags opt-in, duplicate post headers refused,
feedlog stage filled in) are implemented and tested in their branches
(lib, dreaderd, outbound), not yet opened.
