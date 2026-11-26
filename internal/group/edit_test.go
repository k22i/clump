package group

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/k22i/clump/internal/gitdiff"
)

func editTestDiff(n int) (*gitdiff.Diff, Grouping) {
	d := &gitdiff.Diff{}
	g := Grouping{}
	files := (n + 2) / 3
	for fi := 0; fi < files; fi++ {
		path := fmt.Sprintf("f%d.go", fi+1)
		f := &gitdiff.File{OldPath: path, NewPath: path, Status: gitdiff.Modified}
		d.Files = append(d.Files, f)
		gr := Group{ID: fmt.Sprintf("g%d", fi+1), Title: fmt.Sprintf("F%d", fi+1)}
		for k := 0; k < 3; k++ {
			idx := fi*3 + k
			if idx >= n {
				break
			}
			id := fmt.Sprintf("h%d", idx+1)
			h := &gitdiff.Hunk{
				ID:       id,
				Index:    len(d.Hunks),
				FileIdx:  fi,
				File:     f,
				Header:   fmt.Sprintf("@@ -%d +%d @@", idx+1, idx+1),
				OldStart: idx + 1,
				OldLines: 1,
				NewStart: idx + 1,
				NewLines: 1,
				Lines:    []string{"-old", "+new"},
				Add:      1,
				Del:      1,
			}
			f.Hunks = append(f.Hunks, h)
			d.Hunks = append(d.Hunks, h)
			gr.Hunks = append(gr.Hunks, id)
		}
		g.Groups = append(g.Groups, gr)
	}
	return d, g
}

func editWant(specs ...string) Grouping {
	g := Grouping{}
	for _, s := range specs {
		parts := strings.SplitN(s, "|", 4)
		for len(parts) < 4 {
			parts = append(parts, "")
		}
		gr := Group{ID: parts[0], Title: parts[1], Description: parts[2]}
		if ids := strings.Fields(parts[3]); len(ids) > 0 {
			gr.Hunks = ids
		}
		g.Groups = append(g.Groups, gr)
	}
	return g
}

func editDiffFor(g Grouping) *gitdiff.Diff {
	d := &gitdiff.Diff{}
	for _, gr := range g.Groups {
		for _, id := range gr.Hunks {
			if d.Hunk(id) != nil {
				continue
			}
			n := HunkNum(id)
			path := fmt.Sprintf("f%s.go", id[1:])
			f := &gitdiff.File{OldPath: path, NewPath: path, Status: gitdiff.Modified}
			h := &gitdiff.Hunk{
				ID:       id,
				Index:    len(d.Hunks),
				FileIdx:  len(d.Files),
				File:     f,
				Header:   fmt.Sprintf("@@ -%d +%d @@", n, n),
				OldStart: n,
				OldLines: 1,
				NewStart: n,
				NewLines: 1,
				Lines:    []string{"-old", "+new"},
				Add:      1,
				Del:      1,
			}
			f.Hunks = append(f.Hunks, h)
			d.Files = append(d.Files, f)
			d.Hunks = append(d.Hunks, h)
		}
	}
	return d
}

const editUngrouped = "ungrouped|Ungrouped|Hunks not in any group.|"

func editShow(g Grouping) string {
	if len(g.Groups) == 0 {
		return "  (<no groups>)\n"
	}
	var b strings.Builder
	for _, gr := range g.Groups {
		fmt.Fprintf(&b, "  %s | %q | %q | %v\n", gr.ID, gr.Title, gr.Description, gr.Hunks)
	}
	return b.String()
}

func editAssertEqual(t *testing.T, d *gitdiff.Diff, got, want Grouping) {
	t.Helper()
	if !Equal(got, want) {
		t.Fatalf("grouping mismatch\ngot:\n%swant:\n%s", editShow(got), editShow(want))
	}
	if d != nil {
		if err := Check(d, got); err != nil {
			t.Fatalf("Check on result = %v\ngot:\n%s", err, editShow(got))
		}
	}
}

func TestNextGroupID(t *testing.T) {
	cases := []struct {
		name string
		ids  []string
		want string
	}{
		{"none", nil, "g1"},
		{"g1 g5 ungrouped", []string{"g1", "g5", "ungrouped"}, "g6"},
		{"g01 gx g ignored", []string{"g01", "gx", "g"}, "g1"},
		{"g9 g10", []string{"g9", "g10"}, "g11"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := Grouping{}
			for _, id := range c.ids {
				g.Groups = append(g.Groups, Group{ID: id, Title: "T"})
			}
			if got := NextGroupID(g); got != c.want {
				t.Fatalf("NextGroupID(%v) = %q, want %q", c.ids, got, c.want)
			}
		})
	}
}

