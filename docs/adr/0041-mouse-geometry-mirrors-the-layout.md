# Mouse geometry mirrors the render layout

`diffRect` and `listRect` recompute the layout that `renderBody` draws (header, the rule under it,
the list's width, padding) so a click maps to `m.scroll + row - diffRect.y`.

The alternative — recording rectangles while drawing — would make the model's event handling depend
on the last render having happened, which is false for a fresh model and awkward in tests.
`TestDiffRectMatchesTheLayout` pins the two together, so a layout change that forgets the geometry
fails a test rather than misfiring every click.

`MouseModeCellMotion` is used so clicks and wheel events arrive while the terminal keeps its own
text selection on shift.
