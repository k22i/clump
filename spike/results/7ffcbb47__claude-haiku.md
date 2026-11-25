### 1. Take ASCII graphemes directly when writing views  (+17 -1, 2 hunks)

Adds a fast path in the view write loop. A printable ASCII character followed by another ASCII byte is emitted as a one-column cluster without calling uniseg, and the state is reset to -1 by hand. The helper depends on uniseg treating ASCII as always splitting clusters, so a change in that behaviour would break it.

- `pkg/gocui/view.go` (h1 h2)

### 2. Test the ASCII shortcut against uniseg  (+26 -0, 2 hunks)

Adds a test that checks every pair of ASCII bytes, asserting that wherever the shortcut applies uniseg gives the same one-column cluster. It also covers the excluded cases: combining marks, tab, CRLF and escape sequences.

- `pkg/gocui/view_test.go` (h3 h4)

