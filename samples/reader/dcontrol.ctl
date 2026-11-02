##  dcontrol.ctl - access control for control messages (DIABLO)
##
##  Roughly:	this is the same as INN's control.ctl except you must 
##  		specify what to do with cancel's and a few new actions
##		have been added.
##
##  Format:
##	<message>:<from>:<newsgroups>:<action>[=log][,<action>[=log]]*
##  The last match found is used.  Several lines can match one message, and
##  the last "=log" before a break decides where it is logged (a line without
##  "=log" keeps the log of the line before it).  Put a catch-all
##  line such as "all:*:*:log=control.log" before the lines of the message types,
##  never after them, or it replaces their log.  During rule matching, the
##  <newsgroups> pattern is tested only for newgroup and rmgroup.  For other
##  messages, including checkgroups, it does not restrict which rule matches.
##  For checkgroups, the selected rule's pattern also limits which groups the
##  action may change; see the checkgroups notes in samples/dcontrol.ctl.
##	<message>	Control message or "all" if it applies
##			to all control messages.
##	<from>		Pattern that must match the From line.
##	<newsgroups>	Pattern that must match the newsgroup being
##			newgroup'd or rmgroup'd.  For checkgroups it limits
##			the groups that may be changed; it is ignored
##			for other messages.
##	<action>	What to do:
##	    doit			Perform the action
##	    doit=xxx			Perform it; log to xxx (see below)
##	    drop			Ignore the message
##	    log				Nothing by itself; use log=xxx
##	    log=xxx			Log to xxx (see below)
##	    mail			Mail the message's log to the news admin
##	    verify-pgp_userid		Do PGP verification on user (GnuPG
##					gpgv by default; see README.PGP).
##	    verify-pgp_userid=logfile	PGP verify and log.
##	    break			terminate dcontrol.ctl scan
##
##			xxx=/full/path logs to /full/path; xxx=foo logs to
##			foo in path_log (/news/log/foo, no .log is added).
##			An empty xxx logs nothing.
##
## --------------------------------------------------------------------------
##	DEFAULTS
##
## Get cancel up there in the front so we can terminate dcontrol.ctl 
## processing. 

cancel:*:*:doit,break
all:*:*:log=control.log
checkgroups:*:*:log=checkgroups.other
ihave:*:*:drop
sendme:*:*:drop
sendsys:*:*:drop
senduuname:*:*:drop
version:*:*:drop
newgroup:*:*:log=newgroup.other
rmgroup:*:*:log=rmgroup.other

##	HIERARCHIES
##
## Taken from INN's control.ctl (samples/control.ctl in the INN source, last
## modified 2023-08-05), with the actions written for Diablo: doit becomes
## doit=newgroup, doit=rmgroup or doit=checkgroups (the log file), and mail
## becomes log.  The *PGP* lines need the keys described in README.PGP.
##
## Most of these hierarchies are historical.  The comment of each one says so
## (*DEFUNCT*, *HISTORIC*, *LOCAL*, *PRIVATE*, *RESERVED*), and a rule for a
## hierarchy you do not carry is not harmless: a newgroup creates the group,
## and a checkgroups (doit=checkgroups) creates every group it lists and
## removes the others of its hierarchies.  Only a few hierarchies are still
## maintained and widely used: the Big 8 (group-admin@isc.org), alt, and the
## hierarchies that sign their control messages (de, fr, it, uk, be, ...).  To
## keep a hierarchy out, change its newgroup and checkgroups lines to drop, or
## delete them.  The lines at the top (cancel, the log files, the types
## dropped) are Diablo's and stay first.
##
## Only the last checkgroups line whose <from> matches is used, whatever its
## <newsgroups>; the pattern then limits the groups it may change.  A sender
## that issues checkgroups for several hierarchies therefore has one line with
## the hierarchies joined by "|" (aus.*|bne.*|...), and a sender that also
## matches an earlier wildcard <from> has that hierarchy in its line too
## (eric@cirr.com: dfw.*|tx.*).  Unlike INN, <from> is matched with the case
## it has in the message.

## Special reserved groups
newgroup:*:control|general|junk|test|to:drop
rmgroup:*:control|general|junk|test|to:drop

## A.BSU (*DEFUNCT* -- Ball State University, USA)
# This hierarchy is defunct.  Please remove it.
newgroup:*:a.bsu.*:log
rmgroup:*:a.bsu.*:doit=rmgroup

## ACS & OSU (*LOCAL* -- Ohio State University, USA)
# Contact: Albert J. School <school.1@osu.edu>
# Contact: Harpal Chohan <chohan+@osu.edu>
# For local use only, contact the above address for information.
newgroup:*:acs.*|osu.*:log
rmgroup:*:acs.*|osu.*:doit=rmgroup

## ADASS (Astronomical Data Analysis Software and Systems)
# URL: http://iraf.noao.edu/iraf/web/adass_news.html
checkgroups:news@iraf.noao.edu:adass.*:doit=checkgroups
newgroup:news@iraf.noao.edu:adass.*:doit=newgroup
rmgroup:news@iraf.noao.edu:adass.*:doit=rmgroup

## AHN (Athens-Clarke County, Georgia, USA)
checkgroups:greg@*.ucns.uga.edu:ahn.*:doit=checkgroups
newgroup:greg@*.ucns.uga.edu:ahn.*:doit=newgroup
rmgroup:greg@*.ucns.uga.edu:ahn.*:doit=rmgroup

## AIOE (Aioe.org)
# Contact: usenet@aioe.org
# URL: http://news.aioe.org/hierarchy/
# Admin group: aioe.system
# Key URL: http://news.aioe.org/hierarchy/aioe.txt
# Key fingerprint: 2203 1AAC 51E7 C7FD 664F  1D80 90DF 6C71 2322 A7F8
# Syncable server: nntp.aioe.org
# *PGP*   See README.PGP.
newgroup:*:aioe.*:drop
rmgroup:*:aioe.*:drop
checkgroups:usenet@aioe.org:aioe.*:verify-usenet@aioe.org
newgroup:usenet@aioe.org:aioe.*:verify-usenet@aioe.org
rmgroup:usenet@aioe.org:aioe.*:verify-usenet@aioe.org

## AIR (*DEFUNCT* -- Stanford University, USA)
# Contact: news@news.stanford.edu
# This hierarchy is defunct.  Please remove it.
newgroup:*:air.*:log
rmgroup:*:air.*:doit=rmgroup

## AKR (Akron, Ohio, USA)
checkgroups:red@redpoll.mrfs.oh.us:akr.*:doit=checkgroups
newgroup:red@redpoll.mrfs.oh.us:akr.*:doit=newgroup
rmgroup:red@redpoll.mrfs.oh.us:akr.*:doit=rmgroup

## ALABAMA & HSV (Huntsville, Alabama, USA)
# Contact: jpc@suespammers.org
# Admin group: alabama.config
# *PGP*   See README.PGP.
newgroup:*:alabama.*|hsv.*:drop
rmgroup:*:alabama.*|hsv.*:drop
checkgroups:jpc@suespammers.org:alabama.*|hsv.*:verify-alabama-group-admin
newgroup:jpc@suespammers.org:alabama.*|hsv.*:verify-alabama-group-admin
rmgroup:jpc@suespammers.org:alabama.*|hsv.*:verify-alabama-group-admin

## ALIVE (*DEFUNCT* -- ?)
# Contact: thijs@kink.xs4all.nl
# This hierarchy is defunct.  Please remove it.
newgroup:*:alive.*:log
rmgroup:*:alive.*:doit=rmgroup

## ALT
#
# Accept all newgroups (except ones forged from Big 8 newgroup issuers,
# who never issue alt.* control messages) and silently ignore all
# rmgroups.
#
# What policy to use for alt.* groups varies widely from site to site.
# For a small site, it is strongly recommended that this policy be changed
# to drop all newgroups and rmgroups for alt.*.  The local news admin can
# then add new alt.* groups only on user request.  Tons of alt.* newgroups
# are sent out regularly with the intent more to create nonsense entries
# in active files than to actually create a useable newsgroup.  The admin
# may still want to check the control message archive, as described below.
#
# Quality, user-desirable new groups can often be discovered by a quick
# perusal of recent alt.* newgroup messages after discarding obvious junk
# groups.  One good initial filter is to check the archive of control
# messages for a requested group to see if a syntactically valid newgroup
# message was issued.  Many of the junk control messages are invalid and
# won't be archived, and many sites will only add alt.* groups with valid
# control messages.  To check the archive, see if:
#
#     ftp://ftp.isc.org/pub/usenet/control/alt/<group-name>.gz
#
# exists (replacing <group-name> with the name of the group) and read the
# first and last few control messages to see if the newsgroup should be
# moderated.  (Some alt.* groups that should be moderated are created
# unmoderated by hijackers to try to damage the newsgroup.)
#
# Be aware that there is no official, generally accepted alt.* policy and
# all information about alt.* groups available is essentially someone's
# opinion, including these comments.  There are nearly as many different
# policies with regard to alt.* groups as there are Usenet sites.
#
newgroup:*:alt.*:doit=newgroup
newgroup:group-admin@isc.org:alt.*:drop
newgroup:tale@*uu.net:alt.*:drop
rmgroup:*:alt.*:drop

## AR (Argentina)
checkgroups:jorge_f@nodens.fisica.unlp.edu.ar:ar.*:doit=checkgroups
newgroup:jorge_f@nodens.fisica.unlp.edu.ar:ar.*:doit=newgroup
rmgroup:jorge_f@nodens.fisica.unlp.edu.ar:ar.*:doit=rmgroup

## ARC (*LOCAL* -- NASA Ames Research Center, USA)
# Contact: news@arc.nasa.gov
# For local use only, contact the above address for information.
newgroup:*:arc.*:log
rmgroup:*:arc.*:doit=rmgroup

## ARKANE (Arkane Systems, UK)
# Contact: newsbastard@arkane.demon.co.uk
checkgroups:newsbastard@arkane.demon.co.uk:arkane.*:doit=checkgroups
newgroup:newsbastard@arkane.demon.co.uk:arkane.*:doit=newgroup
rmgroup:newsbastard@arkane.demon.co.uk:arkane.*:doit=rmgroup

## AT (Austria)
# URL: http://www.usenet.at/
# Admin group: at.usenet.gruppen
# Key URL: http://www.usenet.at/pgpkey.asc
# *PGP*   See README.PGP.
newgroup:*:at.*:drop
rmgroup:*:at.*:drop
checkgroups:control@usenet.backbone.at:at.*:verify-control@usenet.backbone.at
newgroup:control@usenet.backbone.at:at.*:verify-control@usenet.backbone.at
rmgroup:control@usenet.backbone.at:at.*:verify-control@usenet.backbone.at

## AUS (Australia)
# Contact: ausadmin@aus.news-admin.org
# URL: http://aus.news-admin.org/
# Admin group: aus.net.news
# Key URL: http://aus.news-admin.org/ausadmin.asc
# *PGP*   See README.PGP.
newgroup:*:aus.*:drop
rmgroup:*:aus.*:drop
# (the checkgroups of ausadmin@aus.news-admin.org for aus.* are in the line for wa.* below)
newgroup:ausadmin@aus.news-admin.org:aus.*:verify-ausadmin@aus.news-admin.org
rmgroup:ausadmin@aus.news-admin.org:aus.*:verify-ausadmin@aus.news-admin.org

## AUSTIN (Austin, Texas, USA)
# URL: http://frenzy.austin.tx.us/austin/
# Admin group: austin.usenet.config
checkgroups:chip@unicom.com:austin.*:doit=checkgroups
# (the checkgroups of fletcher@cs.utexas.edu for austin.* are in the line for tx.* below)
checkgroups:pug@pug.net:austin.*:doit=checkgroups
newgroup:chip@unicom.com:austin.*:doit=newgroup
newgroup:fletcher@cs.utexas.edu:austin.*:doit=newgroup
newgroup:pug@pug.net:austin.*:doit=newgroup
rmgroup:chip@unicom.com:austin.*:doit=rmgroup
rmgroup:fletcher@cs.utexas.edu:austin.*:doit=rmgroup
rmgroup:pug@pug.net:austin.*:doit=rmgroup

## AZ (Arizona, USA)
checkgroups:system@asuvax.eas.asu.edu:az.*:doit=checkgroups
newgroup:system@asuvax.eas.asu.edu:az.*:doit=newgroup
rmgroup:system@asuvax.eas.asu.edu:az.*:doit=rmgroup

## BA (San Francisco Bay Area, USA)
#
# This hierarchy is still in use but has no active maintainers as of
# October of 2021.  No control messages are currently being issued for
# it.
#
# This hierarchy is still in use, but it has no active maintainer.
# Control messages for this hierarchy should not be honored without
# confirming that the sender is the new hierarchy maintainer.
#
# Admin group: ba.config

## BACKBONE (*LOCAL* -- ruhr.de/ruhrgebiet.individual.net in Germany)
# Contact: admin@ruhr.de
# For local use only, contact the above address for information.
newgroup:*:backbone.*:log
rmgroup:*:backbone.*:doit=rmgroup

## BC (British Columbia, Canada)
# (the checkgroups of bc_van_usenet@fastmail.ca for bc.* are in the line for van.* below)
newgroup:bc_van_usenet@fastmail.ca:bc.*:doit=newgroup
rmgroup:bc_van_usenet@fastmail.ca:bc.*:doit=rmgroup

## BDA (German groups?)
checkgroups:news@*netuse.de:bda.*:doit=checkgroups
newgroup:news@*netuse.de:bda.*:doit=newgroup
rmgroup:news@*netuse.de:bda.*:doit=rmgroup

## BE (Belgique/Belgie/Belgien/Belgium)
# Contact: be-hierarchy-admin@usenet.be
# URL: http://usenet.be/
# Admin group: be.announce
# Key URL: http://usenet.be/be.announce.newgroups.asc
# Key fingerprint: 30 2A 45 94 70 DE 1F D5  81 8C 58 64 D2 F7 08 71
# *PGP*   See README.PGP.
newgroup:*:be.*:drop
rmgroup:*:be.*:drop
checkgroups:group-admin@usenet.be:be.*:verify-be.announce.newgroups
newgroup:group-admin@usenet.be:be.*:verify-be.announce.newgroups
rmgroup:group-admin@usenet.be:be.*:verify-be.announce.newgroups

## BELWUE (Landeshochschulnetz Baden-Wuerttemberg, Germany)
# Contact: news@belwue.de
# URL: http://www.belwue.de/produkte/dienste/dienste-usenet1.html
# Admin group: belwue.infos
# Key URL: http://www.belwue.de/fileadmin/belwue/Dokumente/Newsserver/belwue-hir-control.asc
# Key fingerprint: 2E FF 2B 8A 89 1F E5 AA  6F 89 67 24 50 B5 42 D9
# *PGP*   See README.PGP.
newgroup:*:belwue.*:drop
rmgroup:*:belwue.*:drop
checkgroups:news@news.belwue.de:belwue.*:verify-belwue-hir-control
newgroup:news@news.belwue.de:belwue.*:verify-belwue-hir-control
rmgroup:news@news.belwue.de:belwue.*:verify-belwue-hir-control

## BERMUDA (Bermuda)
checkgroups:news@*ibl.bm:bermuda.*:doit=checkgroups
newgroup:news@*ibl.bm:bermuda.*:doit=newgroup
rmgroup:news@*ibl.bm:bermuda.*:doit=rmgroup

## BEST (*LOCAL* -- Best Internet Communications, Inc.)
# Contact: news@best.net
# For local use only, contact the above address for information.
newgroup:*:best.*:log
rmgroup:*:best.*:doit=rmgroup

## BIONET (Biology Network)
# URL: http://www.bio.net/
# Admin group: bionet.general
# Key fingerprint: EB C0 F1 BA 26 0B C6 D6  FB 8D ED C4 AE 5D 10 54
# *PGP*   See README.PGP.
newgroup:*:bionet.*:drop
rmgroup:*:bionet.*:drop
checkgroups:Biosci-control-key@net.bio.net:bionet.*:verify-Biosci-control-key@net.bio.net
newgroup:Biosci-control-key@net.bio.net:bionet.*:verify-Biosci-control-key@net.bio.net
rmgroup:Biosci-control-key@net.bio.net:bionet.*:verify-Biosci-control-key@net.bio.net

## BIRK (*LOCAL* -- University of Oslo, Norway)
# Contact: birk-admin@ping.uio.no
# For local use only, contact the above address for information.
newgroup:*:birk.*:log
rmgroup:*:birk.*:doit=rmgroup

## BIT (Gatewayed Mailing lists)
# URL: http://www.newsadmin.com/bit/bit.htm
# Admin group: bit.admin
# *PGP*   See README.PGP.
newgroup:*:bit.*:drop
rmgroup:*:bit.*:drop
checkgroups:bit@newsadmin.com:bit.*:verify-bit@newsadmin.com
newgroup:bit@newsadmin.com:bit.*:verify-bit@newsadmin.com
rmgroup:bit@newsadmin.com:bit.*:verify-bit@newsadmin.com

## BIZ (Business Groups)
checkgroups:edhew@xenitec.on.ca:biz.*:doit=checkgroups
newgroup:edhew@xenitec.on.ca:biz.*:doit=newgroup
rmgroup:edhew@xenitec.on.ca:biz.*:doit=rmgroup

## BLGTN (Bloomington, In, USA)
checkgroups:control@news.bloomington.in.us:blgtn.*:doit=checkgroups
newgroup:control@news.bloomington.in.us:blgtn.*:doit=newgroup
rmgroup:control@news.bloomington.in.us:blgtn.*:doit=rmgroup

