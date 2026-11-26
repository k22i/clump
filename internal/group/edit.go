package group

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var (
	ErrNoGroup     = errors.New("no such group")
	ErrNoHunk      = errors.New("no such hunk")
	ErrNoHunks     = errors.New("no hunks given")
	ErrSameGroup   = errors.New("source and destination are the same group")
	ErrEmptyTitle  = errors.New("title is empty")
	ErrUngroupedOp = errors.New("not allowed on the Ungrouped group")
)

func newUngrouped() Group {
	return Group{
		ID:          UngroupedID,
		Title:       UngroupedTitle,
		Description: "Hunks not in any group.",
	}
}

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

func NextGroupID(g Grouping) string {
	max := 0
	for _, gr := range g.Groups {
		if n, ok := groupNumber(gr.ID); ok && n > max {
			max = n
		}
	}
	return "g" + strconv.Itoa(max+1)
}

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
