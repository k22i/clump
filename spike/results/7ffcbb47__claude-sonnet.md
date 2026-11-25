### 1. Add ASCII fast path to view write  (+17 -1, 2 hunks)

Introduces startsWithPrintableASCIICluster helper and uses it in viewBuffer.write to take printable ASCII characters directly, falling back to uniseg for everything else. This skips grapheme segmentation for most text, with the risk being any divergence from uniseg for edge cases like control characters or CRLF.

- `pkg/gocui/view.go` (h2 h1)

### 2. Test ASCII shortcut against uniseg  (+26 -0, 2 hunks)

Adds the utf8 import and a test that checks the shortcut agrees with uniseg for every single byte and every pair of ASCII bytes, plus a few explicit true and false cases.

- `pkg/gocui/view_test.go` (h3 h4)