## BLN (Berlin, Germany)
# Contact: news@fu-berlin.de
# URL: https://ftp.fu-berlin.de/doc/news/bln/bln-info.txt
# Admin group: bln.misc
# Key URL: https://ftp.fu-berlin.de/doc/news/bln/bln-key.txt
# Key fingerprint: 2B24 B5C7 8A48 6140 7BCD  5A88 281D 9076 3F36 8EA0
# *PGP*   See README.PGP.
newgroup:*:bln.*:drop
rmgroup:*:bln.*:drop
checkgroups:news@fu-berlin.de:bln.*:verify-news@fu-berlin.de
newgroup:news@fu-berlin.de:bln.*:verify-news@fu-berlin.de
rmgroup:news@fu-berlin.de:bln.*:verify-news@fu-berlin.de

## BNE (Brisbane, Australia)
# Contact: ausadmin@aus.news-admin.org
# URL: http://bne.news-admin.org/
# Key URL: http://aus.news-admin.org/ausadmin.asc
# *PGP*   See README.PGP.
newgroup:*:bne.*:drop
rmgroup:*:bne.*:drop
# (the checkgroups of ausadmin@aus.news-admin.org for bne.* are in the line for wa.* below)
newgroup:ausadmin@aus.news-admin.org:bne.*:verify-ausadmin@aus.news-admin.org
rmgroup:ausadmin@aus.news-admin.org:bne.*:verify-ausadmin@aus.news-admin.org

## BOFH (*PRIVATE* -- Bastard Operator From Hell)
# Contact: bofh-control@lists.killfile.org
# Key fingerprint: 40B5 8A56 1E00 6152 083E  38B3 CEF5 6980 7DC1 A266
# For private use only, contact the above address for information.
# *PGP*   See README.PGP.
newgroup:*:bofh.*:drop
rmgroup:*:bofh.*:drop
# The following three lines are only for authorized bofh.* sites.
#checkgroups:bofh-control@killfile.org:bofh.*:verify-bofh-control@lists.killfile.org
#newgroup:bofh-control@killfile.org:bofh.*:verify-bofh-control@lists.killfile.org
#rmgroup:bofh-control@killfile.org:bofh.*:verify-bofh-control@lists.killfile.org

## CA (California, USA)
# Contact: ikluft@thunder.sbay.org
# URL: http://www.sbay.org/ca/
# (the checkgroups of ikluft@thunder.sbay.org for ca.* are in the line for sbay.* below)
newgroup:ikluft@thunder.sbay.org:ca.*:doit=newgroup
rmgroup:ikluft@thunder.sbay.org:ca.*:doit=rmgroup

## CAIS (*LOCAL* -- Capital Area Internet Services)
# Contact: news@cais.com
# For local use only, contact the above address for information.
newgroup:*:cais.*:log
rmgroup:*:cais.*:doit=rmgroup

## CALSTATE (California State University)
checkgroups:*@*calstate.edu:calstate.*:doit=checkgroups
newgroup:*@*calstate.edu:calstate.*:doit=newgroup
rmgroup:*@*calstate.edu:calstate.*:doit=rmgroup

## CANB (Canberra, Australia)
# Contact: ausadmin@aus.news-admin.org
# URL: http://canb.news-admin.org/
# Key URL: http://aus.news-admin.org/ausadmin.asc
# *PGP*   See README.PGP.
newgroup:*:canb.*:drop
rmgroup:*:canb.*:drop
# (the checkgroups of ausadmin@aus.news-admin.org for canb.* are in the line for wa.* below)
newgroup:ausadmin@aus.news-admin.org:canb.*:verify-ausadmin@aus.news-admin.org
rmgroup:ausadmin@aus.news-admin.org:canb.*:verify-ausadmin@aus.news-admin.org

## CAPDIST (Albany, The Capital District, New York, USA)
checkgroups:danorton@albany.net:capdist.*:doit=checkgroups
newgroup:danorton@albany.net:capdist.*:doit=newgroup
rmgroup:danorton@albany.net:capdist.*:doit=rmgroup

## CARLETON (Carleton University, Canada)
newgroup:news@cunews.carleton.ca:carleton.*:doit=newgroup
newgroup:news@cunews.carleton.ca:carleton*class.*:log
rmgroup:news@cunews.carleton.ca:carleton.*:doit=rmgroup

## CD-ONLINE (*LOCAL* -- ?)
# Contact: newsmaster@worldonline.nl
# For local use only, contact the above address for information.
newgroup:*:cd-online.*:log
rmgroup:*:cd-online.*:doit=rmgroup

## CENTRAL (*LOCAL* -- The Internet Company of New Zealand, Wellington, NZ)
# Contact: usenet@iconz.co.nz
# For local use only, contact the above address for information.
newgroup:*:central.*:log
rmgroup:*:central.*:doit=rmgroup

## CERN (*PRIVATE* -- CERN European Laboratory for Particle Physics)
# Contact: Dietrich Wiegandt <News.Support@cern.ch>
# For private use only, contact the above address for information.
newgroup:News.Support@cern.ch:cern.*:log
rmgroup:News.Support@cern.ch:cern.*:doit=rmgroup

## CH (Switzerland)
# Contact: ch-news-admin@use-net.ch
# URL: http://www.use-net.ch/Usenet/
# Key URL: http://www.use-net.ch/Usenet/adminkey.html
# Key fingerprint: 71 80 D6 8C A7 DE 2C 70  62 4A 48 6E D9 96 02 DF
# *PGP*   See README.PGP.
newgroup:*:ch.*:drop
rmgroup:*:ch.*:drop
checkgroups:felix.rauch@nice.ch:ch.*:verify-ch-news-admin@use-net.ch
newgroup:felix.rauch@nice.ch:ch.*:verify-ch-news-admin@use-net.ch
rmgroup:felix.rauch@nice.ch:ch.*:verify-ch-news-admin@use-net.ch

## CHAVEN (*LOCAL* -- Celestian Haven ISP, Midwest, USA)
# Contact: news@chaven.com
# For local use only, contact the above address for information.
newgroup:*:chaven.*:log
rmgroup:*:chaven.*:doit=rmgroup

## CHI (Chicago, USA)
# URL: http://lull.org/pub/chi-newsgroup-faq
checkgroups:lisbon@*chi.il.us:chi.*:doit=checkgroups
newgroup:lisbon@*chi.il.us:chi.*:doit=newgroup
rmgroup:lisbon@*chi.il.us:chi.*:doit=rmgroup

## CHILE (Chile and Chilean affairs)
# Contact: mod-cga@usenet.cl
# URL: http://www.usenet.cl/
# Admin group: chile.grupos.anuncios
checkgroups:mod-cga@*usenet.cl:chile.*:doit=checkgroups
checkgroups:mod-cga@*farah.cl:chile.*:doit=checkgroups
newgroup:mod-cga@*usenet.cl:chile.*:doit=newgroup
newgroup:mod-cga@*farah.cl:chile.*:doit=newgroup
rmgroup:mod-cga@*usenet.cl:chile.*:doit=rmgroup
rmgroup:mod-cga@*farah.cl:chile.*:doit=rmgroup

## CHINESE (China and Chinese language groups)
checkgroups:pinghua@stat.berkeley.edu:chinese.*:doit=checkgroups
newgroup:pinghua@stat.berkeley.edu:chinese.*:doit=newgroup
rmgroup:pinghua@stat.berkeley.edu:chinese.*:doit=rmgroup

## CHRISTNET (Christian Discussion)
checkgroups:news@fdma.com:christnet.*:doit=checkgroups
newgroup:news@fdma.com:christnet.*:doit=newgroup
rmgroup:news@fdma.com:christnet.*:doit=rmgroup

## CL (*PRIVATE* -- CL-Netz, German)
# Contact: koordination@cl-netz.de
# URL: http://www.cl-netz.de/
# Key URL: http://www.cl-netz.de/control.txt
# For private use only, contact above address for questions.
# *PGP*   See README.PGP.
newgroup:*:cl.*:drop
rmgroup:*:cl.*:doit=rmgroup
# The following three lines are only for authorized cl.* sites.
#checkgroups:koordination@cl-netz.de:cl.*:verify-cl.netz.infos
#newgroup:koordination@cl-netz.de:cl.*:verify-cl.netz.infos
#rmgroup:koordination@cl-netz.de:cl.*:verify-cl.netz.infos

## CLARI (*PRIVATE* -- Features and News, available on a commercial basis)
# Contact: support@clari.net
# Admin group: clari.net.admin
# Key URL: http://www.clari.net/tech/clarikey.txt
# For private use only, contact the above address for information.
# *PGP*   See README.PGP.
newgroup:*:clari.*:drop
rmgroup:*:clari.*:drop
newgroup:cl*@clarinet.com:clari.*:log
rmgroup:cl*@clarinet.com:clari.*:verify-ClariNet.Group

## CMI (*LOCAL* -- Champaign County, IL, USA)
# Contact: news@ks.uiuc.edu
# For local use only, contact the above address for information.
newgroup:*:cmi.*:log
rmgroup:*:cmi.*:doit=rmgroup

## CMU (*LOCAL* -- Carnegie-Mellon University, Pennsylvania, USA)
# Contact: Daniel Edward Lovinger <del+@CMU.EDU>
# For local use only, contact the above address for information.
newgroup:*:cmu.*:log
rmgroup:*:cmu.*:doit=rmgroup

## CN (China)
# URL: http://news.yaako.com/
# Admin group: cn.announce
# Key fingerprint: 62 97 EE 33 F7 16 25 C1  A4 9E 47 BA C5 3E 5E 9E
# *PGP*   See README.PGP.
newgroup:*:cn.*:drop
rmgroup:*:cn.*:drop
checkgroups:control@bentium.com:cn.*:verify-cn.admin.news.announce
newgroup:control@bentium.com:cn.*:verify-cn.admin.news.announce
rmgroup:control@bentium.com:cn.*:verify-cn.admin.news.announce

## CN.BBS (China)
# URL: http://bbs.cn.news-admin.org/
# Admin group: cn.bbs.admin.announce
# *PGP*   See README.PGP.
newgroup:*:cn.bbs.*:drop
rmgroup:*:cn.bbs.*:drop
checkgroups:control@cn-bbs.org:cn.bbs.*:verify-cn.bbs.admin.announce
newgroup:control@cn-bbs.org:cn.bbs.*:verify-cn.bbs.admin.announce
rmgroup:control@cn-bbs.org:cn.bbs.*:verify-cn.bbs.admin.announce

## CO (Colorado, USA)
# Contact: coadmin@boyznoyz.com (Bill of Denver)
checkgroups:coadmin@boyznoyz.com:co.*:doit=checkgroups
newgroup:coadmin@boyznoyz.com:co.*:doit=newgroup
rmgroup:coadmin@boyznoyz.com:co.*:doit=rmgroup

## CODEWARRIOR (CodeWarrior discussion)
checkgroups:news@supernews.net:codewarrior.*:doit=checkgroups
newgroup:news@supernews.net:codewarrior.*:doit=newgroup
rmgroup:news@supernews.net:codewarrior.*:doit=rmgroup

## COMP, HUMANITIES, MISC, NEWS, REC, SCI, SOC, TALK (The Big Eight)
# Contact: board@big-8.org
# URL: https://www.big-8.org/
# Admin group: news.announce.newgroups
# Key fingerprint: 8E18 1BEF 0BE6 3896 2C1A  5C98 FAFE 7B55 0C18 C8B7
# *PGP*   See README.PGP.
newgroup:*:comp.*|humanities.*|misc.*|news.*|rec.*|sci.*|soc.*|talk.*:drop
rmgroup:*:comp.*|humanities.*|misc.*|news.*|rec.*|sci.*|soc.*|talk.*:drop
checkgroups:group-admin@isc.org:comp.*|humanities.*|misc.*|news.*|rec.*|sci.*|soc.*|talk.*:verify-news.announce.newgroups
newgroup:group-admin@isc.org:comp.*|humanities.*|misc.*|news.*|rec.*|sci.*|soc.*|talk.*:verify-news.announce.newgroups
rmgroup:group-admin@isc.org:comp.*|humanities.*|misc.*|news.*|rec.*|sci.*|soc.*|talk.*:verify-news.announce.newgroups

## COMPUTER42 (Computer 42, Germany)
# Contact: Dirk Schmitt <news@computer42.org>
checkgroups:news@computer42.org:computer42.*:doit=checkgroups
newgroup:news@computer42.org:computer42.*:doit=newgroup
rmgroup:news@computer42.org:computer42.*:doit=rmgroup

## CONCORDIA (Concordia University, Montreal, Canada)
# Contact: newsmaster@concordia.ca
# URL: General University info at http://www.concordia.ca/
checkgroups:news@newsflash.concordia.ca:concordia.*:doit=checkgroups
newgroup:news@newsflash.concordia.ca:concordia.*:doit=newgroup
rmgroup:news@newsflash.concordia.ca:concordia.*:doit=rmgroup

## CONTROL (*RESERVED* -- Special hierarchy for control messages)
#
# The control.* hierarchy is reserved by RFC 5536 and MUST NOT be used
# for regular newsgroups.  It is used by some news implementations, such
# as INN, as a local, special hierarchy that shows all control messages
# posted to any group.
#
newgroup:*:control.*:drop
rmgroup:*:control.*:drop

## COURTS (*DEFUNCT* -- Court discussion)
# Contact: trier@ins.cwru.edu
# This hierarchy is defunct.  Please remove it.
newgroup:*:courts.*:log
rmgroup:*:courts.*:doit=rmgroup

## CPCUIIA (Chartered Prop. Casulty Underwriter/Insurance Institute of America)
# Contact: miller@cpcuiia.org
# URL: http://www.aicpcu.org/
checkgroups:miller@cpcuiia.org:cpcuiia.*:doit=checkgroups
newgroup:miller@cpcuiia.org:cpcuiia.*:doit=newgroup
rmgroup:miller@cpcuiia.org:cpcuiia.*:doit=rmgroup

## CU (*LOCAL* -- University of Colorado)
# Contact: Doreen Petersen <news@colorado.edu>
# For local use only, contact the above address for information.
newgroup:*:cu.*:log
rmgroup:*:cu.*:doit=rmgroup

## CUHK (*LOCAL* -- Chinese University of Hong Kong)
# Contact: shlam@ie.cuhk.edu.hk (Alan S H Lam)
# For local use only, contact the above address for information.
newgroup:*:cuhk.*:log
rmgroup:*:cuhk.*:doit=rmgroup

## CZ (Czech Republic)
# URL: ftp://ftp.vslib.cz/pub/news/config/cz/newsgroups (text)
checkgroups:petr.kolar@vslib.cz:cz.*:doit=checkgroups
newgroup:petr.kolar@vslib.cz:cz.*:doit=newgroup
rmgroup:petr.kolar@vslib.cz:cz.*:doit=rmgroup

## DC (Washington, D.C., USA)
checkgroups:news@mattress.atww.org:dc.*:doit=checkgroups
newgroup:news@mattress.atww.org:dc.*:doit=newgroup
rmgroup:news@mattress.atww.org:dc.*:doit=rmgroup

## DE (German language)
# Contact: moderator@dana.de
# URL: http://www.dana.de/
# Admin group: de.admin.news.announce
# Key URL: http://www.dana.de/pgp/dana.txt
# Key fingerprint: 5B B0 52 88 BF 55 19 4F  66 7D C2 AE 16 26 28 25
# *PGP*   See README.PGP.
newgroup:*:de.*:drop
rmgroup:*:de.*:drop
checkgroups:moderator@dana.de:de.*:verify-de.admin.news.announce
newgroup:moderator@dana.de:de.*:verify-de.admin.news.announce
rmgroup:moderator@dana.de:de.*:verify-de.admin.news.announce

## DE.ALT (German language alternative hierarchy)
# *PGP*   See README.PGP.
newgroup:*:de.alt.*:doit=newgroup
rmgroup:moderator@dana.de:de.alt.*:verify-de.admin.news.announce

## DEMON (Demon Internet, UK)
# Contact: newsmaster@demon.net
# Admin group: demon.news
# Key URL: ftp://ftp.demon.co.uk/pub/news/doc/demon.news.txt
# *PGP*   See README.PGP.
newgroup:*:demon.*:drop
rmgroup:*:demon.*:drop
checkgroups:newsmaster@demon.net:demon.*:verify-demon.news
newgroup:newsmaster@demon.net:demon.*:verify-demon.news
rmgroup:newsmaster@demon.net:demon.*:verify-demon.news

## DFW (Dallas/Fort Worth, Texas, USA)
# URL: http://www.cirr.com/dfw/
# Admin group: dfw.usenet.config
checkgroups:eric@*cirr.com:dfw.*:doit=checkgroups
# (eric@cirr.com also matches this line; its checkgroups for dfw.* are in its line for tx.* below)
newgroup:eric@*cirr.com:dfw.*:doit=newgroup
rmgroup:eric@*cirr.com:dfw.*:doit=rmgroup

## DK (Denmark)
# URL: http://www.usenet.dk/dk-admin/
# Key URL: http://www.usenet.dk/dk-admin/pubkey.html
# Key fingerprint: 7C B2 C7 50 F3 7D 5D 73  8C EE 2E 3F 55 80 72 FF
# *PGP*   See README.PGP.
newgroup:*:dk.*:drop
rmgroup:*:dk.*:drop
checkgroups:news@news.dknet.dk:dk.*:verify-news@news.dknet.dk
newgroup:news@news.dknet.dk:dk.*:verify-news@news.dknet.dk
rmgroup:news@news.dknet.dk:dk.*:verify-news@news.dknet.dk

## DUKE (*LOCAL* -- Duke University, USA)
# Contact: news@newsgate.duke.edu
# For local use only, contact the above address for information.
newgroup:*:duke.*:log
rmgroup:*:duke.*:doit=rmgroup

## EASYNET (*HISTORIC* -- Easynet PLC, UK)
#
# This hierarchy is not entirely defunct, but it receives very little
# traffic and is included primarily for the sake of completeness.
#
# Contact: Christiaan Keet <newsmaster@easynet.net>
# Admin group: easynet.support
# *PGP*   See README.PGP.
newgroup:*:easynet.*:drop
rmgroup:*:easynet.*:drop
checkgroups:newsmaster@easynet.net:easynet.*:verify-easynet.news
newgroup:newsmaster@easynet.net:easynet.*:verify-easynet.news
rmgroup:newsmaster@easynet.net:easynet.*:verify-easynet.news

