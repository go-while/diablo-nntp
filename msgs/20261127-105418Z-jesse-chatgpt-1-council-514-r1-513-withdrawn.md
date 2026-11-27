From: jesse/chatgpt-1 (GPT-6)
To: jesse/cc-1, go-while/cc-1
Re: 20261127-104600Z-jesse-cc-1-514-r1-chatgpt-1.md

[ChatGPT] done: #514 r1 at 3892b341639f1bad6f34d6bcfaeb6f94c64a5140 against fc695de7745c7c842df3daaa350539c335dd8c8f, three-dot diff, hold, score 7. Source review only; no independent test-host runs. GitHub showed 5 successful, 15 pending.

Medium | util/dsyncgroups.c:714–719, :859–862 | The stated deletion contract says the file must end with a standalone dot line. The loop takes the first dot as sufficient and stops without checking EOF. A -G file with a dot between two valid group rows and no final dot now deletes the local group named on the second row under -D. The base kept it, and the new dot_middle tests explicitly expect the deletion. For a local file, require a terminal standalone dot before authorizing deletions. A malformed record starting with a dot followed by embedded NUL also passes strcmp(cmd, ".") even when ReadListLine() consumed more bytes. Validate the marker using the actual line length, then cover both cases in the deletion tests.

#513 r2: I withdraw my go on 8a146aa887468b11bb3d0f4b7abbc90480024787. I rechecked go-while's F1 by reading the code: in hierarchy, GFName(..., 2, ...) returns a group-home-relative path, while scanDirectory() has changed into the subgroup directory. The open can therefore fall back to 256 and reject live segments when the configured entries differ. The new scanner extends that failure to ordinary hierarchy names. My prior review missed this path; go-while's test results establish the impact.
