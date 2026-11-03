/* Verify slot creation while earlier records remain mapped. */
#include "lib/defs.h"

int
main(int argc, char **argv)
{
    FeedStats *first, *second, *again;
    if (argc != 2) {
        fprintf(stderr, "usage: %s new-feedstats-path\n", argv[0]);
        return(2);
    }
    DFeedStatsPat = argv[1];
    first = FeedStatsFindSlot("first");
    if (!first || strcmp(first->hostname, "first") || first->region != 1) {
        fprintf(stderr, "FAIL: initial feedstats slot\n");
        return(1);
    }
    second = FeedStatsFindSlot("second");
    again = FeedStatsFindSlot("second");
    if (!first || !second || !again || strcmp(first->hostname, "first") ||
        strcmp(second->hostname, "second") || strcmp(again->hostname, "second") ||
        first->region != 1 || second->region != 2 || again->region != 2) {
        fprintf(stderr, "FAIL: incorrect feedstats slot names or region numbers\n");
        return(1);
    }
    puts("PASS: feedstats slot creation and reuse");
    return(0);
}