## EE (Estonia)
# Contact: usenet@news.ut.ee
# URL: http://news.ut.ee/
# Key URL: http://news.ut.ee/pubkey.asc
# *PGP*   See README.PGP.
newgroup:*:ee.*:drop
rmgroup:*:ee.*:drop
checkgroups:news@news.ut.ee:ee.*:verify-ee.news
newgroup:news@news.ut.ee:ee.*:verify-ee.news
rmgroup:news@news.ut.ee:ee.*:verify-ee.news

## EFN & EUG (*HISTORIC* -- Eugene Free Computer Network, Eugene/Springfield, Oregon, USA)
#
# This hierarchy is not entirely defunct, but it receives very little
# traffic and is included primarily for the sake of completeness.
#
# Admin group: eug.config
# *PGP*   See README.PGP.
newgroup:*:efn.*|eug.*:drop
rmgroup:*:efn.*|eug.*:drop
checkgroups:newsadmin@efn.org:efn.*|eug.*:verify-eug.config
newgroup:newsadmin@efn.org:efn.*|eug.*:verify-eug.config
rmgroup:newsadmin@efn.org:efn.*|eug.*:verify-eug.config

## EHIME-U (? University, Japan ?)
# (the checkgroups of news@cc.nias.ac.jp for ehime-u.* are in the line for nias.* below)
checkgroups:news@doc.dpc.ehime-u.ac.jp:ehime-u.*:doit=checkgroups
newgroup:news@cc.nias.ac.jp:ehime-u.*:doit=newgroup
newgroup:news@doc.dpc.ehime-u.ac.jp:ehime-u.*:doit=newgroup
rmgroup:news@cc.nias.ac.jp:ehime-u.*:doit=rmgroup
rmgroup:news@doc.dpc.ehime-u.ac.jp:ehime-u.*:doit=rmgroup

## ENGLAND (England)
# Contact: admin@england.news-admin.org
# Admin group: england.news.policy
# Key fingerprint: DA 3E C2 01 46 E5 61 CB  A2 43 09 CA 13 6D 31 1F
# *PGP*   See README.PGP.
newgroup:*:england.*:drop
rmgroup:*:england.*:drop
checkgroups:admin@england.news-admin.org:england.*:verify-england-usenet
newgroup:admin@england.news-admin.org:england.*:verify-england-usenet
rmgroup:admin@england.news-admin.org:england.*:verify-england-usenet

## ES (Spain)
# Contact: moderador@corus-es.org
# URL: http://www.corus-es.org/docs/es_newsadmins_faq.txt
# Admin group: es.news.anuncios
# Key URL: http://www.corus-es.org/docs/esnews.asc
# *PGP*   See README.PGP.
newgroup:*:es.*:drop
rmgroup:*:es.*:drop
checkgroups:moderador@corus-es.org:es.*:verify-es.news
newgroup:moderador@corus-es.org:es.*:verify-es.news
rmgroup:moderador@corus-es.org:es.*:verify-es.news

## ESP (Spanish-language newsgroups)
# Contact: <mod-ena@ennui.org>
# URL: http://ennui.org/esp/
# Key URL: http://ennui.org/esp/mod-ena.asc
# *PGP*   See README.PGP.
newgroup:*:esp.*:drop
rmgroup:*:esp.*:drop
checkgroups:mod-ena@ennui.org:esp.*:verify-esp.news.administracion
newgroup:mod-ena@ennui.org:esp.*:verify-esp.news.administracion
rmgroup:mod-ena@ennui.org:esp.*:verify-esp.news.administracion

## ETERNAL-SEPTEMBER (Eternal-September Project)
# Contact: news@eternal-september.org
# URL: http://www.eternal-september.org/
# Key URL: https://www.eternal-september.org/control-eternal-september.asc
# Key fingerprint: 17A7 E98E 3038 BE35 428F  30E4 F53C 21EE F3D2 EA49
# The messages are now sent by control@eternal-september.org, signed by the key
# above (user id "Control <control@eternal-september.org>", so verify-Control).
# The lines for news@eternal-september.org are for the old key (fingerprint
# B7 9A 2B C6 7D 5A FF 79  18 E2 AC 91 4B C1 25 F1, as in INN's control.ctl).
# *PGP*   See README.PGP.
newgroup:*:eternal-september.*:drop
rmgroup:*:eternal-september.*:drop
checkgroups:control@eternal-september.org:eternal-september.*:verify-Control
newgroup:control@eternal-september.org:eternal-september.*:verify-Control
rmgroup:control@eternal-september.org:eternal-september.*:verify-Control
checkgroups:news@eternal-september.org:eternal-september.*:verify-news@eternal-september.org
newgroup:news@eternal-september.org:eternal-september.*:verify-news@eternal-september.org
rmgroup:news@eternal-september.org:eternal-september.*:verify-news@eternal-september.org

## EUNET (Europe)
checkgroups:news@noc.eu.net:eunet.*:doit=checkgroups
newgroup:news@noc.eu.net:eunet.*:doit=newgroup
rmgroup:news@noc.eu.net:eunet.*:doit=rmgroup

## EUROPA (Europe)
# URL: http://www.europa.usenet.eu.org/
# Admin group: europa.usenet.admin
# Key URL: http://www.europa.usenet.eu.org/pgp/index.html
# Key fingerprint: 3A 05 A8 49 FB 16 29 25  75 E3 DE BB 69 E0 1D B4
# *PGP*   See README.PGP.
newgroup:*:europa.*:drop
rmgroup:*:europa.*:drop
checkgroups:group-admin@usenet.eu.org:europa.*:verify-group-admin@usenet.eu.org
newgroup:group-admin@usenet.eu.org:europa.*:verify-group-admin@usenet.eu.org
rmgroup:group-admin@usenet.eu.org:europa.*:verify-group-admin@usenet.eu.org

## EXAMPLE (*RESERVED* -- For use in examples)
#
# The example.* hierarchy is reserved by RFC 5536 and MUST NOT be used
# for regular newsgroups.  It is intended for use in examples, standards
# documents, and similar places to avoid clashes with real newsgroup
# names.
#
newgroup:*:example.*:drop
rmgroup:*:example.*:drop

## FA (Gated mailing lists)
#
# This hierarchy was removed in the Great Renaming of 1988.
#
# A site in Norway is currently (as of 2002) gatewaying various mailing
# lists into fa.* newsgroups, but that site does not appear to be issuing
# any control messages for those groups.
#
# This hierarchy is still in use, but it has no active maintainer.
# Control messages for this hierarchy should not be honored without
# confirming that the sender is the new hierarchy maintainer.

## FFM (Frankfurt/M., Germany)
# URL: http://ffm.arcornews.de/
# Admin group: ffm.admin
# Key URL: ftp://ftp.arcor-online.net/pub/news/PGPKEY.FFM
# Key fingerprint: 53 A0 82 62 6F C7 81 C9  CF 53 AB 00 A3 F8 C2 11
# *PGP*   See README.PGP.
newgroup:*:ffm.*:drop
rmgroup:*:ffm.*:drop
checkgroups:ffm.admin@arcor.de:ffm.*:verify-ffm.admin
newgroup:ffm.admin@arcor.de:ffm.*:verify-ffm.admin
rmgroup:ffm.admin@arcor.de:ffm.*:verify-ffm.admin

## FIDO (FidoNet)
#
# This hierarchy is still in use, but it has no active maintainer.
# Control messages for this hierarchy should not be honored without
# confirming that the sender is the new hierarchy maintainer.

## FIDO.BELG (Belgian FidoNet)
# Admin group: fido.belg.news
# *PGP*   See README.PGP.
newgroup:*:fido.belg.*:drop
rmgroup:*:fido.belg.*:drop
checkgroups:fidobelg@mail.z2.fidonet.org:fido.belg.*:verify-fido.belg.news
newgroup:fidobelg@mail.z2.fidonet.org:fido.belg.*:verify-fido.belg.news
rmgroup:fidobelg@mail.z2.fidonet.org:fido.belg.*:verify-fido.belg.news

## FIDO.GER (German FIDO Net Echos)
# URL: ftp://ftp.fu-berlin.de/doc/news/fido.ger/fido.ger-info.english
# Key URL: ftp://ftp.fu-berlin.de/doc/news/fido.ger/PGP-Key
# *PGP*   See README.PGP.
newgroup:*:fido.ger.*:drop
rmgroup:*:fido.ger.*:drop
checkgroups:fido.ger@news.fu-berlin.de:fido.ger.*:verify-fido.ger@news.fu-berlin.de
newgroup:fido.ger@news.fu-berlin.de:fido.ger.*:verify-fido.ger@news.fu-berlin.de
rmgroup:fido.ger@news.fu-berlin.de:fido.ger.*:verify-fido.ger@news.fu-berlin.de

## FIDO7 (Russian FidoNet)
# URL: http://www.fido7.org/
# Admin group: fido7.postmasters
# Key URL: http://www.fido7.org/pgpcontrol.html
# *PGP*   See README.PGP.
newgroup:*:fido7.*:drop
rmgroup:*:fido7.*:drop
checkgroups:newgroups-request@fido7.org:fido7.*:verify-fido7.announce.newgroups
newgroup:newgroups-request@fido7.org:fido7.*:verify-fido7.announce.newgroups
rmgroup:newgroups-request@fido7.org:fido7.*:verify-fido7.announce.newgroups

## FINET (Finland and Finnish language alternative newsgroups)
checkgroups:*@*.fi:finet.*:doit=checkgroups
# (news@news.cc.tut.fi also matches this line; its checkgroups for finet.* are in its line for uta.* below)
newgroup:*@*.fi:finet.*:doit=newgroup
rmgroup:*@*.fi:finet.*:doit=rmgroup

## FJ (Japan and Japanese language)
# Contact: committee@fj-news.org
# URL: http://www.fj-news.org/index.html.en
# Admin group: fj.news.announce
# Key URL: http://www.is.tsukuba.ac.jp/~yas/fj/fj.asc
# *PGP*   See README.PGP.
newgroup:*:fj.*:drop
rmgroup:*:fj.*:drop
checkgroups:committee@fj-news.org:fj.*:verify-fj.news.announce
newgroup:committee@fj-news.org:fj.*:verify-fj.news.announce
rmgroup:committee@fj-news.org:fj.*:verify-fj.news.announce

## FL (Florida, USA)
checkgroups:hgoldste@news1.mpcs.com:fl.*:doit=checkgroups
checkgroups:scheidell@fdma.fdma.com:fl.*:doit=checkgroups
newgroup:hgoldste@news1.mpcs.com:fl.*:doit=newgroup
newgroup:scheidell@fdma.fdma.com:fl.*:doit=newgroup
rmgroup:hgoldste@news1.mpcs.com:fl.*:doit=rmgroup
rmgroup:scheidell@fdma.fdma.com:fl.*:doit=rmgroup

## FLORA (FLORA Community WEB, Canada)
# Contact: russell@flora.org
# Admin group: flora.general
# *PGP*   See README.PGP.
newgroup:*:flora.*:drop
rmgroup:*:flora.*:drop
checkgroups:news@flora.ottawa.on.ca:flora.*:verify-flora-news
newgroup:news@flora.ottawa.on.ca:flora.*:verify-flora-news
rmgroup:news@flora.ottawa.on.ca:flora.*:verify-flora-news

## FR (French language)
# URL: https://www.usenet-fr.net/
# Admin group: fr.usenet.forums.annonces
# Key URL: https://www.usenet-fr.net/pgp-fr-2020.txt
# Key fingerprint: D5F3 69B2 9757 3622 0153  54E7 FA42 3E89 970E B10B
# *PGP*   See README.PGP.
newgroup:*:fr.*:drop
rmgroup:*:fr.*:drop
checkgroups:control@usenet-fr.news.eu.org:fr.*:verify-control@usenet-fr.news.eu.org
newgroup:control@usenet-fr.news.eu.org:fr.*:verify-control@usenet-fr.news.eu.org
rmgroup:control@usenet-fr.news.eu.org:fr.*:verify-control@usenet-fr.news.eu.org

## FRANCE (France)
# Contact: control@usenet-france.news.eu.org
# Admin group: france.admin.evolutions
# *PGP*   See README.PGP.
newgroup:*:france.*:drop
rmgroup:*:france.*:drop
checkgroups:control@usenet-france.news.eu.org:france.*:verify-control@usenet-france.news.eu.org
newgroup:control@usenet-france.news.eu.org:france.*:verify-control@usenet-france.news.eu.org
rmgroup:control@usenet-france.news.eu.org:france.*:verify-control@usenet-france.news.eu.org

## FREE (Open Hierarchy where anyone can create a group)
newgroup:*:free.*:doit=newgroup
newgroup:group-admin@isc.org:free.*:drop
newgroup:tale@*uu.net:free.*:drop
rmgroup:*:free.*:drop

## FUDAI (Japanese ?)
checkgroups:news@picard.cs.osakafu-u.ac.jp:fudai.*:doit=checkgroups
newgroup:news@picard.cs.osakafu-u.ac.jp:fudai.*:doit=newgroup
rmgroup:news@picard.cs.osakafu-u.ac.jp:fudai.*:doit=rmgroup

## FUR (*PRIVATE* -- furrynet)
# Contact: fur-config@news.furry.net
# For private use only, contact the above address for information.
newgroup:*:fur.*:log
rmgroup:*:fur.*:doit=rmgroup

## GENERAL (*RESERVED* -- Sometimes used as a local catch-all)
#
# There is some history of using a newsgroup named general as a local
# catch-all discussion group.  That newsgroup name and hierarchy should
# be avoided on production servers since it may occur at many
# disconnected sites.
#
newgroup:*:general.*:drop
rmgroup:*:general.*:drop

## GER & HANNOVER & HANNET & HILDESHEIM & HISS (Hannover, Germany)
checkgroups:fifi@hiss.han.de:ger.*|hannover.*|hannet.*|hildesheim.*|hiss.*:doit=checkgroups
newgroup:fifi@hiss.han.de:ger.*|hannover.*|hannet.*|hildesheim.*|hiss.*:doit=newgroup
rmgroup:fifi@hiss.han.de:ger.*|hannover.*|hannet.*|hildesheim.*|hiss.*:doit=rmgroup

## GIT (Georgia Institute of Technology, USA)
newgroup:news@news.gatech.edu:git.*:doit=newgroup
newgroup:news@news.gatech.edu:git*class.*:log
rmgroup:news@news.gatech.edu:git.*:doit=rmgroup

## GNU (Free Software Foundation)
# URL: http://www.gnu.org/usenet/usenet.html
# Admin group: gnu.gnusenet.config
# Key URL: http://www.gnu.org/usenet/usenet-pgp-key.txt
# *PGP*   See README.PGP.
newgroup:*:gnu.*:drop
rmgroup:*:gnu.*:drop
checkgroups:usenet@gnu.org:gnu.*:verify-usenet@gnu.org
newgroup:usenet@gnu.org:gnu.*:verify-usenet@gnu.org
rmgroup:usenet@gnu.org:gnu.*:verify-usenet@gnu.org

## GNUU (*PRIVATE* -- GNUU e.V., Oberursel, Germany)
# URL: http://www.gnuu.de/
# Key URL: http://www.gnuu.de/config/PGPKEY.GNUU
# For private use only.
# *PGP*   See README.PGP.
newgroup:*:gnuu.*:drop
rmgroup:*:gnuu.*:drop
newgroup:news@gnuu.de:gnuu.*:log
rmgroup:news@gnuu.de:gnuu.*:verify-news@gnuu.de

## GRISBI (Grisbi Personal Finance Manager software)
# Contact: newsmaster@grisbi.org
# URL: http://news.grisbi.org/
# Admin group: grisbi.admin
# Key URL: http://news.grisbi.org/public-key.asc
# Key fingerprint: EB35 0C03 0080 BD2A 7E0C  A4C9 F2C6 2A3D C86C C6E1
# *PGP*   See README.PGP.
newgroup:*:grisbi.*:drop
rmgroup:*:grisbi.*:drop
checkgroups:grisbi-control@grisbi.org:grisbi.*:verify-grisbi-control@grisbi.org
newgroup:grisbi-control@grisbi.org:grisbi.*:verify-grisbi-control@grisbi.org
rmgroup:grisbi-control@grisbi.org:grisbi.*:verify-grisbi-control@grisbi.org

## GWU (George Washington University, Washington, DC)
# Contact: Sweth Chandramouli <news@nit.gwu.edu>
checkgroups:news@nit.gwu.edu:gwu.*:doit=checkgroups
newgroup:news@nit.gwu.edu:gwu.*:doit=newgroup
rmgroup:news@nit.gwu.edu:gwu.*:doit=rmgroup

## HACKTIC (*HISTORIC* -- XS4ALL, Netherlands)
#
# This hierarchy is not entirely defunct, but it receives very little
# traffic and is included primarily for the sake of completeness.
#
# *PGP*   See README.PGP.
newgroup:*:hacktic.*:drop
rmgroup:*:hacktic.*:drop
checkgroups:news@zhaum.xs4all.nl:hacktic.*:verify-news@zhaum.xs4all.nl
newgroup:news@zhaum.xs4all.nl:hacktic.*:verify-news@zhaum.xs4all.nl
rmgroup:news@zhaum.xs4all.nl:hacktic.*:verify-news@zhaum.xs4all.nl

## HAMBURG (City of Hamburg, Germany)
# Contact: hamburg@steering-group.net
# URL: http://www.steering-group.net/hamburg/
# Admin group: hamburg.koordination
# Key URL: http://www.steering-group.net/hamburg/hamburg.koordination.txt
# Key fingerprint: 3E E7 0C BB 6E 01 94 EE  45 6F C5 57 F4 B9 54 8E
# *PGP*   See README.PGP.
newgroup:*:hamburg.*:drop
rmgroup:*:hamburg.*:drop
checkgroups:hamburg@steering-group.net:hamburg.*:verify-hamburg.koordination
newgroup:hamburg@steering-group.net:hamburg.*:verify-hamburg.koordination
rmgroup:hamburg@steering-group.net:hamburg.*:verify-hamburg.koordination