func TestMove(t *testing.T) {
	cases := []struct {
		name  string
		in    Grouping
		hunks []string
		to    string
		want  Grouping
	}{
		{
			name:  "normal move sorts the target",
			in:    editWant("g1|A||h1 h2", "g2|B||h3 h4"),
			hunks: []string{"h2"},
			to:    "g2",
			want:  editWant("g1|A||h1", "g2|B||h2 h3 h4"),
		},
		{
			name:  "move to ungrouped creates it at the end",
			in:    editWant("g1|A||h1 h2"),
			hunks: []string{"h2"},
			to:    UngroupedID,
			want: editWant("g1|A||h1",
				editUngrouped+"h2"),
		},
		{
			name:  "moving the last hunk out of ungrouped removes it",
			in:    editWant("g1|A||h1", editUngrouped+"h2"),
			hunks: []string{"h2"},
			to:    "g1",
			want:  editWant("g1|A||h1 h2"),
		},
		{
			name:  "duplicates in the list are ignored",
			in:    editWant("g1|A||h1 h2 h3", "g2|B||"),
			hunks: []string{"h2", "h3", "h2", "h3"},
			to:    "g2",
			want:  editWant("g1|A||h1", "g2|B||h2 h3"),
		},
		{
			name:  "already in the target is a no-op",
			in:    editWant("g1|A||h1 h2", "g2|B||h3"),
			hunks: []string{"h1", "h2"},
			to:    "g1",
			want:  editWant("g1|A||h1 h2", "g2|B||h3"),
		},
		{
			name:  "unsorted hunk list still lands sorted",
			in:    editWant("g1|A||h1", "g2|B||h4 h5"),
			hunks: []string{"h5", "h1"},
			to:    "g2",
			want:  editWant("g1|A||", "g2|B||h1 h4 h5"),
		},
		{
			name:  "emptied source group stays",
			in:    editWant("g1|A||h1 h2", "g2|B||h3"),
			hunks: []string{"h1", "h2"},
			to:    "g2",
			want:  editWant("g1|A||", "g2|B||h1 h2 h3"),
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := editDiffFor(c.in)
			got, err := Move(c.in, c.hunks, c.to)
			if err != nil {
				t.Fatalf("Move(%s, %v, %q) error: %v", editShow(c.in), c.hunks, c.to, err)
			}
			editAssertEqual(t, d, got, c.want)
		})
	}
}

func TestMoveErrors(t *testing.T) {
	d, base := editTestDiff(3)
	cases := []struct {
		name  string
		hunks []string
		to    string
		want  error
	}{
		{"no hunks", nil, "g1", ErrNoHunks},
		{"unknown hunk", []string{"h1", "h99"}, "g1", ErrNoHunk},
		{"unknown target", []string{"h1"}, "gZZ", ErrNoGroup},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			before := base.Clone()
			got, err := Move(base, c.hunks, c.to)
			if !errors.Is(err, c.want) {
				t.Fatalf("Move(..., %v, %q) error = %v, want %v", c.hunks, c.to, err, c.want)
			}
			editAssertEqual(t, d, got, before)
			editAssertEqual(t, d, base, before)
		})
	}
}

func TestMoveToNew(t *testing.T) {
	t.Run("appends a new group with the trimmed title", func(t *testing.T) {
		d, base := editTestDiff(6)
		got, id, err := MoveToNew(base, []string{"h5", "h4"}, "  Docs  ")
		if err != nil {
			t.Fatalf("MoveToNew error: %v", err)
		}
		if id != "g3" {
			t.Fatalf("id = %q, want %q", id, "g3")
		}
		editAssertEqual(t, d, got, editWant(
			"g1|F1||h1 h2 h3",
			"g2|F2||h6",
			"g3|Docs||h4 h5",
		))
	})
	t.Run("emptied source stays and ungrouped is removed", func(t *testing.T) {
		in := editWant("g1|A||h1", editUngrouped+"h2 h3")
		d := editDiffFor(in)
		got, id, err := MoveToNew(in, []string{"h2", "h3"}, "Moved")
		if err != nil {
			t.Fatalf("MoveToNew error: %v", err)
		}
		if id != "g2" {
			t.Fatalf("id = %q, want %q", id, "g2")
		}
		editAssertEqual(t, d, got, editWant("g1|A||h1", "g2|Moved||h2 h3"))
	})
	t.Run("blank title", func(t *testing.T) {
		d, base := editTestDiff(3)
		before := base.Clone()
		got, id, err := MoveToNew(base, []string{"h1"}, "   ")
		if !errors.Is(err, ErrEmptyTitle) {
			t.Fatalf("error = %v, want ErrEmptyTitle", err)
		}
		if id != "" {
			t.Fatalf("id = %q, want \"\"", id)
		}
		editAssertEqual(t, d, got, before)
	})
	t.Run("no hunks", func(t *testing.T) {
		d, base := editTestDiff(3)
		before := base.Clone()
		got, id, err := MoveToNew(base, nil, "New")
		if !errors.Is(err, ErrNoHunks) {
			t.Fatalf("error = %v, want ErrNoHunks", err)
		}
		if id != "" {
			t.Fatalf("id = %q, want \"\"", id)
		}
		editAssertEqual(t, d, got, before)
	})
	t.Run("unknown hunk", func(t *testing.T) {
		d, base := editTestDiff(3)
		before := base.Clone()
		got, id, err := MoveToNew(base, []string{"h99"}, "New")
		if !errors.Is(err, ErrNoHunk) {
			t.Fatalf("error = %v, want ErrNoHunk", err)
		}
		if id != "" {
			t.Fatalf("id = %q, want \"\"", id)
		}
		editAssertEqual(t, d, got, before)
	})
}

