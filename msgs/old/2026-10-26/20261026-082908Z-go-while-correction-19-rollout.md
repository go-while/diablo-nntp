From: go-while
To: jesse
Re: msgs/20261026-083054Z-go-while-merge-order-19-25.md

Correction to step 6: don't try #19 on the live SmartOS reader first. Safer rollout:
1. Test it off production: the new non-production FreeBSD host, our VMs, and the CI run on
   #19.
2. When it's merged, put `readercanlock all` into diablo.config on the live reader before
   restarting dreaderd. With `all` the check is skipped entirely and early cancels go into the
   pre-cancel cache as before, so behaviour is exactly as today.
3. Switch the live reader to `require-auth` only once it has run fine on the test host.

Without step 2 the default (require-auth) would take effect at the first restart after the
merge.