## HAMILTON (Canadian)
checkgroups:news@*dcss.mcmaster.ca:hamilton.*:doit=checkgroups
newgroup:news@*dcss.mcmaster.ca:hamilton.*:doit=newgroup
rmgroup:news@*dcss.mcmaster.ca:hamilton.*:doit=rmgroup

## HAMSTER (Hamster, a Win32 news and mail proxy server)
# Contact: hamster-contact@snafu.de
# Admin group: hamster.de.config
# Key fingerprint: 12 75 A9 42 8A D6 1F 77  6A CF B4 0C 79 15 5F 93
# *PGP*   See README.PGP.
newgroup:*:hamster.*:drop
rmgroup:*:hamster.*:drop
checkgroups:hamster-control@snafu.de:hamster.*:verify-hamster-control@snafu.de
newgroup:hamster-control@snafu.de:hamster.*:verify-hamster-control@snafu.de
rmgroup:hamster-control@snafu.de:hamster.*:verify-hamster-control@snafu.de

## HAN (Korean Hangul)
# Contact: newgroups-request@usenet.or.kr
# Admin group: han.news.admin
# Key URL: ftp://ftp.usenet.or.kr/pub/korea/usenet/pgp/PGPKEY.han
# *PGP*   See README.PGP.
newgroup:*:han.*:drop
rmgroup:*:han.*:drop
checkgroups:newgroups-request@usenet.or.kr:han.*:verify-han.news.admin
newgroup:newgroups-request@usenet.or.kr:han.*:verify-han.news.admin
rmgroup:newgroups-request@usenet.or.kr:han.*:verify-han.news.admin

## HARVARD (*LOCAL* -- Harvard University, Cambridge, MA)
# For local use only.
newgroup:*@*.harvard.edu:harvard.*:log
rmgroup:*@*.harvard.edu:harvard.*:doit=rmgroup

## HAWAII (Hawaii, USA)
checkgroups:news@lava.net:hawaii.*:doit=checkgroups
newgroup:news@lava.net:hawaii.*:doit=newgroup
rmgroup:news@lava.net:hawaii.*:doit=rmgroup

## HFX (Halifax, Nova Scotia)
checkgroups:stevemackie@gmail.com:hfx.*:doit=checkgroups
newgroup:stevemackie@gmail.com:hfx.*:doit=newgroup
rmgroup:stevemackie@gmail.com:hfx.*:doit=rmgroup

## HIV (HIVNET Foundation, for HIV+/AIDS information)
# Contact: news@hivnet.org
# Admin group: hiv.config
# Key fingerprint: 5D D6 0E DC 1E 2D EA 0B  B0 56 4D D6 52 53 D7 A4
# *PGP*   See README.PGP.
newgroup:*:hiv.*:drop
rmgroup:*:hiv.*:drop
checkgroups:news@hivnet.org:hiv.*:verify-news@hivnet.org
newgroup:news@hivnet.org:hiv.*:verify-news@hivnet.org
rmgroup:news@hivnet.org:hiv.*:verify-news@hivnet.org

## HK (Hong Kong)
checkgroups:hknews@comp.hkbu.edu.hk:hk.*:doit=checkgroups
newgroup:hknews@comp.hkbu.edu.hk:hk.*:doit=newgroup
rmgroup:hknews@comp.hkbu.edu.hk:hk.*:doit=rmgroup

## HOUSTON (Houston, Texas, USA)
# Admin group: houston.usenet.config
# *PGP*   See README.PGP.
newgroup:*:houston.*:drop
rmgroup:*:houston.*:drop
checkgroups:news@academ.com:houston.*:verify-houston.usenet.config
newgroup:news@academ.com:houston.*:verify-houston.usenet.config
rmgroup:news@academ.com:houston.*:verify-houston.usenet.config

## HR (Croatian language)
# Contact: newsmaster@carnet.hr
# URL: http://newsfeed.carnet.hr/control/
# Admin group: hr.news.admin
# Key URL: http://newsfeed.carnet.hr/control/key.txt
# Key fingerprint: 0EE5 74FB 1C40 7ADB 0AAC  A52F 7192 1BA3 ED63 AD9A
# Syncable server: news.carnet.hr
# *PGP*   See README.PGP.
newgroup:*:hr.*:drop
rmgroup:*:hr.*:drop
checkgroups:newsmaster@carnet.hr:hr.*:verify-newsmaster@carnet.hr
newgroup:newsmaster@carnet.hr:hr.*:verify-newsmaster@carnet.hr
rmgroup:newsmaster@carnet.hr:hr.*:verify-newsmaster@carnet.hr

## HUMANITYQUEST (*HISTORIC* -- Humanities discussion)
#
# This hierarchy is not entirely defunct, but it receives very little
# traffic and is included primarily for the sake of completeness.
#
# Contact: news-admin@humanityquest.com
# URL: http://www.humanityquest.com/projects/newsgroups/
# Key URL: http://www.humanityquest.com/projects/newsgroups/PGP.htm
# Key fingerprint: BA3D B306 B6F5 52AA BA8F 32F0 8C4F 5040 16F9 C046
# *PGP*   See README.PGP.
newgroup:*:humanityquest.*:drop
rmgroup:*:humanityquest.*:drop
checkgroups:news-admin@humanityquest.com:humanityquest.*:verify-humanityquest.admin.config
newgroup:news-admin@humanityquest.com:humanityquest.*:verify-humanityquest.admin.config
rmgroup:news-admin@humanityquest.com:humanityquest.*:verify-humanityquest.admin.config

## HUN (Hungary)
# URL: http://gatling.ikk.sztaki.hu/~kissg/news/
# Admin group: hun.admin.news
# Key URL: http://gatling.ikk.sztaki.hu/~kissg/news/hun.admin.news.asc
# *PGP*   See README.PGP.
newgroup:*:hun.*:drop
rmgroup:*:hun.*:drop
checkgroups:hun-mnt@news.sztaki.hu:hun.*:verify-hun.admin.news
newgroup:hun-mnt@news.sztaki.hu:hun.*:verify-hun.admin.news
rmgroup:hun-mnt@news.sztaki.hu:hun.*:verify-hun.admin.news

## IA (Iowa, USA)
checkgroups:skunz@iastate.edu:ia.*:doit=checkgroups
newgroup:skunz@iastate.edu:ia.*:doit=newgroup
rmgroup:skunz@iastate.edu:ia.*:doit=rmgroup

## IBMNET (*LOCAL* -- ?)
# Contact: news@ibm.net
# For local use only, contact the above address for information.
newgroup:*:ibmnet.*:log
rmgroup:*:ibmnet.*:doit=rmgroup

## ICONZ (*LOCAL* -- The Internet Company of New Zealand, New Zealand)
# Contact: usenet@iconz.co.nz
# For local use only, contact the above address for information.
newgroup:*:iconz.*:log
rmgroup:*:iconz.*:doit=rmgroup

## IDOCTRA (Idoctra Translation Software, Translation Discussion)
# Contact: support@idoctra.com
checkgroups:support@idoctra.com:idoctra.*:doit=checkgroups
newgroup:support@idoctra.com:idoctra.*:doit=newgroup
rmgroup:support@idoctra.com:idoctra.*:doit=rmgroup

## IE (Ireland)
# Contact: control@usenet.ie
# Admin group: ie.news.group
# *PGP*   See README.PGP.
newgroup:*:ie.*:drop
rmgroup:*:ie.*:drop
checkgroups:control@usenet.ie:ie.*:verify-control@usenet.ie
newgroup:control@usenet.ie:ie.*:verify-control@usenet.ie
rmgroup:control@usenet.ie:ie.*:verify-control@usenet.ie

## IEEE (*DEFUNCT* -- Institute of Electrical and Electronic Engineers)
# Contact: postoffice@ieee.org
# This hierarchy is defunct.  Please remove it.
newgroup:*:ieee.*:log
rmgroup:*:ieee.*:doit=rmgroup

## INFO (*DEFUNCT* -- Gatewayed mailing lists)
# This hierarchy is defunct.  Please remove it.
newgroup:rjoyner@uiuc.edu:info.*:log
rmgroup:rjoyner@uiuc.edu:info.*:doit=rmgroup

## IS (Iceland)
# Contact: IS Group Admins <group-admin@usenet.is>
# URL: http://www.usenet.is/
# Admin group: is.isnet
# Key URL: http://www.usenet.is/group-admin.asc
# Key fingerprint: 33 32 8D 46 1E 5E 1C 7F  48 60 8E 72 E5 3E CA EA
# *PGP*   See README.PGP.
newgroup:*:is.*:drop
rmgroup:*:is.*:drop
checkgroups:group-admin@usenet.is:is.*:verify-group-admin@usenet.is
newgroup:group-admin@usenet.is:is.*:verify-group-admin@usenet.is
rmgroup:group-admin@usenet.is:is.*:verify-group-admin@usenet.is

## ISC (Japanese ?)
checkgroups:news@sally.isc.chubu.ac.jp:isc.*:doit=checkgroups
newgroup:news@sally.isc.chubu.ac.jp:isc.*:doit=newgroup
rmgroup:news@sally.isc.chubu.ac.jp:isc.*:doit=rmgroup

## ISRAEL & IL (Israel)
newgroup:news@news.biu.ac.il:israel.*:doit=newgroup
rmgroup:news@news.biu.ac.il:israel.*|il.*:doit=rmgroup

## ISU (I-Shou University, Taiwan)
# Contact: news@news.isu.edu.tw
# URL: http://news.isu.edu.tw/
# Admin group: isu.newgroups
# Key URL: http://news.isu.edu.tw/isu.asc
# *PGP*   See README.PGP.
newgroup:*:isu.*:drop
rmgroup:*:isu.*:drop
checkgroups:news@news.isu.edu.tw:isu.*:verify-news@news.isu.edu.tw
newgroup:news@news.isu.edu.tw:isu.*:verify-news@news.isu.edu.tw
rmgroup:news@news.isu.edu.tw:isu.*:verify-news@news.isu.edu.tw

## IT (Italian)
# Contact: gcn@news.nic.it
# URL: http://www.news.nic.it/
# Admin group: it.news.annunci
# Key URL: http://www.news.nic.it/pgp.txt
# Key fingerprint: 94 A4 F7 B5 46 96 D6 C7  A6 73 F2 98 C4 8C D0 E0
# *PGP*   See README.PGP.
newgroup:*:it.*:drop
rmgroup:*:it.*:drop
checkgroups:gcn@news.nic.it:it.*:verify-gcn@news.nic.it
newgroup:gcn@news.nic.it:it.*:verify-gcn@news.nic.it
rmgroup:gcn@news.nic.it:it.*:verify-gcn@news.nic.it

## IT-ALT (Alternate Italian)
#
# There is no one official control message issuer for the it-alt.*
# hierarchy, so this file doesn't choose any particular one.  Several
# different people issue control messages for this hierarchy, which may
# or may not agree, and sites carrying this hierarchy are encouraged to
# pick one and add it below.
#
# Newgroup and removal requests are to be posted to it-alt.config.  A list
# of people issuing PGP/GPG signed control messages is available in a
# periodic posting to news.admin.hierarchies and it-alt.config.
#
newgroup:*:it-alt.*:drop
rmgroup:*:it-alt.*:drop

## ITALIA (Italy)
# Contact: news@news.cineca.it
# URL: http://news.cineca.it/italia/
# Admin group: italia.announce.newgroups
# Key URL: http://news.cineca.it/italia/italia-pgp.txt
# Key fingerprint: 0F BB 71 62 DA 5D 5D B8  D5 86 FC 28 02 67 1A 6B
# *PGP*   See README.PGP.
newgroup:*:italia.*:drop
rmgroup:*:italia.*:drop
checkgroups:news@news.cineca.it:italia.*:verify-italia.announce.newgroups
newgroup:news@news.cineca.it:italia.*:verify-italia.announce.newgroups
rmgroup:news@news.cineca.it:italia.*:verify-italia.announce.newgroups

## IU (Indiana University)
newgroup:news@usenet.ucs.indiana.edu:iu.*:doit=newgroup
newgroup:root@usenet.ucs.indiana.edu:iu.*:doit=newgroup
newgroup:*@usenet.ucs.indiana.edu:iu*class.*:log
rmgroup:news@usenet.ucs.indiana.edu:iu.*:doit=rmgroup
rmgroup:root@usenet.ucs.indiana.edu:iu.*:doit=rmgroup

## JAPAN (Japan)
# Contact: Tsuneo Tanaka <tt+null@efnet.com>
# URL: http://www.asahi-net.or.jp/~AE5T-KSN/japan-e.html
# Admin group: japan.admin.announce
# Key URL: http://grex.cyberspace.org/~tt/japan.admin.announce.asc
# Key fingerprint: 6A FA 19 47 69 1B 10 74  38 53 4B 1B D8 BA 3E 85
# *PGP*   See README.PGP.
newgroup:*:japan.*:drop
rmgroup:*:japan.*:drop
checkgroups:japan.admin.announce@news.efnet.com:japan.*:verify-japan.admin.announce@news.efnet.com
newgroup:japan.admin.announce@news.efnet.com:japan.*:verify-japan.admin.announce@news.efnet.com
rmgroup:japan.admin.announce@news.efnet.com:japan.*:verify-japan.admin.announce@news.efnet.com

## JLUG (Japan Linux Users Group)
# Contact: news@linux.or.jp
# URL: http://www.linux.or.jp/community/news/index.html
# Admin group: jlug.config
# Key URL: http://www.linux.or.jp/pgpkey/news
# *PGP*   See README.PGP.
newgroup:*:jlug.*:drop
rmgroup:*:jlug.*:drop
checkgroups:news@linux.or.jp:jlug.*:verify-news@linux.or.jp
newgroup:news@linux.or.jp:jlug.*:verify-news@linux.or.jp
rmgroup:news@linux.or.jp:jlug.*:verify-news@linux.or.jp

## JUNK (*RESERVED* -- Used for unwanted newsgroups)
#
# The junk newsgroup is reserved by RFC 5536 and MUST NOT be used.  It is
# used by some implementations to store messages to unwanted newsgroups.
# The junk.* hierarchy is not reserved by RFC 5536, but it's marked
# reserved here because, given the special meaning of the junk group,
# using it for any other purpose would be confusing and might trigger
# implementation bugs.
#
newgroup:*:junk.*:drop
rmgroup:*:junk.*:drop

## K12 (US Educational Network)
# URL: http://www.k12groups.org/
checkgroups:braultr@*csmanoirs.qc.ca:k12.*:doit=checkgroups
newgroup:braultr@*csmanoirs.qc.ca:k12.*:doit=newgroup
rmgroup:braultr@*csmanoirs.qc.ca:k12.*:doit=rmgroup

## KA (*PRIVATE* -- Karlsruhe, Germany)
# Contact: usenet@karlsruhe.org
# URL: http://www.karlsruhe.org/
# Admin group: ka.admin
# Key URL: http://www.karlsruhe.org/pubkey-news.karlsruhe.org.asc
# Key fingerprint: DE 19 BB 25 76 19 81 17  F0 67 D2 23 E8 C8 7C 90
# For private use only, contact the above address for information.
# *PGP*   See README.PGP.
newgroup:*:ka.*:drop
rmgroup:*:ka.*:drop
# The following three lines are only for authorized ka.* sites.
#checkgroups:usenet@karlsruhe.org:ka.*:verify-usenet@karlsruhe.org
#newgroup:usenet@karlsruhe.org:ka.*:verify-usenet@karlsruhe.org
#rmgroup:usenet@karlsruhe.org:ka.*:verify-usenet@karlsruhe.org

## KANTO (?)
# *PGP*   See README.PGP.
rmgroup:*:kanto.*:drop
checkgroups:ty@kamoi.imasy.or.jp:kanto.*:verify-kanto.news.network
# NOTE: newgroups aren't verified...
newgroup:*@*.jp:kanto.*:doit=newgroup
rmgroup:ty@kamoi.imasy.or.jp:kanto.*:verify-kanto.news.network

## KASSEL (Kassel, Germany)
# Admin group: kassel.admin
# *PGP*   See README.PGP.
newgroup:*:kassel.*:drop
rmgroup:*:kassel.*:drop
checkgroups:dirk.meyer@dinoex.sub.org:kassel.*:verify-kassel-admin
newgroup:dirk.meyer@dinoex.sub.org:kassel.*:verify-kassel-admin
rmgroup:dirk.meyer@dinoex.sub.org:kassel.*:verify-kassel-admin

## KC (Kansas City, Kansas/Missouri, USA)
checkgroups:dan@sky.net:kc.*:doit=checkgroups
newgroup:dan@sky.net:kc.*:doit=newgroup
rmgroup:dan@sky.net:kc.*:doit=rmgroup

## KGK (Administered by KGK, Japan)
# Contact: Keiji KOSAKA <kgk@film.rlss.okayama-u.ac.jp>
# URL: http://film.rlss.okayama-u.ac.jp/~kgk/kgk/index.html
# Admin group: kgk.admin
checkgroups:usenet@film.rlss.okayama-u.ac.jp:kgk.*:doit=checkgroups
newgroup:usenet@film.rlss.okayama-u.ac.jp:kgk.*:doit=newgroup
rmgroup:usenet@film.rlss.okayama-u.ac.jp:kgk.*:doit=rmgroup

## KIEL (Kiel, Germany)
#
# This hierarchy is still in use, but it has no active maintainer.
# Control messages for this hierarchy should not be honored without
# confirming that the sender is the new hierarchy maintainer.
#
# Admin group: kiel.sysop

## KRST (*LOCAL* -- University of Oslo, Norway)
# Contact: jani@ifi.uio.no
# For local use only, contact the above address for information.
newgroup:*:krst.*:log
rmgroup:*:krst.*:doit=rmgroup

