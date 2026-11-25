package group

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/k22i/clump/internal/gitdiff"
)

// ValidateOpts configures Validate.
type ValidateOpts struct {
	// MaxGroups caps the number of model groups; <= 0 means DefaultMaxGroups.
	MaxGroups int
	// Aliases maps a collapsed catalog unit ID ("c1") to the hunk IDs it
	// stands for. The model may assign a unit; Validate expands it.
	Aliases map[string][]string
}

// Limits the validator applies to model text. Model text reaches the terminal,
// so it must never carry escape sequences and must never be unbounded.
const (
	maxTitleRunes = 80
	maxDescRunes  = 1000

	untitledGroup = "(untitled group)"
	otherChanges  = "Other changes"
	missingDesc   = "Hunks the model did not assign."
	ellipsis      = "\u2026"
)

// Validate parses the model's text output and returns a Result that always
// passes Check(d, result.Grouping). It repairs what it can and otherwise
// falls back to one group per file. See DESIGN.md "Validation and repair".
func Validate(d *gitdiff.Diff, text string, o ValidateOpts) Result {
	max := o.MaxGroups
	if max <= 0 {
		max = DefaultMaxGroups
	}

	obj, ok := ExtractJSON(text)
	if !ok {
		return FallbackPerFile(d, "no JSON object in model output")
	}
	var raw struct {
		Groups []struct {
			Title       string            `json:"title"`
			Description string            `json:"description"`
			Hunks       []json.RawMessage `json:"hunks"`
		} `json:"groups"`
	}
	if err := json.Unmarshal([]byte(obj), &raw); err != nil {
		return FallbackPerFile(d, "invalid JSON: "+err.Error())
	}

	known := make(map[string]bool, len(d.Hunks))
	for _, h := range d.Hunks {
		known[h.ID] = true
	}

	var groups []Group
	seen := make(map[string]bool, len(d.Hunks))
	unknown, dup := 0, 0

	// place adds one hunk ID to g, counting the reasons it cannot be placed.
	place := func(g *Group, id string) {
		switch {
		case !known[id]:
			unknown++
		case seen[id]:
			dup++
		default:
			seen[id] = true
			g.Hunks = append(g.Hunks, id)
		}
	}

	for _, mg := range raw.Groups {
		g := Group{
			Title:       sanitiseTitle(mg.Title),
			Description: sanitiseDescription(mg.Description),
		}
		for _, rawID := range mg.Hunks {
			// Only JSON strings are hunk IDs; numbers, null, objects and
			// arrays count as unknown.
			s := strings.TrimSpace(string(rawID))
			if s == "" || s[0] != '"' {
				unknown++
				continue
			}
			var id string
			if err := json.Unmarshal([]byte(s), &id); err != nil {
				// Unreachable for valid JSON (the raw message already came
				// from the decoder and starts with a quote), but counting the
				// element as unknown is the safe answer if it ever happens.
				unknown++
				continue
			}
			id = strings.TrimSpace(id)
			if members, isAlias := o.Aliases[id]; isAlias {
				for _, m := range members {
					place(&g, strings.TrimSpace(m))
				}
				continue
			}
			place(&g, id)
		}
		if len(g.Hunks) > 0 {
			groups = append(groups, g)
		}
	}
	if len(groups) == 0 {
		return FallbackPerFile(d, "model returned no usable groups")
	}

	var notices []string
	if unknown > 0 {
		notices = append(notices, fmt.Sprintf("repaired: dropped %d unknown hunk IDs", unknown))
	}
	if dup > 0 {
		notices = append(notices, fmt.Sprintf("repaired: %d hunks were in two groups; kept the first", dup))
	}

	if n := len(groups); n > max {
		merged := Group{
			Title:       otherChanges,
			Description: fmt.Sprintf("Groups beyond the cap of %d, merged.", max),
		}
		for _, g := range groups[max-1:] {
			merged.Hunks = append(merged.Hunks, g.Hunks...)
		}
		kept := append([]Group(nil), groups[:max-1]...)
		groups = append(kept, merged)
		notices = append(notices, fmt.Sprintf("repaired: %d groups exceeded the cap of %d; merged the tail", n, max))
	}

	var missing []string
	for _, h := range d.Hunks {
		if !seen[h.ID] {
			missing = append(missing, h.ID)
		}
	}
	if len(missing) > 0 {
		groups = append(groups, Group{
			ID:          UngroupedID,
			Title:       UngroupedTitle,
			Description: missingDesc,
			Hunks:       missing,
		})
		notices = append(notices, fmt.Sprintf("repaired: %d hunks missing from the model output; put in Ungrouped", len(missing)))
	}

	k := 0
	for i := range groups {
		if groups[i].ID != UngroupedID {
			k++
			groups[i].ID = fmt.Sprintf("g%d", k)
		}
		sortHunks(groups[i].Hunks)
	}

	return Result{Grouping: Grouping{Groups: groups}, Notices: notices}
}

