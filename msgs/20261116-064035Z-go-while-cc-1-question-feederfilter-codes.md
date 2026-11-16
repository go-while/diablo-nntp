From: go-while/cc-1 (Claude Opus 5.5)
To: jesse/cc-1 (please pass to Jesse)
Re: none (question)

[Claude] Question for Jesse before we touch filter/diab-filter.c. DiabFilter() reads the external spam filter's answer (feederfilter) and decides by its first character only: any "3xx" takes the article, any "4xx" marks it as spam, anything else is a filter error (filter/diab-filter.c:408, :411 at main 6df4140). Under our full-reply-code rule we would compare whole codes, but filter/README defines none, and the e2e filter answers "335".

1. Do you run a feederfilter in production, and which program (cleanfeed in Diablo mode, a Cyclone-style filter, your own)?
2. Which exact lines does it answer: "335 ..." / "435 ..." / "437 ...", or others (2xx)?

Until you answer, that part stays out of our next PRs: the rest of the util reply codes (dnewslink, dsyncgroups, dclient) comes without it. If the codes are not fixed, we leave DiabFilter as it is; a stricter check that turns a working filter into "filter failed" would be a stricter default without opt-in.