func TestNewGroup(t *testing.T) {
	t.Run("appends an empty group", func(t *testing.T) {
		d, base := editTestDiff(3)
		got, id, err := NewGroup(base, " Docs ")
		if err != nil {
			t.Fatalf("NewGroup error: %v", err)
		}
		if id != "g2" {
			t.Fatalf("id = %q, want %q", id, "g2")
		}
		editAssertEqual(t, d, got, editWant("g1|F1||h1 h2 h3", "g2|Docs||"))
	})
	t.Run("blank title", func(t *testing.T) {
		d, base := editTestDiff(3)
		before := base.Clone()
		got, id, err := NewGroup(base, "")
		if !errors.Is(err, ErrEmptyTitle) {
			t.Fatalf("error = %v, want ErrEmptyTitle", err)
		}
		if id != "" {
			t.Fatalf("id = %q, want \"\"", id)
		}
		editAssertEqual(t, d, got, before)
	})
}

func TestRename(t *testing.T) {
	d, _ := editTestDiff(3)
	base := editWant("g1|A|old|h1 h2 h3")

	t.Run("trims title and description", func(t *testing.T) {
		got, err := Rename(base, "g1", "  Tests  ", "  covers x  ")
		if err != nil {
			t.Fatalf("Rename error: %v", err)
		}
		editAssertEqual(t, d, got, editWant("g1|Tests|covers x|h1 h2 h3"))
	})
	t.Run("clears the description", func(t *testing.T) {
		got, err := Rename(base, "g1", "B", "   ")
		if err != nil {
			t.Fatalf("Rename error: %v", err)
		}
		editAssertEqual(t, d, got, editWant("g1|B||h1 h2 h3"))
	})
	t.Run("unknown group", func(t *testing.T) {
		got, err := Rename(base, "gZZ", "B", "")
		if !errors.Is(err, ErrNoGroup) {
			t.Fatalf("error = %v, want ErrNoGroup", err)
		}
		editAssertEqual(t, d, got, base)
	})
	t.Run("ungrouped", func(t *testing.T) {
		in := editWant("g1|A||h1", editUngrouped+"h2 h3")
		got, err := Rename(in, UngroupedID, "B", "")
		if !errors.Is(err, ErrUngroupedOp) {
			t.Fatalf("error = %v, want ErrUngroupedOp", err)
		}
		editAssertEqual(t, d, got, in)
	})
	t.Run("empty title", func(t *testing.T) {
		got, err := Rename(base, "g1", "  \t ", "desc")
		if !errors.Is(err, ErrEmptyTitle) {
			t.Fatalf("error = %v, want ErrEmptyTitle", err)
		}
		editAssertEqual(t, d, got, base)
	})
}

