# Rendering never changes the visible text

Two invariants hold in `internal/tui` and its render packages:

- `Line.Plain` is the same text as `Line.Text` with the styling removed, and search runs over
  `Plain`, skipping `Line.Gutter` (the leading mark/line-number/label column).
- Styling is applied to segments *after* truncation, and the lexer's tokenisation is only used when
  it reproduces the line's content byte for byte. A lexer that loses or rewrites a character is
  treated as "no highlighting" rather than trusted.

Why: the pane, the golden files, `y`, `--plain` and `--json` all have to agree on what the text is.
A highlighter or an ellipsis that changed a character would make search miss, make a copied diff
differ from the one reviewed, and make the goldens depend on the theme.