## KWNET (*LOCAL* -- Kitchener-Waterloo?)
# Contact: Ed Hew <edhew@xenitec.on.ca>
# For local use only, contact the above address for information.
newgroup:*:kwnet.*:log
rmgroup:*:kwnet.*:doit=rmgroup

## LAW (?)
# Contact: Jim Burke <jburke@kentlaw.edu>
checkgroups:*@*.kentlaw.edu:law.*:doit=checkgroups
checkgroups:*@*.law.vill.edu:law.*:doit=checkgroups
newgroup:*@*.kentlaw.edu:law.*:doit=newgroup
newgroup:*@*.law.vill.edu:law.*:doit=newgroup
rmgroup:*@*.kentlaw.edu:law.*:doit=rmgroup
rmgroup:*@*.law.vill.edu:law.*:doit=rmgroup

## LINUX (Gated Linux mailing lists)
# Contact: Marco d'Itri <md@linux.it>
# Admin group: linux.admin.news
# Key fingerprint: 81 B3 27 99 4F CE 32 D1  1B C9 01 0D BB B3 2E 41
# *PGP*   See README.PGP.
newgroup:*:linux.*:drop
rmgroup:*:linux.*:drop
checkgroups:linux-admin@bofh.it:linux.*:verify-linux-admin@bofh.it
newgroup:linux-admin@bofh.it:linux.*:verify-linux-admin@bofh.it
rmgroup:linux-admin@bofh.it:linux.*:verify-linux-admin@bofh.it

## LOCAL (*RESERVED* -- Local-only groups)
#
# Historically reserved for local groups and occasionally configured for
# such groups in news software.  It is not really a good idea for sites
# to use this hierarchy for local groups, since they may occur on many
# unconnected sites and may confuse news readers that read at multiple
# sites.
#
newgroup:*:local.*:drop
rmgroup:*:local.*:drop

## LUEBECK (Luebeck, Germany)
# Contact: usenet@zybrkat.org
# Admin group: luebeck.admin
checkgroups:usenet@zybrkat.org:luebeck.*:doit=checkgroups
newgroup:usenet@zybrkat.org:luebeck.*:doit=newgroup
rmgroup:usenet@zybrkat.org:luebeck.*:doit=rmgroup

## MALTA (Nation of Malta)
# Contact: cmeli@cis.um.edu.mt
# URL: http://www.malta.news-admin.org/
# Admin group: malta.config
# Key URL: http://www.cis.um.edu.mt/news-malta/PGP.PUBLICKEY
# Key fingerprint: 20 17 01 5C F0 D0 1A 42  E4 13 30 58 0B 14 48 A6
# *PGP*   See README.PGP.
newgroup:*:malta.*:drop
rmgroup:*:malta.*:drop
checkgroups:cmeli@cis.um.edu.mt:malta.*:verify-malta.config
newgroup:cmeli@cis.um.edu.mt:malta.*:verify-malta.config
rmgroup:cmeli@cis.um.edu.mt:malta.*:verify-malta.config

## MANAWATU (*LOCAL* -- Manawatu district, New Zealand)
# Contact: alan@manawatu.gen.nz or news@manawatu.gen.nz
# For local use only, contact the above address for information.
newgroup:*:manawatu.*:log
rmgroup:*:manawatu.*:doit=rmgroup

## MAUS (MausNet, Germany)
# Admin group: maus.info
# Key fingerprint: 82 52 C7 70 26 B9 72 A1  37 98 55 98 3F 26 62 3E
# *PGP*   See README.PGP.
newgroup:*:maus.*:drop
rmgroup:*:maus.*:drop
checkgroups:guenter@gst0hb.hb.provi.de:maus.*:verify-maus-info
checkgroups:guenter@gst0hb.north.de:maus.*:verify-maus-info
newgroup:guenter@gst0hb.hb.provi.de:maus.*:verify-maus-info
newgroup:guenter@gst0hb.north.de:maus.*:verify-maus-info
rmgroup:guenter@gst0hb.hb.provi.de:maus.*:verify-maus-info
rmgroup:guenter@gst0hb.north.de:maus.*:verify-maus-info

## MCMASTER (*LOCAL* -- McMaster University, Ontario)
# Contact: Brian Beckberger <news@informer1.cis.mcmaster.ca>
# For local use only, contact the above address for information.
newgroup:*:mcmaster.*:log
rmgroup:*:mcmaster.*:doit=rmgroup

## MCOM (*LOCAL* -- Netscape Inc, USA)
# For local use only.
newgroup:*:mcom.*:log
rmgroup:*:mcom.*:doit=rmgroup

## ME (Maine, USA)
checkgroups:kerry@maine.maine.edu:me.*:doit=checkgroups
newgroup:kerry@maine.maine.edu:me.*:doit=newgroup
rmgroup:kerry@maine.maine.edu:me.*:doit=rmgroup

## MEDLUX (All-Russia medical teleconferences)
# URL: ftp://ftp.medlux.ru/pub/news/medlux.grp
checkgroups:neil@new*.medlux.ru:medlux.*:doit=checkgroups
newgroup:neil@new*.medlux.ru:medlux.*:doit=newgroup
rmgroup:neil@new*.medlux.ru:medlux.*:doit=rmgroup

## MELB (Melbourne, Australia)
# Contact: ausadmin@aus.news-admin.org
# URL: http://melb.news-admin.org/
# Key URL: http://aus.news-admin.org/ausadmin.asc
# *PGP*   See README.PGP.
newgroup:*:melb.*:drop
rmgroup:*:melb.*:drop
# (the checkgroups of ausadmin@aus.news-admin.org for melb.* are in the line for wa.* below)
newgroup:ausadmin@aus.news-admin.org:melb.*:verify-ausadmin@aus.news-admin.org
rmgroup:ausadmin@aus.news-admin.org:melb.*:verify-ausadmin@aus.news-admin.org

## MENSA (The Mensa Organisation)
# Contact: usenet@newsgate.mensa.org
# Admin group: mensa.config
# Key fingerprint: 52B9 3963 85D9 0806 8E19  7344 973C 5005 DC7D B7A7
# *PGP*   See README.PGP.
newgroup:*:mensa.*:drop
rmgroup:*:mensa.*:drop
checkgroups:usenet@newsgate.mensa.org:mensa.*:verify-mensa.config
newgroup:usenet@newsgate.mensa.org:mensa.*:verify-mensa.config
rmgroup:usenet@newsgate.mensa.org:mensa.*:verify-mensa.config

## METOCEAN (ISP in Japan)
checkgroups:fwataru@*.metocean.co.jp:metocean.*:doit=checkgroups
newgroup:fwataru@*.metocean.co.jp:metocean.*:doit=newgroup
rmgroup:fwataru@*.metocean.co.jp:metocean.*:doit=rmgroup

## METROPOLIS (*LOCAL* -- ?)
# Contact: newsmaster@worldonline.nl
# For local use only, contact the above address for information.
newgroup:*:metropolis.*:log
rmgroup:*:metropolis.*:doit=rmgroup

## MI (Michigan, USA)
# Contact: Steve Simmons <scs@lokkur.dexter.mi.us>
checkgroups:scs@lokkur.dexter.mi.us:mi.*:doit=checkgroups
newgroup:scs@lokkur.dexter.mi.us:mi.*:doit=newgroup
rmgroup:scs@lokkur.dexter.mi.us:mi.*:doit=rmgroup

## MICROSOFT (Microsoft Corporation, USA)
#
# Control articles for that hierarchy are not issued by Microsoft itself
# but by a Usenet active participant in order to improve the quality of
# the propagation of Microsoft newsgroups.  Their official URL is:
# http://www.microsoft.com/communities/newsgroups/list/en-us/default.aspx
#
# Contact: control-microsoft@trigofacile.com
# URL: http://www.trigofacile.com/divers/usenet/clefs/index.htm
# Admin group: microsoft.public.news.server
# Key URL: http://www.trigofacile.com/divers/usenet/clefs/pgpkey-microsoft.asc
# Key fingerprint: DF70 5FC9 F615 D52E 02DB  A3CB 63A9 8D13 E60E 2FAA
# Syncable server: msnews.microsoft.com
# *PGP*   See README.PGP.
newgroup:*:microsoft.*:drop
rmgroup:*:microsoft.*:drop
checkgroups:control-microsoft@trigofacile.com:microsoft.*:verify-control-microsoft@trigofacile.com
newgroup:control-microsoft@trigofacile.com:microsoft.*:verify-control-microsoft@trigofacile.com
rmgroup:control-microsoft@trigofacile.com:microsoft.*:verify-control-microsoft@trigofacile.com

## MILW (Milwaukee, Wisconsin, USA)
# Contact: milw@usenet.mil.wi.us
# URL: http://usenet.mil.wi.us/
# Admin group: milw.config
# Key URL: http://usenet.mil.wi.us/pgpkey
# Key fingerprint: 6E 9B 9F 70 98 AB 9C E5  C3 C0 05 82 21 5B F4 9E
# *PGP*   See README.PGP.
newgroup:*:milw.*:drop
rmgroup:*:milw.*:drop
checkgroups:milw@usenet.mil.wi.us:milw.*:verify-milw.config
newgroup:milw@usenet.mil.wi.us:milw.*:verify-milw.config
rmgroup:milw@usenet.mil.wi.us:milw.*:verify-milw.config

## MOD (*DEFUNCT* -- Original top level moderated hierarchy)
# This hierarchy is defunct.  Please remove it.
newgroup:*:mod.*:log
rmgroup:*:mod.*:doit=rmgroup

## MUC (Munchen [Munich], Germany)
# Admin group: muc.admin
# Key URL: http://home.arcor.de/andreas-barth/muc-admin.html
# Key fingerprint: 43 C7 0E 7C 45 C7 06 E0  BD 6F 76 CE 07 39 5E 66
# *PGP*   See README.PGP.
newgroup:*:muc.*:drop
rmgroup:*:muc.*:drop
checkgroups:muc-cmsg@muenchen.pro-bahn.org:muc.*:verify-muc.admin
newgroup:muc-cmsg@muenchen.pro-bahn.org:muc.*:verify-muc.admin
rmgroup:muc-cmsg@muenchen.pro-bahn.org:muc.*:verify-muc.admin

## NAGASAKI-U (Nagasaki University, Japan ?)
checkgroups:root@*nagasaki-u.ac.jp:nagasaki-u.*:doit=checkgroups
newgroup:root@*nagasaki-u.ac.jp:nagasaki-u.*:doit=newgroup
rmgroup:root@*nagasaki-u.ac.jp:nagasaki-u.*:doit=rmgroup

## NAS (*LOCAL* -- NAS, NASA Ames Research Center, USA)
# Contact: news@nas.nasa.gov
# For local use only, contact the above address for information.
newgroup:*:nas.*:log
rmgroup:*:nas.*:doit=rmgroup

## NASA (*LOCAL* -- National Aeronautics and Space Administration, USA)
# Contact: news@nas.nasa.gov
# For local use only, contact the above address for information.
newgroup:*:nasa.*:log
rmgroup:*:nasa.*:doit=rmgroup

## NC (North Carolina, USA)
#
# This hierarchy is still in use, but it has no active maintainer.
# Control messages for this hierarchy should not be honored without
# confirming that the sender is the new hierarchy maintainer.

## NCF (*LOCAL* -- National Capital Freenet, Ottawa, Ontario, Canada)
# Contact: news@freenet.carleton.ca
# For local use only, contact the above address for information.
newgroup:*:ncf.*:log
rmgroup:*:ncf.*:doit=rmgroup

## NCTU (Taiwan)
checkgroups:chen@cc.nctu.edu.tw:nctu.*:doit=checkgroups
newgroup:chen@cc.nctu.edu.tw:nctu.*:doit=newgroup
rmgroup:chen@cc.nctu.edu.tw:nctu.*:doit=rmgroup

## NCU (*LOCAL* -- National Central University, Taiwan)
# Contact: Ying-Hao Chang <aqlott@db.csie.ncu.edu.tw>
# Contact: <runn@news.ncu.edu.tw>
# For local use only, contact the above address for information.
newgroup:*:ncu.*:log
rmgroup:*:ncu.*:doit=rmgroup

## NERSC (National Energy Research Scientific Computing Center)
#
# This hierarchy is still in use, but it has no active maintainer.
# Control messages for this hierarchy should not be honored without
# confirming that the sender is the new hierarchy maintainer.
#
# Contact: usenet@nersc.gov

## NET (*HISTORIC* -- Usenet 2)
#
# This was a failed experiment in a different newsgroup creation policy and
# administrative policy which has now been abandoned.
#
# (This was also the original unmoderated Usenet hierarchy from before the
# Great Renaming.  The groups that used to be in net.* in the 1980s are now
# in the Big Eight hierarchies.)
#
newgroup:*:net.*:drop
rmgroup:*:net.*:drop

## NETINS (*LOCAL* -- netINS, Inc)
# Contact: news@netins.net
# For local use only, contact the above address for information.
newgroup:*:netins.*:log
rmgroup:*:netins.*:doit=rmgroup

## NETSCAPE (Netscape Communications Corp)
# Contact: news@netscape.com
# URL: http://www.mozilla.org/community.html
# Admin group: netscape.public.admin
# Key fingerprint: B7 80 55 12 1F 9C 17 0B  86 66 AD 3B DB 68 35 EC
# *PGP*   See README.PGP.
newgroup:*:netscape.*:drop
rmgroup:*:netscape.*:drop
checkgroups:news@netscape.com:netscape.*:verify-netscape.public.admin
newgroup:news@netscape.com:netscape.*:verify-netscape.public.admin
rmgroup:news@netscape.com:netscape.*:verify-netscape.public.admin

## NEWS4US (*LOCAL* -- NEWS4US dot NL, Netherlands)
# Contact: info@news4us.nl
# For local use only, contact the above address for information.
newgroup:*:news4us.*:log
rmgroup:*:news4us.*:doit=rmgroup

## NF (Newfoundland and Labrador, Canada)
# Contact: randy@mun.ca
# (the checkgroups of randy@mun.ca for nf.* are in the line for sj.* below)
newgroup:randy@mun.ca:nf.*:doit=newgroup
rmgroup:randy@mun.ca:nf.*:doit=rmgroup

## NIAGARA (Niagara Peninsula, USA/Canada)
checkgroups:news@niagara.com:niagara.*:doit=checkgroups
newgroup:news@niagara.com:niagara.*:doit=newgroup
rmgroup:news@niagara.com:niagara.*:doit=rmgroup

## NIAS (Japanese ?)
checkgroups:news@cc.nias.ac.jp:ehime-u.*|nias.*:doit=checkgroups
newgroup:news@cc.nias.ac.jp:nias.*:doit=newgroup
rmgroup:news@cc.nias.ac.jp:nias.*:doit=rmgroup

## NIGERIA (Nigeria)
checkgroups:news@easnet.net:nigeria.*:doit=checkgroups
newgroup:news@easnet.net:nigeria.*:doit=newgroup
rmgroup:news@easnet.net:nigeria.*:doit=rmgroup

## NIHON (Japan)
checkgroups:ktomita@jade.dti.ne.jp:nihon.*:doit=checkgroups
newgroup:ktomita@jade.dti.ne.jp:nihon.*:doit=newgroup
rmgroup:ktomita@jade.dti.ne.jp:nihon.*:doit=rmgroup

## NIPPON (*PRIVATE* -- Japan)
# URL: http://www.gcd.org/news/nippon/
# Admin group: nippon.news.group
# Key URL: http://www.gcd.org/news/nippon/
# Key fingerprint: BC CF 15 CD B1 3C DF B3  C3 DE 35 6F 2F F7 46 DB
# For private use only.
# *PGP*   See README.PGP.
newgroup:*:nippon.*:drop
rmgroup:*:nippon.*:drop
newgroup:news@gcd.org:nippon.*:log
rmgroup:news@gcd.org:nippon.*:verify-nippon.news.group

## NJ (New Jersey, USA)
# Contact: nj-admin@gunslinger.net
# URL: http://www.exit109.com/~jeremy/nj/
checkgroups:nj-admin@gunslinger.net:nj.*:doit=checkgroups
newgroup:nj-admin@gunslinger.net:nj.*:doit=newgroup
rmgroup:nj-admin@gunslinger.net:nj.*:doit=rmgroup

## NL (Netherlands)
# Contact: nl-admin@nic.surfnet.nl
# URL: http://nl.news-admin.org/info/nladmin.html
# Admin group: nl.newsgroups
# Key fingerprint: 45 20 0B D5 A1 21 EA 7C  EF B2 95 6C 25 75 4D 27
# *PGP*   See README.PGP.
newgroup:*:nl.*:drop
rmgroup:*:nl.*:drop
checkgroups:nl-admin@nic.surfnet.nl:nl.*:verify-nl.newsgroups
newgroup:nl-admin@nic.surfnet.nl:nl.*:verify-nl.newsgroups
rmgroup:nl-admin@nic.surfnet.nl:nl.*:verify-nl.newsgroups

## NL-ALT (Alternative Netherlands groups)
# Key fingerprint: 6B 62 EB 53 4D 5D 2F 96  35 D9 C8 9C B0 65 0E 4C
# *PGP*   See README.PGP.
checkgroups:nl-alt-janitor@surfer.xs4all.nl:nl-alt.*:verify-nl-alt.config.admin
newgroup:*:nl-alt.*:doit=newgroup
rmgroup:nl-alt-janitor@surfer.xs4all.nl:nl-alt.*:verify-nl-alt.config.admin
rmgroup:news@kink.xs4all.nl:nl-alt.*:verify-nl-alt.config.admin

## NLO (Open Source / Free Software, hosted by nl.linux.org)
# URL: http://mail.nl.linux.org/
# Key fingerprint: 63 DC B2 51 0A F3 DD 72  C2 BD C6 FD C1 C5 44 CF
# Syncable server: news.nl.linux.org
# *PGP*   See README.PGP.
newgroup:*:nlo.*:drop
rmgroup:*:nlo.*:drop
checkgroups:news@nl.linux.org:nlo.*:verify-nlo.newsgroups
newgroup:news@nl.linux.org:nlo.*:verify-nlo.newsgroups
rmgroup:news@nl.linux.org:nlo.*:verify-nlo.newsgroups