func TestDelete(t *testing.T) {
	t.Run("hunks join an existing ungrouped group", func(t *testing.T) {
		in := editWant(
			"g1|A||h1 h2",
			"g2|B||h4",
			editUngrouped+"h5",
		)
		d := editDiffFor(in)
		got, err := Delete(in, "g1")
		if err != nil {
			t.Fatalf("Delete error: %v", err)
		}
		editAssertEqual(t, d, got, editWant(
			"g2|B||h4",
			editUngrouped+"h1 h2 h5",
		))
	})
	t.Run("creates ungrouped when absent", func(t *testing.T) {
		d, base := editTestDiff(6)
		got, err := Delete(base, "g2")
		if err != nil {
			t.Fatalf("Delete error: %v", err)
		}
		editAssertEqual(t, d, got, editWant(
			"g1|F1||h1 h2 h3",
			editUngrouped+"h4 h5 h6",
		))
	})
	t.Run("empty group deleted without creating ungrouped", func(t *testing.T) {
		in := editWant("g1|A||", "g2|B||h1")
		d := editDiffFor(in)
		got, err := Delete(in, "g1")
		if err != nil {
			t.Fatalf("Delete error: %v", err)
		}
		editAssertEqual(t, d, got, editWant("g2|B||h1"))
	})
	t.Run("unknown group", func(t *testing.T) {
		in := editWant("g1|A||h1")
		d := editDiffFor(in)
		got, err := Delete(in, "gZZ")
		if !errors.Is(err, ErrNoGroup) {
			t.Fatalf("error = %v, want ErrNoGroup", err)
		}
		editAssertEqual(t, d, got, in)
	})
	t.Run("ungrouped", func(t *testing.T) {
		in := editWant("g1|A||h1", editUngrouped+"h2")
		d := editDiffFor(in)
		got, err := Delete(in, UngroupedID)
		if !errors.Is(err, ErrUngroupedOp) {
			t.Fatalf("error = %v, want ErrUngroupedOp", err)
		}
		editAssertEqual(t, d, got, in)
	})
}

func TestReorder(t *testing.T) {
	d, base := editTestDiff(9)
	cases := []struct {
		name  string
		id    string
		delta int
		want  []string
	}{
		{"down one", "g1", 1, []string{"g2", "g1", "g3"}},
		{"up one", "g2", -1, []string{"g2", "g1", "g3"}},
		{"clamped at the top", "g3", -5, []string{"g3", "g1", "g2"}},
		{"clamped at the bottom", "g1", 5, []string{"g2", "g3", "g1"}},
		{"zero is a no-op", "g2", 0, []string{"g1", "g2", "g3"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Reorder(base, c.id, c.delta)
			if err != nil {
				t.Fatalf("Reorder(%q, %d) error: %v", c.id, c.delta, err)
			}
			specs := map[string]string{
				"g1": "g1|F1||h1 h2 h3",
				"g2": "g2|F2||h4 h5 h6",
				"g3": "g3|F3||h7 h8 h9",
			}
			want := editWant(specs[c.want[0]], specs[c.want[1]], specs[c.want[2]])
			editAssertEqual(t, d, got, want)
		})
	}
	t.Run("unknown group", func(t *testing.T) {
		got, err := Reorder(base, "gZZ", 1)
		if !errors.Is(err, ErrNoGroup) {
			t.Fatalf("error = %v, want ErrNoGroup", err)
		}
		editAssertEqual(t, d, got, base)
	})
}

func TestMerge(t *testing.T) {
	d, base := editTestDiff(9)

	t.Run("dst keeps its title and position", func(t *testing.T) {
		got, err := Merge(base, "g1", "g2")
		if err != nil {
			t.Fatalf("Merge error: %v", err)
		}
		editAssertEqual(t, d, got, editWant(
			"g2|F2||h1 h2 h3 h4 h5 h6",
			"g3|F3||h7 h8 h9",
		))
	})
	t.Run("later source leaves the dst position alone", func(t *testing.T) {
		got, err := Merge(base, "g3", "g2")
		if err != nil {
			t.Fatalf("Merge error: %v", err)
		}
		editAssertEqual(t, d, got, editWant(
			"g1|F1||h1 h2 h3",
			"g2|F2||h4 h5 h6 h7 h8 h9",
		))
	})
	t.Run("ungrouped is merged away", func(t *testing.T) {
		in := editWant("g1|A||h1", editUngrouped+"h2 h3")
		d := editDiffFor(in)
		got, err := Merge(in, UngroupedID, "g1")
		if err != nil {
			t.Fatalf("Merge error: %v", err)
		}
		editAssertEqual(t, d, got, editWant("g1|A||h1 h2 h3"))
	})
	t.Run("merge into ungrouped", func(t *testing.T) {
		in := editWant("g1|A||h1", editUngrouped+"h3")
		d := editDiffFor(in)
		got, err := Merge(in, "g1", UngroupedID)
		if err != nil {
			t.Fatalf("Merge error: %v", err)
		}
		editAssertEqual(t, d, got, editWant(editUngrouped+"h1 h3"))
	})
	t.Run("same group", func(t *testing.T) {
		got, err := Merge(base, "g1", "g1")
		if !errors.Is(err, ErrSameGroup) {
			t.Fatalf("error = %v, want ErrSameGroup", err)
		}
		editAssertEqual(t, d, got, base)
	})
	t.Run("unknown source", func(t *testing.T) {
		got, err := Merge(base, "gZZ", "g1")
		if !errors.Is(err, ErrNoGroup) {
			t.Fatalf("error = %v, want ErrNoGroup", err)
		}
		editAssertEqual(t, d, got, base)
	})
	t.Run("unknown destination", func(t *testing.T) {
		got, err := Merge(base, "g1", "gZZ")
		if !errors.Is(err, ErrNoGroup) {
			t.Fatalf("error = %v, want ErrNoGroup", err)
		}
		editAssertEqual(t, d, got, base)
	})
}

