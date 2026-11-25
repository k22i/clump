### 1. Test the ASCII cluster shortcut against uniseg  (+26 -0, 2 hunks)

Add the unicode/utf8 import and a test that checks the shortcut agrees with uniseg for every pair of ASCII bytes, plus explicit positive and negative cases such as combining marks, tabs, CRLF and escape sequences.

- `pkg/gocui/view_test.go` (h3 h4)

### 2. Ungrouped  (+17 -1, 2 hunks)

Hunks the model did not assign.

- `pkg/gocui/view.go` (h1 h2)

> notice: repaired: 2 hunks missing from the model output; put in Ungrouped
