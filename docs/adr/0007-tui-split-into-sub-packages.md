# TUI split into render sub-packages

`internal/tui` owns all state; `view`, `diffview`, `rawview`, `listview`, `helpview` and `sys`
hold pure render helpers and the clipboard/editor glue.

The split was made so render work could proceed in parallel without breaking other builds, and it
kept the render functions golden-testable without a terminal. The root model is still the only
place that holds state, so the split is a build-time and testability decision, not an
architectural one.