// FallbackPerFile returns one group per file of d (files with their hunks in
// diff order), with Fallback set and the notice
// "fallback: one group per file (<why>)".
func FallbackPerFile(d *gitdiff.Diff, why string) Result {
	r := Result{
		Fallback: true,
		Notices:  []string{"fallback: one group per file (" + why + ")"},
	}
	for _, f := range d.Files {
		if len(f.Hunks) == 0 {
			continue
		}
		g := Group{
			ID:          fmt.Sprintf("g%d", len(r.Groups)+1),
			Title:       f.DisplayPath(),
			Description: "All changes to " + f.DisplayPath() + ".",
		}
		for _, h := range f.Hunks {
			g.Hunks = append(g.Hunks, h.ID)
		}
		r.Groups = append(r.Groups, g)
	}
	return r
}

// ExtractJSON returns the first balanced top-level {...} object in s,
// skipping braces inside JSON strings. Models sometimes wrap JSON in prose or
// code fences. ok is false if there is no balanced object.
func ExtractJSON(s string) (obj string, ok bool) {
	start := strings.Index(s, "{")
	if start < 0 {
		return "", false
	}
	depth, inStr, esc := 0, false, false
	for i := start; i < len(s); i++ {
		c := s[i]
		switch {
		case esc:
			// The escaped byte is consumed; only strings can be escaped.
			esc = false
		case c == '\\' && inStr:
			esc = true
		case c == '"':
			inStr = !inStr
		case inStr:
			// Braces inside a string do not change depth.
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

// sortHunks sorts hunk IDs ascending by their number.
func sortHunks(ids []string) {
	sort.SliceStable(ids, func(i, j int) bool {
		a, b := HunkNum(ids[i]), HunkNum(ids[j])
		if a != b {
			return a < b
		}
		// Only reachable for IDs HunkNum does not understand; a total order
		// keeps the sort deterministic.
		return ids[i] < ids[j]
	})
}

// sanitiseTitle makes a model title safe to print: control runes become
// spaces, whitespace runs collapse, the result is cut to maxTitleRunes and an
// empty title becomes untitledGroup.
func sanitiseTitle(s string) string {
	t := collapseSpaces(s)
	if t == "" {
		return untitledGroup
	}
	return cutRunes(t, maxTitleRunes)
}

// sanitiseDescription sanitises each line like a title (keeping newlines),
// trims the result and cuts it to maxDescRunes.
func sanitiseDescription(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = collapseSpaces(l)
	}
	return cutRunes(strings.TrimSpace(strings.Join(lines, "\n")), maxDescRunes)
}

// collapseSpaces replaces control runes with spaces and collapses whitespace
// runs, trimming both ends.
func collapseSpaces(s string) string {
	return strings.Join(strings.Fields(cleanControls(s)), " ")
}

// cleanControls replaces every control rune of s with a space.
func cleanControls(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
}

// cutRunes shortens s to at most n runes, replacing the last rune with an
// ellipsis when it has to cut.
func cutRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + ellipsis
}
