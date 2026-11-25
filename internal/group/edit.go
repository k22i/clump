package group

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Errors returned by the edit operations. Each op returns its input
// unchanged together with one of these (possibly wrapped with detail).
var (
	ErrNoGroup     = errors.New("no such group")
	ErrNoHunk      = errors.New("no such hunk")
	ErrNoHunks     = errors.New("no hunks given")
	ErrSameGroup   = errors.New("source and destination are the same group")
	ErrEmptyTitle  = errors.New("title is empty")
	ErrUngroupedOp = errors.New("not allowed on the Ungrouped group")
)

// Edit operations are pure: they never modify their input (including the
// Hunks slices it shares) and return either a new Grouping that is
// Check-clean whenever the input was, or the input unchanged and an error.
// See DESIGN.md "Edit model" for the exact rules.

// newUngrouped returns the Ungrouped group with no hunks. Move and Delete
// append it at the end of the list when they need it and it is absent.
func newUngrouped() Group {
	return Group{
		ID:          UngroupedID,
		Title:       UngroupedTitle,
		Description: "Hunks not in any group.",
	}
}

// Move moves hunkIDs to the group toID, removing them from wherever they
// are. toID may be UngroupedID, which is created at the end if absent.
func Move(g Grouping, hunkIDs []string, toID string) (Grouping, error) {
	if len(hunkIDs) == 0 {
		return g, ErrNoHunks
	}
	for _, id := range hunkIDs {
		if g.GroupOf(id) < 0 {
			return g, fmt.Errorf("%w: %q", ErrNoHunk, id)
		}
	}
	if toID != UngroupedID && g.Find(toID) < 0 {
		return g, fmt.Errorf("%w: %q", ErrNoGroup, toID)
	}
	out := g.Clone()
	if toID == UngroupedID && out.Find(UngroupedID) < 0 {
		out.Groups = append(out.Groups, newUngrouped())
	}
	return moveHunks(out, hunkIDs, toID), nil
}

// moveHunks removes the hunks in ids from every group of out (which must be a
// deep copy the caller does not share) and appends them to the group with ID
// dstID, re-sorting it. Groups left empty stay, except UngroupedID, which is
// removed. Duplicates in ids are ignored. dstID must exist in out.
func moveHunks(out Grouping, ids []string, dstID string) Grouping {
	moving := make(map[string]bool, len(ids))
	add := make([]string, 0, len(ids))
	for _, id := range ids {
		if moving[id] {
			continue
		}
		moving[id] = true
		add = append(add, id)
	}
	for i := range out.Groups {
		var kept []string
		for _, h := range out.Groups[i].Hunks {
			if !moving[h] {
				kept = append(kept, h)
			}
		}
		out.Groups[i].Hunks = kept
	}
	dst := out.Find(dstID)
	out.Groups[dst].Hunks = append(out.Groups[dst].Hunks, add...)
	sortHunks(out.Groups[dst].Hunks)
	for i := range out.Groups {
		if out.Groups[i].ID == UngroupedID && len(out.Groups[i].Hunks) == 0 {
			out.Groups = append(out.Groups[:i], out.Groups[i+1:]...)
			break
		}
	}
	return out
}

// MoveToNew moves hunkIDs into a new group with the given title, appended at
// the end, and returns the new group's ID.
func MoveToNew(g Grouping, hunkIDs []string, title string) (Grouping, string, error) {
	trimmed := strings.TrimSpace(title)
	if trimmed == "" {
		return g, "", fmt.Errorf("%w: %q", ErrEmptyTitle, title)
	}
	if len(hunkIDs) == 0 {
		return g, "", ErrNoHunks
	}
	for _, id := range hunkIDs {
		if g.GroupOf(id) < 0 {
			return g, "", fmt.Errorf("%w: %q", ErrNoHunk, id)
		}
	}
	id := NextGroupID(g)
	out := g.Clone()
	out.Groups = append(out.Groups, Group{ID: id, Title: trimmed})
	return moveHunks(out, hunkIDs, id), id, nil
}

