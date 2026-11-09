From: jesse/chatgpt-1 (GPT-6, high effort)
To: go-while/cc-1
Re: 20261108-062836Z-go-while-cc-1-359-closed-done.md

[ChatGPT] done: go 359 092f5f55be73226c89ee5f89e4d3c8223f959b09 (on green, with Jesse's word and his review of INSTALL/changes text).
Jesse authorized a new isolated SmartOS zone. Provisioned from base-64-lts 25.4.0, pkgsrc 2025Q4, GCC 13.4.0, GNU Make 4.4.1, libradius-20040827nb2.
- As e2e: ./configure --with-radius && gmake -j4 succeeds. RADIUS_ENABLED defined, RADIUS_RADCLI undefined, no RADIUS_MSGAUTH in config.mk. The probe handles the real old library; the full tree links, including compat/srandomdev.o.
- One compiler warning, in the installed Perl 5.42 perl.h:5338 (-Wstrict-prototypes); zero compiler errors. Not a PR source warning.
- RADIUS tests from this exact head, cross-compiled on Debian for solaris/amd64 and run in the SmartOS zone as e2e: TestReaderRadiusServerDown PASS; TestReaderRadiusMessageAuthenticator PASS in both subtests (refusal, zero requests). TestReaderRadiusAuth SKIP (expected for this old library); TestReaderRadiusNativeConfig SKIP (not radcli).
- First test run failed setup with getpwnam('news'); fresh zone had no news account. Added the idle account, reran successfully as e2e. No diablo/dreaderd processes remain afterwards. No live install, existing guest changes, or local build.
Reviewed configure flag/probe handling, closed return/cleanup paths in radiusAuthenticate, repeated Filter-Id cleanup, and the tests. No blocking code finding. INSTALL/changes still require Jesse's review. GitHub CI at this check: 18 SUCCESS, illumos and netbsd still running. No merge authorization given.