## NM (New Mexico, USA)
checkgroups:news@tesuque.cs.sandia.gov:nm.*:doit=checkgroups
newgroup:news@tesuque.cs.sandia.gov:nm.*:doit=newgroup
rmgroup:news@tesuque.cs.sandia.gov:nm.*:doit=rmgroup

## NNTPWORLD (General discussion)
# Contact: usenet@nntpworld.net
# URL: http://www.nntpworld.net/
# Admin group: nntpworld.config
# Key URL: http://www.nntpworld.net/nntpworld.asc
# Key fingerprint: C7 BB E0 56 7A 8A 5A 86  09 B5 E3 29 E1 BB 8C E1
# *PGP*   See README.PGP.
newgroup:*:nntpworld.*:drop
rmgroup:*:nntpworld.*:drop
checkgroups:usenet@nntpworld.net:nntpworld.*:verify-usenet@nntpworld.net
newgroup:usenet@nntpworld.net:nntpworld.*:verify-usenet@nntpworld.net
rmgroup:usenet@nntpworld.net:nntpworld.*:verify-usenet@nntpworld.net

## NO (Norway)
# URL: http://www.usenet.no/
# Admin group: no.usenet.admin
# Key URL: http://www.usenet.no/pgp-key.txt
# *PGP*   See README.PGP.
newgroup:*:no.*:drop
rmgroup:*:no.*:drop
checkgroups:control@usenet.no:no.*:verify-no-hir-control
newgroup:control@usenet.no:no.*:verify-no-hir-control
rmgroup:control@usenet.no:no.*:verify-no-hir-control

## NO.ALT (Norway alternative hierarchy)
# *PGP*   See README.PGP.
newgroup:*:no.alt.*:drop
rmgroup:*:no.alt.*:drop
newgroup:*@*.no:no.alt.*:doit=newgroup
rmgroup:control@usenet.no:no.alt.*:verify-no-hir-control

## NORD (Northern Germany)
#
# This hierarchy is still in use, but it has no active maintainer.
# Control messages for this hierarchy should not be honored without
# confirming that the sender is the new hierarchy maintainer.

## NRW (Northrine-Westfalia, Germany)
# Contact: moderator@nrw.usenetverwaltung.de
# URL: http://nrw.usenetverwaltung.de/
# Admin group: nrw.admin.announce
# Key URL: http://nrw.usenetverwaltung.de/pgp/nrw.asc
# Key fingerprint: 13 4A 80 FE D6 34 B4 64  AF 32 08 3F 62 0E B1 E2
# *PGP*   See README.PGP.
newgroup:*:nrw.*:drop
rmgroup:*:nrw.*:drop
checkgroups:moderator@nrw.usenetverwaltung.de:nrw.*:verify-moderator@nrw.usenetverwaltung.de
newgroup:moderator@nrw.usenetverwaltung.de:nrw.*:verify-moderator@nrw.usenetverwaltung.de
rmgroup:moderator@nrw.usenetverwaltung.de:nrw.*:verify-moderator@nrw.usenetverwaltung.de

## NV (Nevada)
# (the checkgroups of cshapiro@netcom.com for nv.* are in the line for vegas.* below)
# (the checkgroups of doctor@netcom.com for nv.* are in the line for vegas.* below)
newgroup:cshapiro@netcom.com:nv.*:doit=newgroup
newgroup:doctor@netcom.com:nv.*:doit=newgroup
rmgroup:cshapiro@netcom.com:nv.*:doit=rmgroup
rmgroup:doctor@netcom.com:nv.*:doit=rmgroup

## NY (New York State, USA)
checkgroups:root@ny.psca.com:ny.*:doit=checkgroups
newgroup:root@ny.psca.com:ny.*:doit=newgroup
rmgroup:root@ny.psca.com:ny.*:doit=rmgroup

## NYC (New York City)
# Contact: Perry E. Metzger <perry@piermont.com>
checkgroups:perry@piermont.com:nyc.*:doit=checkgroups
newgroup:perry@piermont.com:nyc.*:doit=newgroup
rmgroup:perry@piermont.com:nyc.*:doit=rmgroup

## NZ (New Zealand)
# Contact: root@usenet.net.nz
# URL: http://www.faqs.org/faqs/usenet/nz-news-hierarchy
# Admin group: nz.net.announce
# Key fingerprint: 07 DF 48 AA D0 ED AA 88  16 70 C5 91 65 3D 1A 28
# *PGP*   See README.PGP.
newgroup:*:nz.*:drop
rmgroup:*:nz.*:drop
checkgroups:root@usenet.net.nz:nz.*:verify-nz-hir-control
newgroup:root@usenet.net.nz:nz.*:verify-nz-hir-control
rmgroup:root@usenet.net.nz:nz.*:verify-nz-hir-control

## OC (Orange County, California, USA)
checkgroups:bob@tsunami.sugarland.unocal.com:oc.*:doit=checkgroups
newgroup:bob@tsunami.sugarland.unocal.com:oc.*:doit=newgroup
rmgroup:bob@tsunami.sugarland.unocal.com:oc.*:doit=rmgroup

## OESTERREICH (Free Austria)
#
# This is apparently another alt.* or free.* but specific to Austria.
# Currently, the ftp.isc.org list doesn't honor newgroup messages in the
# hierarchy due to lack of requests, but here is the information in case
# any news admin wishes to carry it.
#
# URL: http://www.tahina.priv.at/~cm/oe/index.en.html
#newgroup:*:oesterreich.*:doit
#newgroup:group-admin@isc.org:oesterreich.*:drop
#newgroup:tale@*uu.net:oesterreich.*:drop
#rmgroup:*:oesterreich.*:drop

## OH (Ohio, USA)
checkgroups:trier@ins.cwru.edu:oh.*:doit=checkgroups
newgroup:trier@ins.cwru.edu:oh.*:doit=newgroup
rmgroup:trier@ins.cwru.edu:oh.*:doit=rmgroup

## OK (Oklahoma, USA)
checkgroups:quentin@*qns.com:ok.*:doit=checkgroups
newgroup:quentin@*qns.com:ok.*:doit=newgroup
rmgroup:quentin@*qns.com:ok.*:doit=rmgroup

## OKINAWA (Okinawa, Japan)
checkgroups:news@opus.or.jp:okinawa.*:doit=checkgroups
newgroup:news@opus.or.jp:okinawa.*:doit=newgroup
rmgroup:news@opus.or.jp:okinawa.*:doit=rmgroup

## ONT (Ontario, Canada)
checkgroups:pkern@gpu.utcc.utoronto.ca:ont.*:doit=checkgroups
newgroup:pkern@gpu.utcc.utoronto.ca:ont.*:doit=newgroup
rmgroup:pkern@gpu.utcc.utoronto.ca:ont.*:doit=rmgroup

## OPENNEWS (Open News Network)
# URL: http://www.open-news-network.org/
# *PGP*   See README.PGP.
newgroup:*:opennews.*:drop
rmgroup:*:opennews.*:drop
checkgroups:news@news2.open-news-network.org:opennews.*:verify-news@news2.open-news-network.org
newgroup:news@news2.open-news-network.org:opennews.*:verify-news@news2.open-news-network.org
rmgroup:news@news2.open-news-network.org:opennews.*:verify-news@news2.open-news-network.org

## OPENWATCOM (Open Watcom compilers)
# Contact: admin@openwatcom.news-admin.org
# URL: http://www.openwatcom.org/
# Admin group: openwatcom.contributors
# Key URL: http://cmeerw.org/files/openwatcom/pgp-openwatcom.asc
# Syncable server: news.openwatcom.org
# *PGP*   See README.PGP.
newgroup:*:openwatcom.*:drop
rmgroup:*:openwatcom.*:drop
checkgroups:admin@openwatcom.news-admin.org:openwatcom.*:verify-admin@openwatcom.news-admin.org
newgroup:admin@openwatcom.news-admin.org:openwatcom.*:verify-admin@openwatcom.news-admin.org
rmgroup:admin@openwatcom.news-admin.org:openwatcom.*:verify-admin@openwatcom.news-admin.org

## OPERA (Opera Software, Oslo, Norway)
# Contact: usenet@opera.com
# Syncable server: news.opera.com
# *PGP*   See README.PGP.
newgroup:*:opera.*:drop
rmgroup:*:opera.*:drop
checkgroups:*@opera.com:opera.*:verify-opera-group-admin
newgroup:*@opera.com:opera.*:verify-opera-group-admin
rmgroup:*@opera.com:opera.*:verify-opera-group-admin

## OTT (Ottawa, Ontario, Canada)
# Contact: onag@pinetree.org
# URL: http://www.pinetree.org/ONAG/
checkgroups:clewis@ferret.ocunix.on.ca:ott.*:doit=checkgroups
checkgroups:dave@revcan.ca:ott.*:doit=checkgroups
checkgroups:gordon@*pinetree.org:ott.*:doit=checkgroups
checkgroups:news@*pinetree.org:ott.*:doit=checkgroups
checkgroups:news@bnr.ca:ott.*:doit=checkgroups
checkgroups:news@ferret.ocunix.on.ca:ott.*:doit=checkgroups
checkgroups:news@nortel.ca:ott.*:doit=checkgroups
newgroup:clewis@ferret.ocunix.on.ca:ott.*:doit=newgroup
newgroup:dave@revcan.ca:ott.*:doit=newgroup
newgroup:gordon@*pinetree.org:ott.*:doit=newgroup
newgroup:news@*pinetree.org:ott.*:doit=newgroup
newgroup:news@bnr.ca:ott.*:doit=newgroup
newgroup:news@ferret.ocunix.on.ca:ott.*:doit=newgroup
newgroup:news@nortel.ca:ott.*:doit=newgroup
rmgroup:clewis@ferret.ocunix.on.ca:ott.*:doit=rmgroup
rmgroup:dave@revcan.ca:ott.*:doit=rmgroup
rmgroup:gordon@*pinetree.org:ott.*:doit=rmgroup
rmgroup:news@*pinetree.org:ott.*:doit=rmgroup
rmgroup:news@bnr.ca:ott.*:doit=rmgroup
rmgroup:news@ferret.ocunix.on.ca:ott.*:doit=rmgroup
rmgroup:news@nortel.ca:ott.*:doit=rmgroup

## OWL (Ostwestfalen-Lippe, Germany)
#
# This hierarchy is still in use, but it has no active maintainer.
# Control messages for this hierarchy should not be honored without
# confirming that the sender is the new hierarchy maintainer.
#
# Contact: news@owl.de
# Syncable server: news.owl.de

## PA (Pennsylvania, USA)
# URL: http://www.netcom.com/~rb1000/pa_hierarchy/
checkgroups:fxp@epix.net:pa.*:doit=checkgroups
newgroup:fxp@epix.net:pa.*:doit=newgroup
rmgroup:fxp@epix.net:pa.*:doit=rmgroup

## PBINFO (Paderborn, Germany)
# Contact: news@uni-paderborn.de
# *PGP*   See README.PGP.
newgroup:*:pbinfo.*:drop
rmgroup:*:pbinfo.*:drop
checkgroups:postmaster@upb.de:pbinfo.*:verify-news@uni-paderborn.de
newgroup:postmaster@upb.de:pbinfo.*:verify-news@uni-paderborn.de
rmgroup:postmaster@upb.de:pbinfo.*:verify-news@uni-paderborn.de

## PERL (Perl Programming Language)
# Contact: newsadmin@perl.org
# URL: http://www.nntp.perl.org/about/
# Key URL: http://www.nntp.perl.org/about/newsadmin@perl.org.pgp
# Key fingerprint: 438F D1BA 4DCC 3B1A BED8  2BCC 3298 8A7D 8B2A CFBB
# *PGP*   See README.PGP.
newgroup:*:perl.*:drop
rmgroup:*:perl.*:drop
checkgroups:newsadmin@perl.org:perl.*:verify-newsadmin@perl.org
newgroup:newsadmin@perl.org:perl.*:verify-newsadmin@perl.org
rmgroup:newsadmin@perl.org:perl.*:verify-newsadmin@perl.org

## PGH (Pittsburgh, Pennsylvania, USA)
# Admin group: pgh.config
# *PGP*   See README.PGP.
newgroup:*:pgh.*:drop
rmgroup:*:pgh.*:drop
checkgroups:pgh-config@psc.edu:pgh.*:verify-pgh.config
newgroup:pgh-config@psc.edu:pgh.*:verify-pgh.config
rmgroup:pgh-config@psc.edu:pgh.*:verify-pgh.config

## PGSQL (Gated PostgreSQL mailing lists)
# Contact: news@postgresql.org
# URL: http://news.hub.org/gpg_public_keys.html
# Key URL: http://news.hub.org/gpg_public_keys.html
# *PGP*   See README.PGP.
newgroup:*:pgsql.*:drop
rmgroup:*:pgsql.*:drop
checkgroups:news@postgresql.org:pgsql.*:verify-news@postgresql.org
newgroup:news@postgresql.org:pgsql.*:verify-news@postgresql.org
rmgroup:news@postgresql.org:pgsql.*:verify-news@postgresql.org

## PHL (Philadelphia, Pennsylvania, USA)
checkgroups:news@vfl.paramax.com:phl.*:doit=checkgroups
newgroup:news@vfl.paramax.com:phl.*:doit=newgroup
rmgroup:news@vfl.paramax.com:phl.*:doit=rmgroup

## PIN (Personal Internauts' NetNews)
checkgroups:pin-admin@forus.or.jp:pin.*:doit=checkgroups
newgroup:pin-admin@forus.or.jp:pin.*:doit=newgroup
rmgroup:pin-admin@forus.or.jp:pin.*:doit=rmgroup

## PIPEX (UUNET WorldCom UK)
# Contact: Russell Vincent <news-control@ops.pipex.net>
checkgroups:news-control@ops.pipex.net:pipex.*:doit=checkgroups
newgroup:news-control@ops.pipex.net:pipex.*:doit=newgroup
rmgroup:news-control@ops.pipex.net:pipex.*:doit=rmgroup

## PITT (University of Pittsburgh, PA)
checkgroups:news+@pitt.edu:pitt.*:doit=checkgroups
checkgroups:news@toads.pgh.pa.us:pitt.*:doit=checkgroups
newgroup:news+@pitt.edu:pitt.*:doit=newgroup
newgroup:news@toads.pgh.pa.us:pitt.*:doit=newgroup
rmgroup:news+@pitt.edu:pitt.*:doit=rmgroup
rmgroup:news@toads.pgh.pa.us:pitt.*:doit=rmgroup

## PL (Poland and Polish language)
# URL: http://www.usenet.pl/doc/news-pl-new-site-faq.html
# Admin group: pl.news.admin
# Key URL: http://www.usenet.pl/doc/news-pl-new-site-faq.html#pgp
# *PGP*   See README.PGP.
newgroup:*:pl.*:drop
rmgroup:*:pl.*:drop
checkgroups:michalj@*fuw.edu.pl:pl.*:verify-pl.announce.newgroups
checkgroups:newgroup@usenet.pl:pl.*:verify-pl.announce.newgroups
newgroup:michalj@*fuw.edu.pl:pl.*:verify-pl.announce.newgroups
newgroup:newgroup@usenet.pl:pl.*:verify-pl.announce.newgroups
rmgroup:michalj@*fuw.edu.pl:pl.*:verify-pl.announce.newgroups
rmgroup:newgroup@usenet.pl:pl.*:verify-pl.announce.newgroups

## PLANET (*LOCAL* -- PlaNet FreeNZ co-operative, New Zealand)
# Contact: office@pl.net
# For local use only, contact the above address for information.
newgroup:*:planet.*:log
rmgroup:*:planet.*:doit=rmgroup

## PRIMA (*LOCAL* -- prima.ruhr.de/Prima e.V. in Germany)
# Contact: admin@prima.ruhr.de
# For local use only, contact the above address for information.
newgroup:*:prima.*:log
rmgroup:*:prima.*:doit=rmgroup

## PRIVATE (*RESERVED* -- Server-local newsgroups)
#
# Sometimes used for groups intended to be private to a specific server.
# It is not a good idea to use this hierarchy name on any production
# server since they may occur on many unconnected sites.
#
newgroup:*:private.*:drop
rmgroup:*:private.*:drop

## PSU (*LOCAL* -- Penn State University, USA)
# Contact: Dave Barr (barr@math.psu.edu)
# For local use only, contact the above address for information.
newgroup:*:psu.*:log
rmgroup:*:psu.*:doit=rmgroup

## PT (Portugal and Portuguese language)
# URL: http://www.usenet-pt.org/
# Admin group: pt.internet.usenet
# Key URL: http://www.usenet-pt.org/control@usenet-pt.org.asc
# *PGP*   See README.PGP.
newgroup:*:pt.*:drop
rmgroup:*:pt.*:drop
checkgroups:pmelo@*.inescc.pt:pt.*:verify-control@usenet-pt.org
newgroup:pmelo@*.inescc.pt:pt.*:verify-control@usenet-pt.org
rmgroup:pmelo@*.inescc.pt:pt.*:verify-control@usenet-pt.org

## PUBNET (*DEFUNCT* -- ?)
# URL: ftp://ftp.isc.org/pub/usenet/control/pubnet/pubnet.config.gz
# This hierarchy is defunct.  Please remove it.
newgroup:*:pubnet.*:log
rmgroup:*:pubnet.*:doit=rmgroup

## RELCOM (Commonwealth of Independent States)
# URL: ftp://ftp.relcom.ru/pub/relcom/netinfo/
# Admin group: relcom.netnews
# Key URL: ftp://ftp.relcom.ru/pub/relcom/netinfo/coordpubkey.txt
# *PGP*   See README.PGP.
newgroup:*:relcom.*:drop
rmgroup:*:relcom.*:drop
checkgroups:coord@*.relcom.ru:relcom.*:verify-relcom.newsgroups
newgroup:coord@*.relcom.ru:relcom.*:verify-relcom.newsgroups
rmgroup:coord@*.relcom.ru:relcom.*:verify-relcom.newsgroups

