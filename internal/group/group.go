// Package group turns a parsed diff into a prompt, validates and repairs
// the model's answer, and holds the edit model (pure edit operations plus an
// undo/redo History) for groups the user fixes by hand.
//
// The one invariant everything here keeps is Check: every hunk of the diff
// is in exactly one group.
package group

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/k22i/clump/internal/gitdiff"
)

// PromptVersion is part of every cache key. Bump it when prompt_v1.txt,
// prompt_summary_v1.txt or Build's rendering changes.
const PromptVersion = "v1"

// DefaultMaxGroups is the group cap when none is configured.
const DefaultMaxGroups = 12

// UngroupedID is the reserved group ID of the "Ungrouped" group. It exists
// only while it holds at least one hunk.
const UngroupedID = "ungrouped"

// UngroupedTitle is the title of the Ungrouped group.
const UngroupedTitle = "Ungrouped"

// GeneratedID is the reserved group ID of the "Generated files" group: the
// hunks of files gitdiff.File.Generated marks, which are never sent to the
// model. It is placed last by Validate and FallbackPerFile; after that it is
// an ordinary group the user may edit.
const GeneratedID = "generated"

// GeneratedTitle is the title of the Generated files group.
const GeneratedTitle = "Generated files"

// generatedDesc is the Generated files group's description.
const generatedDesc = "Lock files, codegen output and vendored code. Not sent to the model."

// GeneratedHunks returns the IDs of the hunks of d's generated files, in
// diff order.
func GeneratedHunks(d *gitdiff.Diff) []string {
	var out []string
	for _, h := range d.Hunks {
		if IsGenerated(h) {
			out = append(out, h.ID)
		}
	}
	return out
}

// IsGenerated reports whether h belongs to a generated file.
func IsGenerated(h *gitdiff.Hunk) bool { return h.File != nil && h.File.Generated != "" }

// generatedGroup is the Generated files group for d, if d has any.
func generatedGroup(d *gitdiff.Diff) (Group, bool) {
	ids := GeneratedHunks(d)
	if len(ids) == 0 {
		return Group{}, false
	}
	return Group{ID: GeneratedID, Title: GeneratedTitle, Description: generatedDesc, Hunks: ids}, true
}

// Group is one logical change.
type Group struct {
	// ID is stable for the life of the grouping: "g1", "g2", ... or
	// UngroupedID. Edits never renumber existing groups.
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	// Hunks are hunk IDs ("h3"), sorted ascending by HunkNum. May be empty,
	// except in the Ungrouped group.
	Hunks []string `json:"hunks"`
}

// Grouping is an ordered list of groups: what the user sees and edits.
type Grouping struct {
	Groups []Group `json:"groups"`
}

// Result is a validated model answer (or a fallback).
type Result struct {
	Grouping
	// Notices are human-readable lines about repairs or the fallback.
	Notices []string `json:"notices,omitempty"`
	// Fallback is true when the model output was unusable and the grouping
	// is one group per file.
	Fallback bool `json:"fallback"`
}

// HunkNum returns N for a hunk ID "hN", or -1 if id is not of that form.
func HunkNum(id string) int {
	if len(id) < 2 || id[0] != 'h' {
		return -1
	}
	n, err := strconv.Atoi(id[1:])
	if err != nil || n < 1 || strconv.Itoa(n) != id[1:] {
		return -1
	}
	return n
}

// Clone returns a deep copy of g.
func (g Grouping) Clone() Grouping {
	out := Grouping{Groups: make([]Group, len(g.Groups))}
	for i, gr := range g.Groups {
		gr.Hunks = append([]string(nil), gr.Hunks...)
		out.Groups[i] = gr
	}
	if g.Groups == nil {
		out.Groups = nil
	}
	return out
}

// Equal reports whether a and b have the same groups in the same order with
// the same fields (a nil and an empty Hunks slice are equal).
func Equal(a, b Grouping) bool {
	if len(a.Groups) != len(b.Groups) {
		return false
	}
	for i := range a.Groups {
		x, y := a.Groups[i], b.Groups[i]
		if x.ID != y.ID || x.Title != y.Title || x.Description != y.Description || len(x.Hunks) != len(y.Hunks) {
			return false
		}
		for j := range x.Hunks {
			if x.Hunks[j] != y.Hunks[j] {
				return false
			}
		}
	}
	return true
}

// Find returns the index of the group with the given ID, or -1.
func (g Grouping) Find(id string) int {
	for i, gr := range g.Groups {
		if gr.ID == id {
			return i
		}
	}
	return -1
}

// GroupOf returns the index of the group holding hunkID, or -1.
func (g Grouping) GroupOf(hunkID string) int {
	for i, gr := range g.Groups {
		for _, h := range gr.Hunks {
			if h == hunkID {
				return i
			}
		}
	}
	return -1
}

// Check reports the first violation of the grouping invariants against d:
//
//  1. every group has a non-empty ID, and IDs are unique;
//  2. every group has a non-empty (after trimming) Title;
//  3. every hunk ID in d appears in exactly one group;
//  4. no group lists an ID that is not a hunk of d;
//  5. each group's Hunks are sorted strictly ascending by HunkNum;
//  6. the group with ID UngroupedID, if present, is not empty.
//
// It returns nil when g is clean.
func Check(d *gitdiff.Diff, g Grouping) error {
	known := make(map[string]bool, len(d.Hunks))
	for _, h := range d.Hunks {
		known[h.ID] = true
	}
	seenGroup := map[string]bool{}
	count := map[string]int{}
	for i, gr := range g.Groups {
		if gr.ID == "" {
			return fmt.Errorf("group %d has an empty ID", i)
		}
		if seenGroup[gr.ID] {
			return fmt.Errorf("duplicate group ID %q", gr.ID)
		}
		seenGroup[gr.ID] = true
		if strings.TrimSpace(gr.Title) == "" {
			return fmt.Errorf("group %q has an empty title", gr.ID)
		}
		if gr.ID == UngroupedID && len(gr.Hunks) == 0 {
			return fmt.Errorf("group %q is empty", UngroupedID)
		}
		prev := 0
		for _, id := range gr.Hunks {
			if !known[id] {
				return fmt.Errorf("group %q lists unknown hunk %q", gr.ID, id)
			}
			n := HunkNum(id)
			if n <= prev {
				return fmt.Errorf("group %q: hunks not sorted ascending at %q", gr.ID, id)
			}
			prev = n
			count[id]++
		}
	}
	for _, h := range d.Hunks {
		if c := count[h.ID]; c != 1 {
			return fmt.Errorf("hunk %s appears in %d groups", h.ID, c)
		}
	}
	return nil
}