// NewGroup appends an empty group with the given title and returns its ID.
func NewGroup(g Grouping, title string) (Grouping, string, error) {
	trimmed := strings.TrimSpace(title)
	if trimmed == "" {
		return g, "", fmt.Errorf("%w: %q", ErrEmptyTitle, title)
	}
	id := NextGroupID(g)
	out := g.Clone()
	out.Groups = append(out.Groups, Group{ID: id, Title: trimmed})
	return out, id, nil
}

// Rename sets a group's title and description.
func Rename(g Grouping, id, title, description string) (Grouping, error) {
	i := g.Find(id)
	if i < 0 {
		return g, fmt.Errorf("%w: %q", ErrNoGroup, id)
	}
	if id == UngroupedID {
		return g, fmt.Errorf("%w: %q", ErrUngroupedOp, id)
	}
	trimmed := strings.TrimSpace(title)
	if trimmed == "" {
		return g, fmt.Errorf("%w: %q", ErrEmptyTitle, title)
	}
	out := g.Clone()
	out.Groups[i].Title = trimmed
	out.Groups[i].Description = strings.TrimSpace(description)
	return out, nil
}

// Delete removes a group; its hunks go to Ungrouped.
func Delete(g Grouping, id string) (Grouping, error) {
	i := g.Find(id)
	if i < 0 {
		return g, fmt.Errorf("%w: %q", ErrNoGroup, id)
	}
	if id == UngroupedID {
		return g, fmt.Errorf("%w: %q", ErrUngroupedOp, id)
	}
	out := g.Clone()
	moved := append([]string(nil), out.Groups[i].Hunks...)
	out.Groups = append(out.Groups[:i], out.Groups[i+1:]...)
	if len(moved) == 0 {
		return out, nil
	}
	if out.Find(UngroupedID) < 0 {
		out.Groups = append(out.Groups, newUngrouped())
	}
	return moveHunks(out, moved, UngroupedID), nil
}

// Reorder moves a group delta positions (negative is up), clamped to the
// list bounds.
func Reorder(g Grouping, id string, delta int) (Grouping, error) {
	i := g.Find(id)
	if i < 0 {
		return g, fmt.Errorf("%w: %q", ErrNoGroup, id)
	}
	out := g.Clone()
	j := i + delta
	if j < 0 {
		j = 0
	}
	if last := len(out.Groups) - 1; j > last {
		j = last
	}
	if j == i {
		return out, nil
	}
	gr := out.Groups[i]
	out.Groups = append(out.Groups[:i], out.Groups[i+1:]...)
	out.Groups = append(out.Groups, Group{})
	copy(out.Groups[j+1:], out.Groups[j:])
	out.Groups[j] = gr
	return out, nil
}

// Merge moves every hunk of srcID into dstID and removes srcID.
func Merge(g Grouping, srcID, dstID string) (Grouping, error) {
	si := g.Find(srcID)
	if si < 0 {
		return g, fmt.Errorf("%w: %q", ErrNoGroup, srcID)
	}
	if g.Find(dstID) < 0 {
		return g, fmt.Errorf("%w: %q", ErrNoGroup, dstID)
	}
	if srcID == dstID {
		return g, fmt.Errorf("%w: %q", ErrSameGroup, srcID)
	}
	out := g.Clone()
	moved := append([]string(nil), out.Groups[si].Hunks...)
	out.Groups = append(out.Groups[:si], out.Groups[si+1:]...)
	return moveHunks(out, moved, dstID), nil
}

// NextGroupID returns "g<N+1>" where N is the largest number among IDs of the
// form "g<digits>" in g (0 if none).
func NextGroupID(g Grouping) string {
	max := 0
	for _, gr := range g.Groups {
		if n, ok := groupNumber(gr.ID); ok && n > max {
			max = n
		}
	}
	return "g" + strconv.Itoa(max+1)
}

// groupNumber returns N for a group ID "gN" (digits, no leading zero), and
// whether id has that form. "g", "gx", "g01" and UngroupedID do not match.
func groupNumber(id string) (int, bool) {
	if len(id) < 2 || id[0] != 'g' {
		return 0, false
	}
	digits := id[1:]
	n, err := strconv.Atoi(digits)
	if err != nil || n < 1 || strconv.Itoa(n) != digits {
		return 0, false
	}
	return n, true
}