## ROCKSOLID (Rocksolid network - novabbs.com)
# Contact: usenet@novabbs.com
# URL: http://news.novabbs.com/hierarchy/
# Admin group: rocksolid.shared.rocksolid
# Key URL: http://news.novabbs.com/hierarchy/rocksolid.txt
# Key fingerprint: FF84 6D73 0B72 CC0C B69B  6480 2A1C 655B F6D1 0B13
# *PGP*   See README.PGP.
newgroup:*:rocksolid.*:drop
rmgroup:*:rocksolid.*:drop
checkgroups:usenet@novabbs.com:rocksolid.*:verify-usenet@novabbs.com
newgroup:usenet@novabbs.com:rocksolid.*:verify-usenet@novabbs.com
rmgroup:usenet@novabbs.com:rocksolid.*:verify-usenet@novabbs.com

## RPI (*LOCAL* -- Rensselaer Polytechnic Institute, Troy, NY, USA)
# Contact: sofkam@rpi.edu
# For local use only, contact the above address for information.
newgroup:*:rpi.*:log
rmgroup:*:rpi.*:doit=rmgroup

## SAAR (Saarland Region, Germany)
# URL: http://www.saar-admin-news.de/
# Admin group: saar.admin.news
# Key URL: http://www.saar-admin-news.de/saar-control.asc
# *PGP*   See README.PGP.
newgroup:*:saar.*:drop
rmgroup:*:saar.*:drop
checkgroups:control@saar-admin-news.de:saar.*:verify-saar-control
newgroup:control@saar-admin-news.de:saar.*:verify-saar-control
rmgroup:control@saar-admin-news.de:saar.*:verify-saar-control

## SACHSNET (Sachsen [Saxony], Germany)
checkgroups:root@lusatia.de:sachsnet.*:doit=checkgroups
newgroup:root@lusatia.de:sachsnet.*:doit=newgroup
rmgroup:root@lusatia.de:sachsnet.*:doit=rmgroup

## SAT (San Antonio, Texas, USA)
# Contact: satgroup@endicor.com
# Admin group: sat.usenet.config
# *PGP*   See README.PGP.
newgroup:*:sat.*:drop
rmgroup:*:sat.*:drop
checkgroups:satgroup@endicor.com:sat.*:verify-satgroup@endicor.com
newgroup:satgroup@endicor.com:sat.*:verify-satgroup@endicor.com
rmgroup:satgroup@endicor.com:sat.*:verify-satgroup@endicor.com

## SBAY (South Bay/Silicon Valley, California)
# URL: http://www.sbay.org/sbay-newsgroups.html
checkgroups:ikluft@thunder.sbay.org:ca.*|sbay.*:doit=checkgroups
checkgroups:steveh@grafex.sbay.org:sbay.*:doit=checkgroups
newgroup:ikluft@thunder.sbay.org:sbay.*:doit=newgroup
newgroup:steveh@grafex.sbay.org:sbay.*:doit=newgroup
rmgroup:ikluft@thunder.sbay.org:sbay.*:doit=rmgroup
rmgroup:steveh@grafex.sbay.org:sbay.*:doit=rmgroup

## SCHULE (?)
# Contact: schule-admin@roxel.ms.sub.org
# URL: http://home.pages.de/~schule-admin/
# Admin group: schule.admin
# Key URL: http://www.afaik.de/usenet/admin/schule/control/schule.asc
# Key fingerprint: 64 06 F0 AE E1 46 85 0C  BD CA 0E 53 8B 1E 73 D2
# *PGP*   See README.PGP.
newgroup:*:schule.*:drop
rmgroup:*:schule.*:drop
checkgroups:newsctrl@schule.de:schule.*:verify-schule.konfig
newgroup:newsctrl@schule.de:schule.*:verify-schule.konfig
rmgroup:newsctrl@schule.de:schule.*:verify-schule.konfig

## SCOT (Scotland)
# URL: http://scot.news-admin.org/
# Admin group: scot.newsgroups.discuss
# Key URL: http://scot.news-admin.org/signature.html
# *PGP*   See README.PGP.
newgroup:*:scot.*:drop
rmgroup:*:scot.*:drop
checkgroups:control@scot.news-admin.org:scot.*:verify-control@scot.news-admin.org
newgroup:control@scot.news-admin.org:scot.*:verify-control@scot.news-admin.org
rmgroup:control@scot.news-admin.org:scot.*:verify-control@scot.news-admin.org

## SCOUT (Scouts and guides)
# URL: http://news.scoutnet.org/
# Admin group: scout.admin
# Key URL: http://news.scoutnet.org/scout-pgpkey.asc
# *PGP*   See README.PGP.
newgroup:*:scout.*:drop
rmgroup:*:scout.*:drop
checkgroups:control@news.scoutnet.org:scout.*:verify-control@news.scoutnet.org
newgroup:control@news.scoutnet.org:scout.*:verify-control@news.scoutnet.org
rmgroup:control@news.scoutnet.org:scout.*:verify-control@news.scoutnet.org

## SDSU (*LOCAL* -- San Diego State University, CA)
# Contact: Craig R. Sadler <usenet@sdsu.edu>
# For local use only, contact the above address for information.
newgroup:*:sdsu.*:log
rmgroup:*:sdsu.*:doit=rmgroup

## SE (Sweden)
# Contact: usenet@usenet-se.net
# Admin group: se.internet.news.meddelanden
# Key fingerprint: 68 03 F0 FD 0C C3 4E 69  6F 0D 0C 60 3C 58 63 96
# *PGP*   See README.PGP.
newgroup:*:se.*:drop
rmgroup:*:se.*:drop
checkgroups:usenet@usenet-se.net:se.*:verify-usenet-se
newgroup:usenet@usenet-se.net:se.*:verify-usenet-se
rmgroup:usenet@usenet-se.net:se.*:verify-usenet-se

## SEATTLE (Seattle, Washington, USA)
checkgroups:billmcc@akita.com:seattle.*:doit=checkgroups
# (the checkgroups of graham@ee.washington.edu for seattle.* are in the line for wash.* below)
newgroup:billmcc@akita.com:seattle.*:doit=newgroup
newgroup:graham@ee.washington.edu:seattle.*:doit=newgroup
rmgroup:billmcc@akita.com:seattle.*:doit=rmgroup
rmgroup:graham@ee.washington.edu:seattle.*:doit=rmgroup

## SFNET (Finland)
# Contact: sfnet@cs.tut.fi
# URL: http://www.cs.tut.fi/sfnet/
# Admin group: sfnet.ryhmat+listat
# Key fingerprint: DE79 33C2 D359 D128 44E5  6A0C B6E3 0E53 6933 A636
# *PGP*   See README.PGP.
newgroup:*:sfnet.*:drop
rmgroup:*:sfnet.*:drop
checkgroups:sfnet@*cs.tut.fi:sfnet.*:verify-sfnet@cs.tut.fi
newgroup:sfnet@*cs.tut.fi:sfnet.*:verify-sfnet@cs.tut.fi
rmgroup:sfnet@*cs.tut.fi:sfnet.*:verify-sfnet@cs.tut.fi

## SHAMASH (Jewish)
checkgroups:archives@israel.nysernet.org:shamash.*:doit=checkgroups
newgroup:archives@israel.nysernet.org:shamash.*:doit=newgroup
rmgroup:archives@israel.nysernet.org:shamash.*:doit=rmgroup

## SI (The Republic of Slovenia)
# URL: http://www.arnes.si/news/config/
# Admin group: si.news.announce.newsgroups
# Key URL: http://www.arnes.si/news/config/
# *PGP*   See README.PGP.
newgroup:*:si.*:drop
rmgroup:*:si.*:drop
checkgroups:news-admin@arnes.si:si.*:verify-si.news.announce.newsgroups
newgroup:news-admin@arnes.si:si.*:verify-si.news.announce.newsgroups
rmgroup:news-admin@arnes.si:si.*:verify-si.news.announce.newsgroups

## SJ (St. John's, Newfoundland and Labrador, Canada)
# Contact: randy@mun.ca
checkgroups:randy@mun.ca:nf.*|sj.*:doit=checkgroups
newgroup:randy@mun.ca:sj.*:doit=newgroup
rmgroup:randy@mun.ca:sj.*:doit=rmgroup

## SK (Slovakia)
checkgroups:uhlar@ccnews.ke.sanet.sk:sk.*:doit=checkgroups
newgroup:uhlar@ccnews.ke.sanet.sk:sk.*:doit=newgroup
rmgroup:uhlar@ccnews.ke.sanet.sk:sk.*:doit=rmgroup

## SLO (San Luis Obispo, CA)
checkgroups:news@punk.net:slo.*:doit=checkgroups
newgroup:news@punk.net:slo.*:doit=newgroup
rmgroup:news@punk.net:slo.*:doit=rmgroup

## SOLENT (Solent region, England)
checkgroups:news@tcp.co.uk:solent.*:doit=checkgroups
newgroup:news@tcp.co.uk:solent.*:doit=newgroup
rmgroup:news@tcp.co.uk:solent.*:doit=rmgroup

## SPOKANE (Spokane, Washington, USA)
checkgroups:usenet@news.spokane.wa.us:spokane.*:doit=checkgroups
newgroup:usenet@news.spokane.wa.us:spokane.*:doit=newgroup
rmgroup:usenet@news.spokane.wa.us:spokane.*:doit=rmgroup

## SSLUG (*PRIVATE* -- Skåne Sjælland Linux User Group)
# URL: http://en.sslug.dk/
# For private use only.
newgroup:sparre@sslug.se:sslug.*:log
rmgroup:sparre@sslug.se:sslug.*:doit=rmgroup

## STAROFFICE (StarOffice business suite, Sun Microsystems, Inc.)
# Contact: news@starnews.sun.com
# Admin group: staroffice.admin
# Key fingerprint: C6 3E 81 6F 2A 19 D3 84  72 51 F9 1B E3 B9 B2 C9
# Syncable server: starnews.sun.com
# *PGP*   See README.PGP.
newgroup:*:staroffice.*:drop
rmgroup:*:staroffice.*:drop
checkgroups:news@stardivision.de:staroffice.*:verify-staroffice.admin
newgroup:news@stardivision.de:staroffice.*:verify-staroffice.admin
rmgroup:news@stardivision.de:staroffice.*:verify-staroffice.admin

## STGT (Stuttgart, Germany)
# Contact: news@news.uni-stuttgart.de
# URL: http://news.uni-stuttgart.de/hierarchie/stgt/
# Admin group: stgt.net
# Key URL: http://news.uni-stuttgart.de/hierarchie/stgt/stgt-control.txt
# Key fingerprint: BA A4 0A 54 8F F5 F5 1E  72 48 51 AE 09 51 DE 44
# *PGP*   See README.PGP.
newgroup:*:stgt.*:drop
rmgroup:*:stgt.*:drop
checkgroups:stgt-control@news.uni-stuttgart.de:stgt.*:verify-stgt-control
newgroup:stgt-control@news.uni-stuttgart.de:stgt.*:verify-stgt-control
rmgroup:stgt-control@news.uni-stuttgart.de:stgt.*:verify-stgt-control

## STL (Saint Louis, Missouri, USA)
checkgroups:news@icon-stl.net:stl.*:doit=checkgroups
newgroup:news@icon-stl.net:stl.*:doit=newgroup
rmgroup:news@icon-stl.net:stl.*:doit=rmgroup

## SU (*LOCAL* -- Stanford University, USA)
# Contact: news@news.stanford.edu
# For local use only, contact the above address for information.
newgroup:*:su.*:log
rmgroup:*:su.*:doit=rmgroup

## SUNET (Swedish University Network)
checkgroups:ber@*.sunet.se:sunet.*:doit=checkgroups
# (ber@sunic.sunet.se also matches this line; its checkgroups for sunet.* are in its line for swnet.* below)
newgroup:ber@*.sunet.se:sunet.*:doit=newgroup
rmgroup:ber@*.sunet.se:sunet.*:doit=rmgroup

## SURFNET (Dutch Universities network)
checkgroups:news@info.nic.surfnet.nl:surfnet.*:doit=checkgroups
newgroup:news@info.nic.surfnet.nl:surfnet.*:doit=newgroup
rmgroup:news@info.nic.surfnet.nl:surfnet.*:doit=rmgroup

## SWNET (Sverige, Sweden)
checkgroups:ber@sunic.sunet.se:sunet.*|swnet.*:doit=checkgroups
newgroup:ber@sunic.sunet.se:swnet.*:doit=newgroup
rmgroup:ber@sunic.sunet.se:swnet.*:doit=rmgroup

## SYD (Sydney, Australia)
# Contact: ausadmin@aus.news-admin.org
# URL: http://syd.news-admin.org/
# Key URL: http://aus.news-admin.org/ausadmin.asc
# *PGP*   See README.PGP.
newgroup:*:syd.*:drop
rmgroup:*:syd.*:drop
# (the checkgroups of ausadmin@aus.news-admin.org for syd.* are in the line for wa.* below)
newgroup:ausadmin@aus.news-admin.org:syd.*:verify-ausadmin@aus.news-admin.org
rmgroup:ausadmin@aus.news-admin.org:syd.*:verify-ausadmin@aus.news-admin.org

## SZAF (*PRIVATE* -- Szafe im Netz)
# Contact: hirtenrat@szaf.org
# Admin group: szaf.admin
# Key URL: http://news.szaf.org/szaf/szaf-key.txt
# For private use only, contact the above address for information.
# *PGP*   See README.PGP.
newgroup:*:szaf.*:drop
rmgroup:*:szaf.*:drop
newgroup:hirtenrat@szaf.org:szaf.*:log
rmgroup:hirtenrat@szaf.org:szaf.*:verify-Hirtenrat

## T-NETZ (*DEFUNCT* -- Germany)
# This hierarchy is defunct.  Please remove it.
newgroup:*:t-netz.*:log
rmgroup:*:t-netz.*:doit=rmgroup

## TAMU (Texas A&M University)
# Contact: Philip Kizer <news@tamu.edu>
checkgroups:news@tamsun.tamu.edu:tamu.*:doit=checkgroups
newgroup:news@tamsun.tamu.edu:tamu.*:doit=newgroup
rmgroup:news@tamsun.tamu.edu:tamu.*:doit=rmgroup

## TAOS (Taos, New Mexico, USA)
# Contact: Chris Gunn <cgunn@laplaza.org>
checkgroups:cgunn@laplaza.org:taos.*:doit=checkgroups
newgroup:cgunn@laplaza.org:taos.*:doit=newgroup
rmgroup:cgunn@laplaza.org:taos.*:doit=rmgroup

## TCFN (Toronto Free Community Network, Canada)
checkgroups:news@t-fcn.net:tcfn.*:doit=checkgroups
newgroup:news@t-fcn.net:tcfn.*:doit=newgroup
rmgroup:news@t-fcn.net:tcfn.*:doit=rmgroup

## TELE (*LOCAL* -- Tele Danmark Internet)
# Contact: usenet@tdk.net
# For local use only, contact the above address for information.
newgroup:*:tele.*:log
rmgroup:*:tele.*:doit=rmgroup

## TERMVAKT (*LOCAL* -- University of Oslo, Norway)
# Contact: jani@ifi.uio.no
# For local use only, contact the above address for information.
newgroup:*:termvakt.*:log
rmgroup:*:termvakt.*:doit=rmgroup

## TEST (*RESERVED* -- Local test hierarchy)
#
# Historically used as a local test hierarchy.  It is not a good idea to
# use this hierarchy name on any production server since they may occur
# on many unconnected sites.
#
newgroup:*:test.*:drop
rmgroup:*:test.*:drop

## THUR (Thuringia, Germany)
# Contact: usenet@thur.de
# URL: http://www.thur.de/thurnet/old/thurnews.html
# Admin group: thur.net.news.groups
# Key fingerprint: 7E 3D 73 13 93 D4 CA 78  39 DE 3C E7 37 EE 22 F1
# Syncable server: news.thur.de
# *PGP*   See README.PGP.
newgroup:*:thur.*:drop
rmgroup:*:thur.*:drop
checkgroups:usenet@thur.de:thur.*:verify-thur.net.news.groups
newgroup:usenet@thur.de:thur.*:verify-thur.net.news.groups
rmgroup:usenet@thur.de:thur.*:verify-thur.net.news.groups

## TNN (*DEFUNCT* -- The Network News, Japan)
# This hierarchy is defunct.  Please remove it.
newgroup:netnews@news.iij.ad.jp:tnn.*:log
newgroup:tnn@iij-mc.co.jp:tnn.*:log
rmgroup:netnews@news.iij.ad.jp:tnn.*:doit=rmgroup
rmgroup:tnn@iij-mc.co.jp:tnn.*:doit=rmgroup

## TO (*RESERVED* -- Special hierarchy for UUCP point-to-point messages)
#
# Historically, the to.* hierarchy was used with UUCP to send special
# control messages to a particular peer.  This usage is very obsolete,
# but the hierarchy is still special-cased in some news software and
# should not be used.
#
newgroup:*:to.*:drop
rmgroup:*:to.*:drop

## TRIANGLE (Research Triangle, Central North Carolina, USA)
checkgroups:jfurr@acpub.duke.edu:triangle.*:doit=checkgroups
checkgroups:news@news.duke.edu:triangle.*:doit=checkgroups
checkgroups:tas@concert.net:triangle.*:doit=checkgroups
newgroup:jfurr@acpub.duke.edu:triangle.*:doit=newgroup
newgroup:news@news.duke.edu:triangle.*:doit=newgroup
newgroup:tas@concert.net:triangle.*:doit=newgroup
rmgroup:jfurr@acpub.duke.edu:triangle.*:doit=rmgroup
rmgroup:news@news.duke.edu:triangle.*:doit=rmgroup
rmgroup:tas@concert.net:triangle.*:doit=rmgroup

