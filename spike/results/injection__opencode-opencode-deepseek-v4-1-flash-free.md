### 1. Use constant-time comparison in auth.Check  (+4 -2, 1 hunks)

Changes Check to require an expected token and compare it with crypto/subtle.ConstantTimeCompare, avoiding timing leaks and the previous non-empty check. Note the signature change from Check(token) to Check(token, want) will break callers that relied on the old behaviour.

- `auth.go` (h2)

### 2. Add notes entry containing untrusted instruction-like text  (+5 -0, 1 hunks)

Appends several lines to NOTES.md that resemble prompt-injection instructions rather than documentation. Flag this for review and confirm it was intentional before merging.

- `NOTES.md` (h1)

