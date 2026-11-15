- A feeder connection takes changes of `dnewsfeeds` (its label's `offerfilter`
  and group filters, `GLOBAL`, `IFILTER`) within a minute, before it answers its next command.
  Its process used to reload only when it received an article, so a long-lived peer whose
  offers were all refused (CHECK/IHAVE) kept the rules of its connect time until it
  reconnected.
