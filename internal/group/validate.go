package group

import "github.com/k22i/clump/internal/gitdiff"

// ValidateOpts configures Validate.
type ValidateOpts struct {
	// MaxGroups caps the number of model groups; <= 0 means DefaultMaxGroups.
	MaxGroups int
	// Aliases maps a collapsed catalog unit ID ("c1") to the hunk IDs it
	// stands for. The model may assign a unit; Validate expands it.
	Aliases map[string][]string
}

// Validate parses the model's text output and returns a Result that always
// passes Check(d, result.Grouping). It repairs what it can and otherwise
// falls back to one group per file. See DESIGN.md "Validation and repair".
func Validate(d *gitdiff.Diff, text string, o ValidateOpts) Result {
	panic("group: Validate not implemented")
}

// FallbackPerFile returns one group per file of d (files with their hunks in
// diff order), with Fallback set and the notice
// "fallback: one group per file (<why>)".
func FallbackPerFile(d *gitdiff.Diff, why string) Result {
	panic("group: FallbackPerFile not implemented")
}

// ExtractJSON returns the first balanced top-level {...} object in s,
// skipping braces inside JSON strings. Models sometimes wrap JSON in prose or
// code fences. ok is false if there is no balanced object.
func ExtractJSON(s string) (obj string, ok bool) {
	panic("group: ExtractJSON not implemented")
}