func TestOpsDoNotMutate(t *testing.T) {
	base := editWant(
		"g1|A||h1 h2",
		"g2|B||h3",
		editUngrouped+"h4",
	)
	cases := []struct {
		name string
		in   Grouping
		op   func(Grouping) (Grouping, error)
	}{
		{name: "Move between groups", op: func(g Grouping) (Grouping, error) {
			return Move(g, []string{"h2"}, "g2")
		}},
		{
			name: "Move to absent ungrouped",
			in:   editWant("g1|A||h1 h2"),
			op:   func(g Grouping) (Grouping, error) { return Move(g, []string{"h1"}, UngroupedID) },
		},
		{name: "Move out of ungrouped", op: func(g Grouping) (Grouping, error) {
			return Move(g, []string{"h4"}, "g1")
		}},
		{name: "Move no hunks", op: func(g Grouping) (Grouping, error) {
			return Move(g, nil, "g1")
		}},
		{name: "Move unknown hunk", op: func(g Grouping) (Grouping, error) {
			return Move(g, []string{unknownNumber}, "g1")
		}},
		{name: "Move unknown target", op: func(g Grouping) (Grouping, error) {
			return Move(g, []string{"h1"}, "gZZ")
		}},
		{name: "MoveToNew", op: func(g Grouping) (Grouping, error) {
			out, _, err := MoveToNew(g, []string{"h3", "h4"}, "New")
			return out, err
		}},
		{name: "MoveToNew blank title", op: func(g Grouping) (Grouping, error) {
			out, _, err := MoveToNew(g, []string{"h3"}, " ")
			return out, err
		}},
		{name: "NewGroup", op: func(g Grouping) (Grouping, error) {
			out, _, err := NewGroup(g, "New")
			return out, err
		}},
		{name: "NewGroup blank title", op: func(g Grouping) (Grouping, error) {
			out, _, err := NewGroup(g, "")
			return out, err
		}},
		{name: "Rename", op: func(g Grouping) (Grouping, error) { return Rename(g, "g1", "X", "Y") }},
		{name: "Rename ungrouped", op: func(g Grouping) (Grouping, error) {
			return Rename(g, UngroupedID, "X", "Y")
		}},
		{name: "Rename unknown", op: func(g Grouping) (Grouping, error) {
			return Rename(g, "gZZ", "X", "Y")
		}},
		{name: "Delete", op: func(g Grouping) (Grouping, error) { return Delete(g, "g1") }},
		{name: "Delete ungrouped", op: func(g Grouping) (Grouping, error) {
			return Delete(g, UngroupedID)
		}},
		{name: "Delete unknown", op: func(g Grouping) (Grouping, error) { return Delete(g, "gZZ") }},
		{name: "Reorder", op: func(g Grouping) (Grouping, error) { return Reorder(g, "g1", 1) }},
		{name: "Reorder unknown", op: func(g Grouping) (Grouping, error) {
			return Reorder(g, "gZZ", 1)
		}},
		{name: "Merge", op: func(g Grouping) (Grouping, error) { return Merge(g, "g1", "g2") }},
		{name: "Merge into ungrouped", op: func(g Grouping) (Grouping, error) {
			return Merge(g, "g2", UngroupedID)
		}},
		{name: "Merge same", op: func(g Grouping) (Grouping, error) { return Merge(g, "g1", "g1") }},
		{name: "Merge unknown", op: func(g Grouping) (Grouping, error) {
			return Merge(g, "g1", "gZZ")
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := base.Clone()
			if c.in.Groups != nil {
				in = c.in.Clone()
			}
			d := editDiffFor(in)
			before := in.Clone()
			out, err := c.op(in)
			if !Equal(in, before) {
				t.Fatalf("op mutated its input\nafter:\n%swant:\n%s", editShow(in), editShow(before))
			}
			if err == nil {
				if cerr := Check(d, out); cerr != nil {
					t.Fatalf("Check on result = %v\nresult:\n%s", cerr, editShow(out))
				}
				for i := range out.Groups {
					out.Groups[i].Title = "mutated"
					out.Groups[i].Description = "mutated"
					if len(out.Groups[i].Hunks) > 0 {
						out.Groups[i].Hunks[0] = "hZZ"
					}
				}
				out.Groups[0].ID = "gZZ"
				if !Equal(in, before) {
					t.Fatalf("result shares memory with the input\nafter:\n%swant:\n%s",
						editShow(in), editShow(before))
				}
			}
		})
	}
}

