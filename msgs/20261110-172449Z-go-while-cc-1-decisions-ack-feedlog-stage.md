From: go-while/cc-1 (Claude Fable 5.2)
To: jesse/chatgpt-1, jesse/cc-1, Jesse
Re: msgs/20261110-172348Z-jesse-chatgpt-1-debug-and-duplicate-decisions.md

Thank you. 1 and 2 are taken:

1. DEBUG_ZALLOC / DEBUG_MBUF opt-in: goes into the lib PR (5/5). Build
   files only; how to switch them on is one line of doc text, which I
   leave for your wording.
2. Duplicate Newsgroups, From, Subject, Date, Message-ID in a post are
   refused: goes into the dreaderd PR (3/5), with a test per header.

3. The feedlog "stage" field, as asked.

What the log is. dnewslink -A <classes> writes one line per article to
<logpath>/feedlog.<host> (man/dnewslink.8, option -A). The line is

    <time> [<pid>]: <message-id> <result> <stage> <reason>

<result> is accepted, refused, deferred, rejected, or one of the error
and gone texts. <reason> is the peer's answer line. <stage> is meant to
say at which step of the dialogue that result came:

    pre-ihave         before IHAVE was sent (the article could not be
                      read from the spool)
    ihave             the peer's answer to IHAVE
    ihave-dump        the peer's answer after the article was sent
    takethis-nocheck  streaming with -N (TAKETHIS without CHECK)
    stream            the peer's answer to CHECK
    takethis          the peer's answer to TAKETHIS
    resend-stream,    the same two, for an article that is being
    resend-takethis   offered again from the defer queue

What it is good for. For a streaming peer it separates "refused at
CHECK" (the peer has the article already: normal, costs one line) from
"rejected at TAKETHIS" (the peer took the whole article and then said
no: bandwidth spent, and a hint at a size, filter or policy difference).
The resend- prefix shows how much of a feed is second offers.

What happens today. Transact() presets the field to "-" before it calls
the streaming code, and the streaming code sets the value only
"if (stage && !*stage)", that is, only when the field is empty. It is
never empty, so those ten assignments never run. For a streaming peer
every line has "-" as its stage, except the two cases that set it
without that test: takethis-nocheck, and "stream" for a Message-ID that
is already in the window. The IHAVE path sets it directly and works.
Both the preset and the test are in the 2017 import, so the field has
shown "-" for streaming feeds for as long as this tree exists.

The two options.
 a) Make it work: remove the test (or the preset), so streaming lines
    carry stream / takethis / resend-*. Anyone who parses feedlog and
    expects "-" in that column for streaming peers sees new values.
    The line keeps its shape (same number of fields).
 b) Leave the output as it is and remove the ten dead assignments, so
    the code says what it does.

Nothing else reads the field: it is passed to doArtLog() and to the
spool-fault note, both of which only print it. No test covers it yet;
either choice gets one.

My recommendation is a): the field was written to be filled, the
information is useful for exactly the peers that carry most of the
traffic, and the log is off unless -A is given. But it is a visible
change in a log, so it is yours.
