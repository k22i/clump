package group

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/k22i/clump/internal/gitdiff"
)

const PromptVersion = "v1"

const DefaultMaxGroups = 12

const UngroupedID = "ungrouped"

const UngroupedTitle = "Ungrouped"

const GeneratedID = "generated"

const GeneratedTitle = "Generated files"

const generatedDesc = "Lock files, codegen output and vendored code. Not sent to the model."

func GeneratedHunks(d *gitdiff.Diff) []string {
	var out []string
	for _, h := range d.Hunks {
		if IsGenerated(h) {
			out = append(out, h.ID)
		}
	}
	return out
}

func IsGenerated(h *gitdiff.Hunk) bool { return h.File != nil && h.File.Generated != "" }

func generatedGroup(d *gitdiff.Diff) (Group, bool) {
	ids := GeneratedHunks(d)
	if len(ids) == 0 {
		return Group{}, false
	}
	return Group{ID: GeneratedID, Title: GeneratedTitle, Description: generatedDesc, Hunks: ids}, true
}

type Group struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Hunks       []string `json:"hunks"`
}

type Grouping struct {
	Groups []Group `json:"groups"`
}

type Result struct {
	Grouping
	Notices  []string `json:"notices,omitempty"`
	Fallback bool     `json:"fallback"`
}

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

func (g Grouping) Find(id string) int {
	for i, gr := range g.Groups {
		if gr.ID == id {
			return i
		}
	}
	return -1
}

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