func TestExampleSequence(t *testing.T) {
	d, _ := editTestDiff(5)
	const ungroupedSpec = editUngrouped + "h2 h3 h4"

	states := []struct {
		action string
		want   Grouping
	}{
		{"start", editWant("g1|A||h1 h2", "g2|B||h3", "g3|C||h4 h5")},
		{"1 Move([h2 h4], g2)", editWant("g1|A||h1", "g2|B||h2 h3 h4", "g3|C||h5")},
		{"2 NewGroup(Docs)", editWant("g1|A||h1", "g2|B||h2 h3 h4", "g3|C||h5",
			"g4|Docs||")},
		{"3 Move([h5], g4)", editWant("g1|A||h1", "g2|B||h2 h3 h4", "g3|C||",
			"g4|Docs||h5")},
		{"4 Delete(g2)", editWant("g1|A||h1", "g3|C||", "g4|Docs||h5", ungroupedSpec)},
		{"5 Move([h2 h3 h4], g1)", editWant("g1|A||h1 h2 h3 h4", "g3|C||",
			"g4|Docs||h5")},
		{"6 Reorder(g4, -5)", editWant("g4|Docs||h5", "g1|A||h1 h2 h3 h4", "g3|C||")},
		{"7 Merge(g1, g4)", editWant("g4|Docs||h1 h2 h3 h4 h5", "g3|C||")},
		{"8 Rename(g3)", editWant("g4|Docs||h1 h2 h3 h4 h5", "g3|Tests|covers x|")},
		{"9 MoveToNew([h1], First)", editWant("g4|Docs||h2 h3 h4 h5",
			"g3|Tests|covers x|", "g5|First||h1")},
	}

	start := states[0].want
	editAssertEqual(t, d, start, editWant("g1|A||h1 h2", "g2|B||h3", "g3|C||h4 h5"))

	h := NewHistory(start)
	editAssertEqual(t, d, h.Current(), states[0].want)

	steps := []struct {
		action string
		op     func(Grouping) (Grouping, error)
	}{
		{states[1].action, func(g Grouping) (Grouping, error) { return Move(g, []string{"h2", "h4"}, "g2") }},
		{states[2].action, func(g Grouping) (Grouping, error) {
			out, _, err := NewGroup(g, "Docs")
			return out, err
		}},
		{states[3].action, func(g Grouping) (Grouping, error) { return Move(g, []string{"h5"}, "g4") }},
		{states[4].action, func(g Grouping) (Grouping, error) { return Delete(g, "g2") }},
		{states[5].action, func(g Grouping) (Grouping, error) {
			return Move(g, []string{"h2", "h3", "h4"}, "g1")
		}},
		{states[6].action, func(g Grouping) (Grouping, error) { return Reorder(g, "g4", -5) }},
		{states[7].action, func(g Grouping) (Grouping, error) { return Merge(g, "g1", "g4") }},
		{states[8].action, func(g Grouping) (Grouping, error) {
			return Rename(g, "g3", " Tests ", " covers x ")
		}},
		{states[9].action, func(g Grouping) (Grouping, error) {
			out, _, err := MoveToNew(g, []string{"h1"}, "First")
			return out, err
		}},
	}

	for i, s := range steps {
		changed, err := h.Apply(s.op)
		if err != nil {
			t.Fatalf("%s: error: %v", s.action, err)
		}
		if !changed {
			t.Fatalf("%s: Apply reported no change", s.action)
		}
		want := states[i+1].want
		editAssertEqual(t, d, h.Current(), want)
		if undo, redo := h.Len(); undo != i+1 || redo != 0 {
			t.Fatalf("%s: Len() = (%d, %d), want (%d, 0)", s.action, undo, redo, i+1)
		}
	}

	for i := 9; i >= 1; i-- {
		if !h.Undo() {
			t.Fatalf("Undo %d returned false", 10-i)
		}
		editAssertEqual(t, d, h.Current(), states[i-1].want)
	}
	if h.CanUndo() {
		t.Fatalf("CanUndo() = true after undoing everything")
	}
	if undo, redo := h.Len(); undo != 0 || redo != 9 {
		t.Fatalf("Len() = (%d, %d), want (0, 9)", undo, redo)
	}

	for i := 1; i <= 9; i++ {
		if !h.Redo() {
			t.Fatalf("Redo %d returned false", i)
		}
		editAssertEqual(t, d, h.Current(), states[i].want)
	}
	if h.CanRedo() {
		t.Fatalf("CanRedo() = true after redoing everything")
	}

	if !h.Undo() {
		t.Fatalf("Undo returned false")
	}
	editAssertEqual(t, d, h.Current(), states[8].want)
	changed, err := h.Apply(func(g Grouping) (Grouping, error) { return Move(g, []string{"h1"}, "g3") })
	if err != nil {
		t.Fatalf("Move after undo: %v", err)
	}
	if !changed {
		t.Fatalf("Move after undo reported no change")
	}
	if h.CanRedo() {
		t.Fatalf("CanRedo() = true after a new change")
	}
	editAssertEqual(t, d, h.Current(), editWant("g4|Docs||h2 h3 h4 h5",
		"g3|Tests|covers x|h1"))
}

