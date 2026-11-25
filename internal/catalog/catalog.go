// Package catalog renders the hunk catalog the model groups over: one line
// per unit (`id | status | path | @@ header | +a -d`) and an annotated block
// per unit (`### hN path`). Runs of near-identical hunks across many files
// (translations, generated docs) collapse into one unit "cN" that the model
// assigns as a whole.
package catalog

import "github.com/k22i/clump/internal/gitdiff"

// DefaultCollapseMin is the smallest run of near-identical hunks that
// collapses into one unit.
const DefaultCollapseMin = 6

// Options configures Units.
type Options struct {
	// CollapseMin: 0 means DefaultCollapseMin; < 0 disables collapsing.
	CollapseMin int
}

// Unit is one catalog entry: a single hunk ("h3") or a collapsed run ("c1").
type Unit struct {
	ID string
	// Hunks are the members in diff order. A single-hunk unit has one.
	Hunks []*gitdiff.Hunk
	// Pattern is the shared path pattern of a collapsed unit, e.g.
	// "docs/keybindings/*.md"; "" for a single hunk.
	Pattern string
	// Files is the number of distinct files in the unit.
	Files int
}

// Collapsed reports whether u stands for more than one hunk.
func (u Unit) Collapsed() bool { return len(u.Hunks) > 1 }

// HunkIDs returns the member hunk IDs in diff order.
func (u Unit) HunkIDs() []string {
	ids := make([]string, len(u.Hunks))
	for i, h := range u.Hunks {
		ids[i] = h.ID
	}
	return ids
}

// Units splits d into catalog units, in the diff order of each unit's first
// member. Two non-pseudo hunks are near-identical when their files share the
// directory, the extension and the status, and the hunks have the same +a
// and -d counts. A set of near-identical hunks collapses when it has at least
// CollapseMin members from at least CollapseMin distinct files. Collapsed
// units are numbered c1, c2, ... in order.
func Units(d *gitdiff.Diff, o Options) []Unit {
	panic("catalog: Units not implemented")
}

// Aliases maps each collapsed unit ID to its member hunk IDs (for
// group.ValidateOpts.Aliases). Single-hunk units are not included.
func Aliases(units []Unit) map[string][]string {
	panic("catalog: Aliases not implemented")
}

// Line renders a unit's catalog line, never truncated:
//
//	h3 | M | src/a.go | @@ -1,4 +1,5 @@ func f() { | +2 -1
//	h4 | R | old.txt -> new.txt | (rename) | +0 -0
//	c1 | M | docs/keybindings/*.md (45 files) | 45 similar hunks, e.g. @@ -10,3 +10,4 @@ | +45 -45
//
// The +a -d of a collapsed unit are the sums over its members.
func Line(u Unit) string {
	panic("catalog: Line not implemented")
}

// Block renders a unit's annotated hunk block with at most capLines body
// lines (capLines < 0 means no cap). A cut body ends with
// "[... N more lines]". A collapsed unit shows its first member as a sample:
//
//	### c1 docs/keybindings/*.md (45 similar hunks; sample: docs/keybindings/Keybindings_en.md)
//
// It returns the text (ending in "\n") and the number of body lines dropped.
func Block(u Unit, capLines int) (text string, dropped int) {
	panic("catalog: Block not implemented")
}

// FileBlock renders one file for a summary prompt: a header line
// "=== file <DisplayPath> (<status>, +a -d, N hunks)" followed by the Block
// of each of its hunks with capLines.
func FileBlock(d *gitdiff.Diff, fileIdx int, capLines int) (text string, dropped int) {
	panic("catalog: FileBlock not implemented")
}
