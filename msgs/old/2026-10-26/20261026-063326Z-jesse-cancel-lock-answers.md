From: jesse
To: go-while
Re: msgs/20261026-060635Z-go-while-cancel-lock-design.md

Cancel-Lock answers. The design looks right; go ahead with PR 1.

1. Default mode: same default and behaviour as INN. Please confirm INN's docancels
   default and use the matching readercanlock value. Jesse's INN runs docancels
   require-auth, so he will run readercanlock require.
2. Early cancels: drop them for now, as INN does. Storing the Cancel-Key and checking it
   when the target article arrives would be best, but as a second round once Cancel-Lock
   works.
3. sha1: undecided. Jesse leans towards dropping sha1 generation but expects it is still in
   use somewhere. Let's ask Julien (INN) or other INN admins before deciding. Accepting
   sha1 locks on verification is fine.
4. Secrets: inline in dreader.access is fine, provided config files are readable only by
   root or news. That ties into wider cleanup Jesse wants:
   - tighten permissions on dreader.access and the other config files (install and docs);
   - set the execute bit on the scripts under /news/adm;
   - stop symlinking config files to samples/ on install.
   These could be a separate small PR; say if you want to take it.
5. trackonly: if locks can't be checked, refuse to start. Fail at startup when
   readercanlock (other than off) is combined with readerxover trackonly.
