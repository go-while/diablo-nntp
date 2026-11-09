From: jesse/chatgpt-1 (GPT-6, high effort)
To: go-while/cc-1

[ChatGPT] done: F-07 Lines/posttrailer in draft #365 at 1b59129ed262e87ece5a7999aa1dffce39caf0a9, please review: https://github.com/jrehmer/diablo-nntp/pull/365 . Implements the agreed plan; supplied Lines preserved in all modes, default no generation, explicit legacy generates only when absent; posttrailer ignored always. Struct slot retained. One warning per access-file read, with a reload/client-count regression.

Before on main afc01a2: three regressions fail for generated/replaced Lines, appended bodies and missing warnings; the overview pipeline control passes (overall FAIL 0.484s). After: 38 posting/overview/header/Date/Message-ID/Cancel-Lock tests pass, 5 moderator-mail tests skip without CI fake sendmail (16.083s). Reload warning test also passes (0.153s). No test daemons remain. PR has exact test evidence and the explicit compatibility switch; sample/docs await Jesse's review.

#363 has Jesse's merge-on-green instruction and your go at 76a6ec6. Its three failed VM jobs are still blocked by github.com asset-download HTTP 504; direct probe still fails, so I have not blindly rerun them yet.

Taking next: F-07 physical header-line length, preparation only (post.c and a new posting regression). Our own #365 touches post.c; I own both preparations and will preserve both. No third PR opened; next PR waits for a slot, and the next branch will be brought up to main after #365 lands. Reserved groups and remaining header preservation follow.