const unknownNumber = "h99"

func editRandomHunks(rng *rand.Rand, d *gitdiff.Diff) []string {
	n := 1 + rng.IntN(3)
	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		if rng.IntN(8) == 0 {
			ids = append(ids, unknownNumber)
			continue
		}
		ids = append(ids, d.Hunks[rng.IntN(len(d.Hunks))].ID)
	}
	return ids
}

func editRandomTarget(rng *rand.Rand, g Grouping) string {
	switch rng.IntN(6) {
	case 0:
		return UngroupedID
	case 1:
		return "gZZ"
	}
	if len(g.Groups) == 0 {
		return UngroupedID
	}
	return g.Groups[rng.IntN(len(g.Groups))].ID
}

func editRandomPair(rng *rand.Rand, g Grouping) (string, string) {
	if len(g.Groups) == 0 {
		return "gZZ", UngroupedID
	}
	src := g.Groups[rng.IntN(len(g.Groups))].ID
	switch rng.IntN(3) {
	case 0:
		return src, src
	case 1:
		return src, "gZZ"
	}
	return src, g.Groups[rng.IntN(len(g.Groups))].ID
}

func editRandomTitle(rng *rand.Rand) string {
	switch rng.IntN(6) {
	case 0:
		return ""
	case 1:
		return "   "
	case 2:
		return "  T " + strconv.Itoa(rng.IntN(10)) + "  "
	}
	return "Title" + strconv.Itoa(rng.IntN(10))
}

func editLast(s []string, n int) []string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

func editSeed(t *testing.T) uint64 {
	t.Helper()
	if s := os.Getenv("CLUMP_SEED"); s != "" {
		seed, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			t.Fatalf("CLUMP_SEED=%q: %v", s, err)
		}
		return seed
	}
	return uint64(time.Now().UnixNano())
}

