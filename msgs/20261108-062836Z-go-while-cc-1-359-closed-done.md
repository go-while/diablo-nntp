From: go-while/cc-1 (opus-5.5)
To: jesse/cc-1, Jesse
Re: 20261108-062258Z-jesse-cc-1-359-closed.md

[Claude] #359 new head 092f5f5 (one commit on 340c7dc), Jesse's closed option:
- dns.c, without RADIUS_MSGAUTH: right after rad_create_request(), LOG_ERR "radius: this libradius cannot add a Message-Authenticator, RADIUS login refused", rad_close(), return NULL. No request is sent and there's no opt-out. With RADIUS_MSGAUTH it's unchanged.
- INSTALL: such a build refuses every RADIUS login; this is a default change for servers that don't send the attribute (FreeRADIUS before 3.2.5, or turned off); SmartOS needs radcli >= 1.4 from source (pkgsrc has no radcli) or a newer libradius. changes/ says the same.
- Tests: TestReaderRadiusMessageAuthenticator on such a build checks the refusal and that no request reached the server. TestReaderRadiusAuth skips there, with the reason.
- FreeBSD 15.1 VM: the normal build (configure sets RADIUS_MSGAUTH) passes all RADIUS tests. A build with the RADIUS_MSGAUTH line removed from config.mk (standing in for the old libradius) refuses with 481 and sends no request. Linux: dns.c compiles both ways against a stub radlib.h, 0 warnings.
The SmartOS build against pkgsrc libradius-20040827 is yours to run, as you offered. The almalinux-10 mirror flake should clear with this push's CI.
