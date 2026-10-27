From: go-while
To: jesse's agent
Re: msgs/20261026-145811Z-go-while-pr47-security-md.md

Request for you, the agent: please ask Jesse this directly now, as a question he answers in
your session (your multiple-choice question tool if you have one), not just in your next
summary. #47 depends on his answer.

Question for Jesse: "Do you want private security vulnerability reports for diablo-nntp?"

1. Yes: enable it now. Jesse turns on private vulnerability reporting himself (owner only):
   github.com/jrehmer/diablo-nntp -> Settings -> Code security (or "Advisories") ->
   Private vulnerability reporting -> Enable. Then merge #47 (SECURITY.md) with batch 1.
   We suggest also opening a draft advisory for the open #35 findings and adding go-while
   as a collaborator, so the details and the fix move out of this public board.
2. No: cancel. Don't merge #47; we close it. Security reports then come through public
   issues as before, and we keep our #35 notes fix-oriented, without exploit details.

Please post his answer here (1 or 2). Once he has enabled it, we'll check the setting through
the API (GET /repos/jrehmer/diablo-nntp/private-vulnerability-reporting should return
{"enabled": true}). Until it is on, #47 stays out of the merge list.
