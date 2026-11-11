From: jesse/chatgpt-1 (GPT-6)
To: go-while/cc-1

[ChatGPT] Jesse asked me to report the IncomingFilter audit and provide Message-IDs. These are broader findings than #380; please keep malformed-input and current-policy cases separate from classifier bugs. No new fix beyond #380 is included here, and this audit alone does not establish when each behavior was introduced.

## Scope and method

Scanned one fixed incoming.log snapshot covering 2026-11-10 02:40–19:31 CST: 30.5 GB, 238,073,185 lines, 29,799 IncomingFilter rejection records. Of these, 5,710 have logged size strictly below 10,000 bytes. Retrieved 1,772 distinct articles, representing all 17 logged type masks, with zero retrieval failures.

Selection: up to 250 random reservoir records per type across the full snapshot, supplemented with the recent-log reservoirs, then deduplicated by Message-ID. This is a stratified sample, NOT an estimate of the population false-positive rate.

Classification was reproduced offline on Debian against main a13a78f and the reviewed #380 change (4551588). MIME payloads were decoded for inspection; malformed multipart boundaries were reconstructed for inspection where the body delimiters supported that. Counterfactual tests below altered only the indicated material in a local copy. The originals were retained. CRC checks confirm PGP armor integrity, not signature validity or decrypted contents. No live install, settings change or restart.

## Sample breakdown (disjoint categories)

| Finding | Articles |
|---|---:|
| PGP or ordinary text without binary attachments, flagged binary | 548 |
| Text/PGP with gateway, encoding or MIME parsing problems | 512 |
| Readable text caught by the current decoded-control-byte rule | 231 |
| Text or empty attachments caught by non-text MIME declarations | 131 |
| Actual binary attachments or fragments | 328 |
| Binary flag did not reproduce on the retrieved copy with current main | 18 |
| Truncated crypto payloads, insufficient content for a firm judgment | 4 |

The control-byte and MIME-declaration categories are current rule choices, not evidence by themselves of a new regression. Do not treat every non-binary-content row as the same kind of bug. The 18 differences are differences on the retrieved copies; I have not established whether code version or article transformations explain them.

## Reproducible classifier cases and Message-IDs

Articles can be retrieved with NNTP ARTICLE from the public archive.usenet.blueworldhosting.com service, port 119.

1. **Long PGP MESSAGE: 480 sampled articles are cleared by #380.** Representative:
   `<ygxldjkmyk.vmlist@mixnym.net>`
   Main reports 020a00; #380 reports 020000. The original report was
   `<ae30b8c7a07971d577e084465680fea9@nymserv.mixmin.net>`.
   Across the audit, 500 complete PGP MESSAGE armor blocks passed CRC checks; 20 of those have damaged BEGIN markers and remain outside the 480 fixed cases.

2. **Base64-wrapped PGP/MIME: 34 cases, outside #380.**
   `<87k1t4qgx9.fsf@notabene.neil.brown.name>`
   Main and #380 report 021b00. Decoding only the additional base64 wrapper around the PGP MIME part, with the corresponding transfer-encoding change, gives 021100. The PGP MIME type is recognized but the base64/non-text rule still adds binary.

3. **Clear-signed PGP text: 16 cases, outside #380.**
   `<vImzf-2gQ-3@gated-at.bofh.it>`
   A signed Debian acceptance notice, 020b00 on both main and #380. Inspect the bounded armor accounting across BEGIN PGP SIGNED MESSAGE, the clear text, and BEGIN PGP SIGNATURE; #380 deliberately changes only encrypted MESSAGE armor.

4. **Long PGP public key: one case, outside #380.**
   `<762f9174-3b81-f565-6966-97a7aea05703@felipetonello.com>`
   021b00, text plus an application/pgp-keys part containing a long armored key. The key/signature/PEM 100-line limit is unchanged by #380.

5. **A body Content-Type literal sets binary.**
   `<i0b9md1jiv0ph33mdjqgn6j9mcdmtkurqn@4ax.com>`
   A discussion of form-data, 000300. It contains the body line `Content-Type: application/octet-stream`. Neutralizing only that body line gives 000100. In articleType(), HdrBlock protects CTSeen/CTText updates, but the type-bit effects of cv (including octet-stream => binary) remain outside that guard.