func TestEditPropertyRandom(t *testing.T) {
	const steps = 5000
	seed := editSeed(t)
	t.Logf("seed=%d (rerun with CLUMP_SEED=%d)", seed, seed)
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))

	d, start := editTestDiff(12)
	if err := Check(d, start); err != nil {
		t.Fatalf("start grouping is not clean: %v\n%s", err, editShow(start))
	}
	h := NewHistory(start)

	myPast := []Grouping{}
	myFuture := []Grouping{}
	myCur := start.Clone()

	var step int
	var actions []string
	var changes, errs, undos, redos, noops int
	failf := func(format string, args ...any) {
		t.Helper()
		t.Fatalf("seed=%d step=%d: %s\nlast actions:\n%s",
			seed, step, fmt.Sprintf(format, args...), strings.Join(editLast(actions, 10), "\n"))
	}

	for step = 0; step < steps; step++ {
		before := h.Current()
		var (
			action  string
			changed bool
			err     error
			undo    bool
			redo    bool
		)
		switch rng.IntN(9) {
		case 0:
			ids := editRandomHunks(rng, d)
			to := editRandomTarget(rng, before)
			action = fmt.Sprintf("Move(%v, %q)", ids, to)
			changed, err = h.Apply(func(g Grouping) (Grouping, error) { return Move(g, ids, to) })
		case 1:
			ids := editRandomHunks(rng, d)
			title := editRandomTitle(rng)
			action = fmt.Sprintf("MoveToNew(%v, %q)", ids, title)
			changed, err = h.Apply(func(g Grouping) (Grouping, error) {
				out, _, e := MoveToNew(g, ids, title)
				return out, e
			})
		case 2:
			title := editRandomTitle(rng)
			action = fmt.Sprintf("NewGroup(%q)", title)
			changed, err = h.Apply(func(g Grouping) (Grouping, error) {
				out, _, e := NewGroup(g, title)
				return out, e
			})
		case 3:
			id := editRandomTarget(rng, before)
			if rng.IntN(2) == 0 {
				id = UngroupedID
			}
			title := editRandomTitle(rng)
			desc := editRandomTitle(rng)
			action = fmt.Sprintf("Rename(%q, %q, %q)", id, title, desc)
			changed, err = h.Apply(func(g Grouping) (Grouping, error) {
				return Rename(g, id, title, desc)
			})
		case 4:
			id := editRandomTarget(rng, before)
			action = fmt.Sprintf("Delete(%q)", id)
			changed, err = h.Apply(func(g Grouping) (Grouping, error) { return Delete(g, id) })
		case 5:
			id := editRandomTarget(rng, before)
			delta := rng.IntN(9) - 4
			action = fmt.Sprintf("Reorder(%q, %d)", id, delta)
			changed, err = h.Apply(func(g Grouping) (Grouping, error) { return Reorder(g, id, delta) })
		case 6:
			src, dst := editRandomPair(rng, before)
			action = fmt.Sprintf("Merge(%q, %q)", src, dst)
			changed, err = h.Apply(func(g Grouping) (Grouping, error) { return Merge(g, src, dst) })
		case 7:
			action = "Undo"
			undo = h.Undo()
		case 8:
			action = "Redo"
			redo = h.Redo()
		}
		actions = append(actions, action)

		switch {
		case undo:
			undos++
			if len(myPast) == 0 {
				failf("%s returned true with an empty model past", action)
			}
			myFuture = append(myFuture, myCur)
			myCur = myPast[len(myPast)-1]
			myPast = myPast[:len(myPast)-1]
			if Equal(h.Current(), before) {
				failf("%s did not change the grouping", action)
			}
		case redo:
			redos++
			if len(myFuture) == 0 {
				failf("%s returned true with an empty model future", action)
			}
			myPast = append(myPast, myCur)
			myCur = myFuture[len(myFuture)-1]
			myFuture = myFuture[:len(myFuture)-1]
			if Equal(h.Current(), before) {
				failf("%s did not change the grouping", action)
			}
		case err != nil:
			errs++
			if !isEditError(err) {
				failf("%s returned an unexpected error: %v", action, err)
			}
			if !Equal(h.Current(), before) {
				failf("%s failed but changed the grouping\nafter:\n%swant:\n%s",
					action, editShow(h.Current()), editShow(before))
			}
			if gotU, gotR := h.Len(); gotU != len(myPast) || gotR != len(myFuture) {
				failf("%s failed but Len() = (%d, %d), model (%d, %d)",
					action, gotU, gotR, len(myPast), len(myFuture))
			}
		case changed:
			changes++
			myPast = append(myPast, before)
			myCur = h.Current().Clone()
			myFuture = nil
		default:
			noops++
			if !Equal(h.Current(), before) {
				failf("%s reported no change but changed the grouping\nafter:\n%swant:\n%s",
					action, editShow(h.Current()), editShow(before))
			}
		}

		cur := h.Current()
		if cerr := Check(d, cur); cerr != nil {
			failf("%s broke the invariants: %v\nstate:\n%s", action, cerr, editShow(cur))
		}
		if !Equal(cur, myCur) {
			failf("%s: Current() diverged from the model\ngot:\n%swant:\n%s",
				action, editShow(cur), editShow(myCur))
		}
		if gotU, gotR := h.Len(); gotU != len(myPast) || gotR != len(myFuture) {
			failf("%s: Len() = (%d, %d), model (%d, %d)",
				action, gotU, gotR, len(myPast), len(myFuture))
		}
		if canUndo, want := h.CanUndo(), len(myPast) > 0; canUndo != want {
			failf("%s: CanUndo() = %v, want %v", action, canUndo, want)
		}
		if canRedo, want := h.CanRedo(), len(myFuture) > 0; canRedo != want {
			failf("%s: CanRedo() = %v, want %v", action, canRedo, want)
		}
	}

	t.Logf("ran %d steps: %d changes, %d errors, %d no-ops, %d undos, %d redos; final stacks (%d, %d)",
		steps, changes, errs, noops, undos, redos, len(myPast), len(myFuture))
	if changes < steps/10 || undos < steps/20 || redos < steps/250 || errs == 0 || noops == 0 {
		t.Fatalf("the random walk did not exercise the interesting paths: changes=%d errors=%d no-ops=%d undos=%d redos=%d",
			changes, errs, noops, undos, redos)
	}
}

func isEditError(err error) bool {
	for _, want := range []error{ErrNoGroup, ErrNoHunk, ErrNoHunks, ErrSameGroup,
		ErrEmptyTitle, ErrUngroupedOp} {
		if errors.Is(err, want) {
			return true
		}
	}
	return false
}
