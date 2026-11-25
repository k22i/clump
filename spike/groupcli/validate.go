package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

type Group struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Hunks       []string `json:"hunks"`
}

type Result struct {
	Groups   []Group  `json:"groups"`
	Notices  []string `json:"notices,omitempty"`
	Fallback bool     `json:"fallback"`
}

// extractJSON finds the outermost {...} in model text (models sometimes add fences/prose).
func extractJSON(s string) (string, bool) {
	start := strings.Index(s, "{")
	if start < 0 {
		return "", false
	}
	depth, inStr, esc := 0, false, false
	for i := start; i < len(s); i++ {
		c := s[i]
		switch {
		case esc:
			esc = false
		case c == '\\' && inStr:
			esc = true
		case c == '"':
			inStr = !inStr
		case inStr:
		case c == '{':
			depth++
		case c == '}':
			depth--
			if depth == 0 {
				return s[start : i+1], true
			}
		}
	}
	return "", false
}

func FallbackPerFile(d *Diff, why string) Result {
	r := Result{Fallback: true, Notices: []string{"fallback: one group per file (" + why + ")"}}
	for _, f := range d.Files {
		g := Group{Title: f.Path(), Description: "Ungrouped: all changes to " + f.Path()}
		for _, h := range f.Hunks {
			g.Hunks = append(g.Hunks, h.ID)
		}
		r.Groups = append(r.Groups, g)
	}
	return r
}

// Validate parses model output and enforces: IDs exist, every hunk in exactly one
// group, group count capped. Repairs what it can; falls back to per-file otherwise.
func Validate(d *Diff, text string, maxGroups int) Result {
	js, ok := extractJSON(text)
	if !ok {
		return FallbackPerFile(d, "no JSON object in model output")
	}
	var raw struct {
		Groups []Group `json:"groups"`
	}
	if err := json.Unmarshal([]byte(js), &raw); err != nil {
		return FallbackPerFile(d, "invalid JSON: "+err.Error())
	}
	known := map[string]bool{}
	for _, h := range d.Hunks {
		known[h.ID] = true
	}
	r := Result{}
	seen := map[string]bool{}
	unknown, dup := 0, 0
	for _, g := range raw.Groups {
		ng := Group{Title: strings.TrimSpace(g.Title), Description: strings.TrimSpace(g.Description)}
		for _, id := range g.Hunks {
			id = strings.TrimSpace(id)
			switch {
			case !known[id]:
				unknown++
			case seen[id]:
				dup++
			default:
				seen[id] = true
				ng.Hunks = append(ng.Hunks, id)
			}
		}
		if len(ng.Hunks) > 0 {
			if ng.Title == "" {
				ng.Title = "(untitled group)"
			}
			r.Groups = append(r.Groups, ng)
		}
	}
	if len(r.Groups) == 0 {
		return FallbackPerFile(d, "model returned no usable groups")
	}
	if unknown > 0 {
		r.Notices = append(r.Notices, fmt.Sprintf("repaired: dropped %d unknown hunk IDs", unknown))
	}
	if dup > 0 {
		r.Notices = append(r.Notices, fmt.Sprintf("repaired: %d hunks were in two groups; kept the first", dup))
	}
	if len(r.Groups) > maxGroups {
		extra := r.Groups[maxGroups-1:]
		merged := Group{Title: "Other changes", Description: "Groups beyond the cap, merged."}
		for _, g := range extra {
			merged.Hunks = append(merged.Hunks, g.Hunks...)
		}
		r.Groups = append(r.Groups[:maxGroups-1], merged)
		r.Notices = append(r.Notices, fmt.Sprintf("repaired: %d groups exceeded the cap of %d; merged the tail", len(raw.Groups), maxGroups))
	}
	var missing []string
	for _, h := range d.Hunks {
		if !seen[h.ID] {
			missing = append(missing, h.ID)
		}
	}
	if len(missing) > 0 {
		r.Groups = append(r.Groups, Group{Title: "Ungrouped", Description: "Hunks the model did not assign.", Hunks: missing})
		r.Notices = append(r.Notices, fmt.Sprintf("repaired: %d hunks missing from the model output; put in Ungrouped", len(missing)))
	}
	return r
}

// Check is the invariant used by tests: every hunk exactly once.
func Check(d *Diff, r Result) error {
	count := map[string]int{}
	for _, g := range r.Groups {
		for _, id := range g.Hunks {
			count[id]++
		}
	}
	for _, h := range d.Hunks {
		if count[h.ID] != 1 {
			return fmt.Errorf("hunk %s appears %d times", h.ID, count[h.ID])
		}
		delete(count, h.ID)
	}
	if len(count) > 0 {
		return fmt.Errorf("unknown ids: %v", count)
	}
	return nil
}