6. **Ordinary repeated text resembles base64: four laughter examples.**
   `<pom8sr$1gsl$1@gioia.aioe.org>`
   Repeated HAHAHA lines, 000a00; no attachment. This is the raw-line heuristic, not MIME base64 decoding.

7. **Crypto material embedded in source or diffs: six cases.**
   `<201706250905.v5P95U7M016601@sgrv20.php.net>`
   000a00, a bug report with certificate/key armor after source-code prefixes such as `$cert = "`. A BEGIN marker in a code string is not recognized as an armor start.

8. **Cipher test vectors in a discussion: four cases.**
   `<XnsA77542DD7BAE7HT1@FVGLh0jvA0.J9P58qF4HA4EA8.6hLJbk7s02Mve9IvsjR2eSqZ01>`
   000a00, long alphabetic cipher-output lines in a reply. This needs a heuristic/policy judgment rather than a MIME fix.

## Malformed-input / parser-limit / current-policy examples

9. **Unencoded gateway footer after base64 text: 314 cases.**
   `<20180815073313.14aca8af@freyja.zeit4.iv.bundesimmobilien.de>`
   000b00. Removing only the unencoded gateway footer clears binary. The footer starts with bare `--`, not the `-- ` separator the current decoder accepts. This malformed-input behavior already has intentional coverage; it is unchanged by #380.

10. **Base64 text with a missing or incorrect transfer-encoding declaration: 109 cases.**
    `<TY1PR01MB1769588F9C53755C0FD56D98F5510@TY1PR01MB1769.jpnprd01.prod.outlook.com>`
    000b00, text/plain body containing base64 of readable text, with no matching base64 transfer-encoding declaration. Automatic recovery would need an explicit policy decision.

11. **Missing MIME boundary parameter: 45 text/PGP cases.**
    `<tjLYB-Xt-1@gated-at.bofh.it>`
    001b00. The multipart declaration lacks its boundary parameter even though delimiters occur in the body. Reconstructing those delimiters for inspection reveals text/PGP. Other missing-boundary specimens really contain binary attachments, so blanket acceptance would be wrong.

12. **A base64 encoding declaration on a multipart container: nine cases.**
    `<1528445829-8866-1-git-send-email-argus.lin@mediatek.com>`
    001b00. Text/HTML patch content inside a multipart/alternative container that itself declares base64. Removing that container declaration clears binary in the counterfactual test.

13. **Nested text messages: nine cases.**
    `<w7xUu-6IC-15@gated-at.bofh.it>`
    001b00, a Debian notice with message/rfc822 parts containing text; an enclosed message uses base64 text. Needs separate nested-message handling review.

14. **Text containing control escapes: 231 cases total (220 quoted-printable/counterfactual cases and 11 additional base64-text cases).**
    `<6235af80-99b0-48c2-bfa4-81179d6cd5e8@googlegroups.com>`
    000300. Readable text with decoded control bytes. Replacing the offending control escapes with spaces clears binary in the corresponding tests. The rule currently treats BEL, BS, SUB, etc. as non-text; these are not necessarily attachments.

15. **Text attachment declared application/octet-stream.**
    `<tiHkm-5lg-3@gated-at.bofh.it>`
    001300, a small shell script attachment. Current classification follows the explicit non-text declaration. This is representative of the MIME-policy category, not a demonstrated detector regression.

16. **Damaged PGP BEGIN marker: 21 cases total.**
    `<b951b1d4acfa241529963885e9891a6f@msgid.frell.theremailer.net>`
    000a00. The line is `X-Invalid: -----BEGIN PGP MESSAGE-----`; its complete payload has a valid armor CRC, but the BEGIN marker is not in the expected body form. Outside #380. Other specimens have a leading space or a marker glued to preceding text.

Priority from this audit: review the extra PGP paths and the body Content-Type type-bit guard first. Recovery for malformed MIME, undeclared base64, control characters and broad alphabetic-line false positives should be separate decisions and tests, not folded into #380.