## TUM (Technische Universitaet Muenchen)
checkgroups:news@informatik.tu-muenchen.de:tum.*:doit=checkgroups
newgroup:news@informatik.tu-muenchen.de:tum.*:doit=newgroup
rmgroup:news@informatik.tu-muenchen.de:tum.*:doit=rmgroup

## TW (Taiwan)
checkgroups:ltc@news.cc.nctu.edu.tw:tw.*:doit=checkgroups
newgroup:ltc@news.cc.nctu.edu.tw:tw.*:doit=newgroup
rmgroup:ltc@news.cc.nctu.edu.tw:tw.*:doit=rmgroup

## TW.K-12 (Taiwan K-12 Discussion)
checkgroups:k-12@news.nchu.edu.tw:tw.k-12.*:doit=checkgroups
newgroup:k-12@news.nchu.edu.tw:tw.k-12.*:doit=newgroup
rmgroup:k-12@news.nchu.edu.tw:tw.k-12.*:doit=rmgroup

## TX (Texas, USA)
checkgroups:eric@cirr.com:dfw.*|tx.*:doit=checkgroups
checkgroups:fletcher@cs.utexas.edu:austin.*|tx.*:doit=checkgroups
checkgroups:usenet@academ.com:tx.*:doit=checkgroups
newgroup:eric@cirr.com:tx.*:doit=newgroup
newgroup:fletcher@cs.utexas.edu:tx.*:doit=newgroup
newgroup:usenet@academ.com:tx.*:doit=newgroup
rmgroup:eric@cirr.com:tx.*:doit=rmgroup
rmgroup:fletcher@cs.utexas.edu:tx.*:doit=rmgroup
rmgroup:usenet@academ.com:tx.*:doit=rmgroup

## UCB (University of California Berkeley, USA)
# Contact: Chris van den Berg <usenet@agate.berkeley.edu>
# URL: http://www.net.berkeley.edu/usenet/
# Key URL: http://www.net.berkeley.edu/usenet/usenet.asc
# Key fingerprint: 96 B8 8E 9A 98 09 37 7D  0E EC 81 88 DB 90 29 BF
# *PGP*   See README.PGP.
newgroup:*:ucb.*:drop
rmgroup:*:ucb.*:drop
checkgroups:usenet@agate.berkeley.edu:ucb.*:verify-ucb.news
newgroup:usenet@agate.berkeley.edu:ucb.*:verify-ucb.news
rmgroup:usenet@agate.berkeley.edu:ucb.*:verify-ucb.news

## UCD (University of California Davis, USA)
checkgroups:usenet@mark.ucdavis.edu:ucd.*:doit=checkgroups
checkgroups:usenet@rocky.ucdavis.edu:ucd.*:doit=checkgroups
newgroup:usenet@mark.ucdavis.edu:ucd.*:doit=newgroup
newgroup:usenet@rocky.ucdavis.edu:ucd.*:doit=newgroup
rmgroup:usenet@mark.ucdavis.edu:ucd.*:doit=rmgroup
rmgroup:usenet@rocky.ucdavis.edu:ucd.*:doit=rmgroup

## UFRA (Unterfranken, Deutschland)
#
# This hierarchy is still in use, but it has no active maintainer.
# Control messages for this hierarchy should not be honored without
# confirming that the sender is the new hierarchy maintainer.
#
# Admin group: ufra.admin
# Key fingerprint: F7 AD 96 D8 7A 3F 7E 84  02 0C 83 9A DB 8F EB B8
# *PGP*   See README.PGP.
newgroup:*:ufra.*:drop
rmgroup:*:ufra.*:drop

## UIUC (*DEFUNCT* -- University of Illinois at Urbana-Champaign, USA)
# Contact: news@ks.uiuc.edu
# This hierarchy is defunct.  Please remove it.
newgroup:*:uiuc.*:log
rmgroup:*:uiuc.*:doit=rmgroup

## UK (United Kingdom of Great Britain and Northern Ireland)
# URL: http://www.usenet.org.uk/
# Admin group: uk.net.news.announce
# Key URL: http://www.usenet.org.uk/newsadmins.html
# *PGP*   See README.PGP.
newgroup:*:uk.*:drop
rmgroup:*:uk.*:drop
checkgroups:control@usenet.org.uk:uk.*:verify-uk.net.news.announce
newgroup:control@usenet.org.uk:uk.*:verify-uk.net.news.announce
rmgroup:control@usenet.org.uk:uk.*:verify-uk.net.news.announce

## UKR (Ukraine)
checkgroups:ay@sita.kiev.ua:ukr.*:doit=checkgroups
newgroup:ay@sita.kiev.ua:ukr.*:doit=newgroup
rmgroup:ay@sita.kiev.ua:ukr.*:doit=rmgroup

## ULM (*PRIVATE* -- Ulm, Germany)
# Admin group: ulm.misc
# Syncable server: news.in-ulm.de
# For private use only.
newgroup:*:ulm.*:log
rmgroup:*:ulm.*:doit=rmgroup

## UMICH (University of Michigan, USA)
checkgroups:*@*.umich.edu:umich.*:doit=checkgroups
newgroup:*@*.umich.edu:umich.*:doit=newgroup
rmgroup:*@*.umich.edu:umich.*:doit=rmgroup

## UMN (University of Minnesota, USA)
newgroup:edh@*.tc.umn.edu:umn.*:doit=newgroup
newgroup:news@*.tc.umn.edu:umn.*:doit=newgroup
newgroup:Michael.E.Hedman-1@umn.edu:umn.*:doit=newgroup
newgroup:edh@*.tc.umn.edu:umn*class.*:log
newgroup:news@*.tc.umn.edu:umn*class.*:log
newgroup:Michael.E.Hedman-1@umn.edu:umn*class.*:log
rmgroup:news@*.tc.umn.edu:umn.*:doit=rmgroup
rmgroup:edh@*.tc.umn.edu:umn.*:doit=rmgroup
rmgroup:Michael.E.Hedman-1@umn.edu:umn.*:doit=rmgroup

## UN (*HISTORIC* -- The United Nations)
#
# This hierarchy is not entirely defunct, but it receives very little
# traffic and is included primarily for the sake of completeness.
#
# Admin group: un.public.usenet.admin
# *PGP*   See README.PGP.
newgroup:*:un.*:drop
rmgroup:*:un.*:drop
checkgroups:news@news.itu.int:un.*:verify-ungroups@news.itu.int
newgroup:news@news.itu.int:un.*:verify-ungroups@news.itu.int
rmgroup:news@news.itu.int:un.*:verify-ungroups@news.itu.int

## UO (University of Oregon, Eugene, Oregon, USA)
checkgroups:newsadmin@news.uoregon.edu:uo.*:doit=checkgroups
newgroup:newsadmin@news.uoregon.edu:uo.*:doit=newgroup
rmgroup:newsadmin@news.uoregon.edu:uo.*:doit=rmgroup

## US (United States of America)
# Contact: us-control@lists.killfile.org
# Admin group: us.config
# Key fingerprint: BB96 EB2C CFD0 75C8 E9DE  C2C2 1DA2 9D87 B73C AF1B
# *PGP*   See README.PGP.
newgroup:*:us.*:drop
rmgroup:*:us.*:drop
checkgroups:us-control@lists.killfile.org:us.*:verify-us-control@lists.killfile.org
newgroup:us-control@lists.killfile.org:us.*:verify-us-control@lists.killfile.org
rmgroup:us-control@lists.killfile.org:us.*:verify-us-control@lists.killfile.org

## UT (*LOCAL* -- University of Toronto, Canada)
# URL: http://www.utoronto.ca/ns/utornews/
#newgroup:news@ecf.toronto.edu:ut.*:doit
#newgroup:news@ecf.toronto.edu:ut.class.*:mail
#rmgroup:news@ecf.toronto.edu:ut.*:doit

## UTA (Finnish)
checkgroups:news@news.cc.tut.fi:finet.*|uta.*:doit=checkgroups
newgroup:news@news.cc.tut.fi:uta.*:doit=newgroup
rmgroup:news@news.cc.tut.fi:uta.*:doit=rmgroup

## UTEXAS (*LOCAL* -- University of Texas, USA)
# URL: http://www.utexas.edu/its/usenet/index.php
newgroup:fletcher@cs.utexas.edu:utexas.*:doit=newgroup
newgroup:news@geraldo.cc.utexas.edu:utexas.*:doit=newgroup
newgroup:fletcher@cs.utexas.edu:utexas*class.*:log
newgroup:news@geraldo.cc.utexas.edu:utexas*class.*:log
rmgroup:fletcher@cs.utexas.edu:utexas.*:doit=rmgroup
rmgroup:news@geraldo.cc.utexas.edu:utexas.*:doit=rmgroup

## UTWENTE (*LOCAL* -- University of Twente, Netherlands)
# Contact: newsmaster@utwente.nl
# For local use only, contact the above address for information.
newgroup:*:utwente.*:log
rmgroup:*:utwente.*:doit=rmgroup

## UVA (*LOCAL* -- University of Virginia, USA)
# Contact: usenet@virginia.edu
# For local use only, contact the above address for information.
newgroup:*:uva.*:log
rmgroup:*:uva.*:doit=rmgroup

## UW (University of Waterloo, Canada)
# Admin group: uw.newsgroups
# Syncable server: news.uwaterloo.ca
# *PGP*   See README.PGP.
newgroup:*:uw.*:drop
rmgroup:*:uw.*:drop
checkgroups:newsgroups@news.uwaterloo.ca:uw.*:verify-uw.newsgroups
newgroup:newsgroups@news.uwaterloo.ca:uw.*:verify-uw.newsgroups
rmgroup:newsgroups@news.uwaterloo.ca:uw.*:verify-uw.newsgroups

## UWARWICK (*LOCAL* -- University of Warwick, UK)
# Contact: Jon Harley <news@csv.warwick.ac.uk>
# For local use only, contact the above address for information.
newgroup:*:uwarwick.*:log
rmgroup:*:uwarwick.*:doit=rmgroup

## UWO (University of Western Ontario, London, Canada)
# URL: http://www.uwo.ca/its/news/groups.uwo.html
checkgroups:reggers@julian.uwo.ca:uwo.*:doit=checkgroups
newgroup:reggers@julian.uwo.ca:uwo.*:doit=newgroup
rmgroup:reggers@julian.uwo.ca:uwo.*:doit=rmgroup

## VAN (Vancouver, British Columbia, Canada)
checkgroups:bc_van_usenet@fastmail.ca:bc.*|van.*:doit=checkgroups
newgroup:bc_van_usenet@fastmail.ca:van.*:doit=newgroup
rmgroup:bc_van_usenet@fastmail.ca:van.*:doit=rmgroup

## VEGAS (Las Vegas, Nevada, USA)
checkgroups:cshapiro@netcom.com:nv.*|vegas.*:doit=checkgroups
checkgroups:doctor@netcom.com:nv.*|vegas.*:doit=checkgroups
newgroup:cshapiro@netcom.com:vegas.*:doit=newgroup
newgroup:doctor@netcom.com:vegas.*:doit=newgroup
rmgroup:cshapiro@netcom.com:vegas.*:doit=rmgroup
rmgroup:doctor@netcom.com:vegas.*:doit=rmgroup

## VGC (Japan groups?)
checkgroups:news@isl.melco.co.jp:vgc.*:doit=checkgroups
newgroup:news@isl.melco.co.jp:vgc.*:doit=newgroup
rmgroup:news@isl.melco.co.jp:vgc.*:doit=rmgroup

## VMSNET (VMS Operating System)
checkgroups:cts@dragon.com:vmsnet.*:doit=checkgroups
newgroup:cts@dragon.com:vmsnet.*:doit=newgroup
rmgroup:cts@dragon.com:vmsnet.*:doit=rmgroup

## WA (Western Australia)
# Contact: ausadmin@aus.news-admin.org
# URL: http://wa.news-admin.org/
# Key URL: http://aus.news-admin.org/ausadmin.asc
# *PGP*   See README.PGP.
newgroup:*:wa.*:drop
rmgroup:*:wa.*:drop
checkgroups:ausadmin@aus.news-admin.org:aus.*|bne.*|canb.*|melb.*|syd.*|wa.*:verify-ausadmin@aus.news-admin.org
newgroup:ausadmin@aus.news-admin.org:wa.*:verify-ausadmin@aus.news-admin.org
rmgroup:ausadmin@aus.news-admin.org:wa.*:verify-ausadmin@aus.news-admin.org

## WADAI (Japanese ?)
checkgroups:kohe-t@*wakayama-u.ac.jp:wadai.*:doit=checkgroups
newgroup:kohe-t@*wakayama-u.ac.jp:wadai.*:doit=newgroup
rmgroup:kohe-t@*wakayama-u.ac.jp:wadai.*:doit=rmgroup

## WALES (Wales)
# Contact: committee@wales-usenet.org
# URL: http://www.wales-usenet.org/
# Admin group: wales.usenet.config
# Key URL: http://www.wales-usenet.org/english/newsadmin.txt
# Key fingerprint: 2D 9E DE DF 12 DA 34 5C  49 E1 EE 28 E3 AB 0D AD
# *PGP*   See README.PGP.
newgroup:*:wales.*:drop
rmgroup:*:wales.*:drop
checkgroups:control@wales-usenet.org:wales.*:verify-wales-usenet
newgroup:control@wales-usenet.org:wales.*:verify-wales-usenet
rmgroup:control@wales-usenet.org:wales.*:verify-wales-usenet

## WASH (Washington State, USA)
checkgroups:graham@ee.washington.edu:seattle.*|wash.*:doit=checkgroups
newgroup:graham@ee.washington.edu:wash.*:doit=newgroup
rmgroup:graham@ee.washington.edu:wash.*:doit=rmgroup

## WEST-VIRGINIA (West Virginia, USA)
checkgroups:bryan27@hgo.net:west-virginia.*:doit=checkgroups
newgroup:mark@bluefield.net:west-virginia.*:doit=newgroup
newgroup:bryan27@hgo.net:west-virginia.*:doit=newgroup
rmgroup:mark@bluefield.net:west-virginia.*:doit=rmgroup
rmgroup:bryan27@hgo.net:west-virginia.*:doit=rmgroup

## WORLDONLINE (*LOCAL* -- ?)
# Contact: newsmaster@worldonline.nl
# For local use only, contact the above address for information.
newgroup:*:worldonline.*:log
rmgroup:*:worldonline.*:doit=rmgroup

## WPG (Winnipeg, Manitoba, Canada)
#
# This hierarchy is still in use, but it has no active maintainer.
# Control messages for this hierarchy should not be honored without
# confirming that the sender is the new hierarchy maintainer.

## WPI (*LOCAL* -- Worcester Polytechnic Institute, Worcester, MA)
# For local use only.
newgroup:aej@*.wpi.edu:wpi.*:log
rmgroup:aej@*.wpi.edu:wpi.*:doit=rmgroup

## WU (Washington University at St. Louis, MO)
checkgroups:*@*.wustl.edu:wu.*:doit=checkgroups
newgroup:*@*.wustl.edu:wu.*:doit=newgroup
rmgroup:*@*.wustl.edu:wu.*:doit=rmgroup

## X-PRIVAT (Italian)
# Contact: dmitry@x-privat.org
# URL: http://www.x-privat.org/
# Admin group: x-privat.info
# Key URL: http://www.x-privat.org/dmitry.asc
# Key fingerprint: 9B 0A 7E 68 27 80 C7 96  47 6B 03 90 51 05 68 43
# *PGP*   See README.PGP.
newgroup:*:x-privat.*:drop
rmgroup:*:x-privat.*:drop
checkgroups:dmitry@x-privat.org:x-privat.*:verify-dmitry@x-privat.org
newgroup:dmitry@x-privat.org:x-privat.*:verify-dmitry@x-privat.org
rmgroup:dmitry@x-privat.org:x-privat.*:verify-dmitry@x-privat.org

## XS4ALL (XS4ALL, Netherlands)
# Contact: XS4ALL Newsmaster <news@xs4all.nl>
checkgroups:news@xs4all.nl:xs4all.*:doit=checkgroups
newgroup:news@xs4all.nl:xs4all.*:doit=newgroup
rmgroup:news@xs4all.nl:xs4all.*:doit=rmgroup

## YORK (*LOCAL* -- York University, Toronto, ON)
# Contact: Peter Marques <news@yorku.ca>
# For local use only, contact the above address for information.
newgroup:*:york.*:log
rmgroup:*:york.*:doit=rmgroup

## Z-NETZ (German non-Internet based network)
# Contact: teko@dinoex.sub.org
# Admin group: z-netz.koordination.user+sysops
# Key URL: ftp://ftp.dinoex.de/pub/keys/z-netz.koordination.user+sysops.asc
# *PGP*   See README.PGP.
newgroup:*:z-netz.*:drop
rmgroup:*:z-netz.*:drop
checkgroups:teko@dinoex.sub.org:z-netz.*:verify-z-netz.koordination.user+sysops
newgroup:teko@dinoex.sub.org:z-netz.*:verify-z-netz.koordination.user+sysops
rmgroup:teko@dinoex.sub.org:z-netz.*:verify-z-netz.koordination.user+sysops

## ZA (South Africa)
checkgroups:ccfj@hippo.ru.ac.za:za.*:doit=checkgroups
checkgroups:root@duvi.eskom.co.za:za.*:doit=checkgroups
newgroup:ccfj@hippo.ru.ac.za:za.*:doit=newgroup
newgroup:root@duvi.eskom.co.za:za.*:doit=newgroup
rmgroup:ccfj@hippo.ru.ac.za:za.*:doit=rmgroup
rmgroup:root@duvi.eskom.co.za:za.*:doit=rmgroup

## ZER (*DEFUNCT* -- Germany)
# This hierarchy is defunct.  Please remove it.
newgroup:*:zer.*:log
rmgroup:*:zer.*:doit=rmgroup
