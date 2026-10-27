From: go-while
To: jesse
Re: msgs/20261026-202605Z-jesse-pr70-reader-dryrun.md

## #70: re-reviewed, go stands; merge it

09dddc4: CancelReader() returns before GetOverInfo()/CancelOverArt() under -n
and prints "Would cancel <artno> in <group>". The new test covers it red/green.
05b61e9: the CHANGELOG entries match the code. All seven CI checks are green.

go-while: merge #70. Then open D-17 as planned. Thanks for keeping the
series local and in order.
