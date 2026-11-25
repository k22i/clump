package group

import (
	"errors"
	"testing"
)

func TestHistoryBasics(t *testing.T) {
	d, start := editTestDiff(6) // g1 [h1 h2 h3], g2 [h4 h5 h6]
	second := editWant("g1|A||h1 h2 h3 h4 h5 h6")
	third := editWant("g1|A||h1 h2 h3 h4 h5 h6", "g2|B||")

	t.Run("Do of an equal grouping is a no-op", func(t *testing.T) {
		h := NewHistory(start)
		if h.Do(start.Clone()) {
			t.Fatalf("Do(equal) = true, want false")
		}
		if undo, redo := h.Len(); undo != 0 || redo != 0 {
			t.Fatalf("Len() = (%d, %d), want (0, 0)", undo, redo)
		}
		editAssertEqual(t, d, h.Current(), start)
	})
	t.Run("Undo and Redo on an empty history", func(t *testing.T) {
		h := NewHistory(start)
		if h.Undo() {
			t.Fatalf("Undo() on an empty history = true")
		}
		if h.Redo() {
			t.Fatalf("Redo() on an empty history = true")
		}
		if h.CanUndo() || h.CanRedo() {
			t.Fatalf("CanUndo/CanRedo = true on an empty history")
		}
	})
	t.Run("Do Undo Redo move one step", func(t *testing.T) {
		h := NewHistory(start)
		if !h.Do(second) {
			t.Fatalf("Do(second) = false, want true")
		}
		if undo, redo := h.Len(); undo != 1 || redo != 0 {
			t.Fatalf("Len() = (%d, %d), want (1, 0)", undo, redo)
		}
		editAssertEqual(t, d, h.Current(), second)
		if !h.Undo() {
			t.Fatalf("Undo() = false, want true")
		}
		if undo, redo := h.Len(); undo != 0 || redo != 1 {
			t.Fatalf("Len() = (%d, %d), want (0, 1)", undo, redo)
		}
		editAssertEqual(t, d, h.Current(), start)
		if !h.Redo() {
			t.Fatalf("Redo() = false, want true")
		}
		editAssertEqual(t, d, h.Current(), second)
		if undo, redo := h.Len(); undo != 1 || redo != 0 {
			t.Fatalf("Len() = (%d, %d), want (1, 0)", undo, redo)
		}
	})
	t.Run("Do clears the redo stack", func(t *testing.T) {
		h := NewHistory(start)
		h.Do(second)
		if !h.Undo() {
			t.Fatalf("Undo() = false")
		}
		if !h.Do(third) {
			t.Fatalf("Do(third) = false")
		}
		if h.CanRedo() {
			t.Fatalf("CanRedo() = true after a new change")
		}
		if undo, redo := h.Len(); undo != 1 || redo != 0 {
			t.Fatalf("Len() = (%d, %d), want (1, 0)", undo, redo)
		}
		editAssertEqual(t, d, h.Current(), third)
	})
	t.Run("Current returns a copy", func(t *testing.T) {
		h := NewHistory(start)
		cur := h.Current()
		cur.Groups[0].Title = "mutated"
		cur.Groups[0].Hunks[0] = unknownNumber
		cur.Groups = append(cur.Groups, Group{ID: "g9", Title: "extra"})
		editAssertEqual(t, d, h.Current(), start)
	})
	t.Run("stored snapshots are deep copies", func(t *testing.T) {
		own := start.Clone()
		h := NewHistory(own)
		own.Groups[0].Title = "mutated"
		own.Groups[0].Hunks[0] = unknownNumber
		editAssertEqual(t, d, h.Current(), editWant("g1|F1||h1 h2 h3", "g2|F2||h4 h5 h6"))

		next := second.Clone()
		h.Do(next)
		next.Groups[0].Hunks[2] = unknownNumber
		editAssertEqual(t, d, h.Current(), second)
	})
	t.Run("Reset clears both stacks", func(t *testing.T) {
		h := NewHistory(start)
		h.Do(second)
		h.Do(third)
		h.Undo()
		reset := editWant("g1|R||h1 h2 h3", "g2|R||h4 h5 h6")
		h.Reset(reset)
		if undo, redo := h.Len(); undo != 0 || redo != 0 {
			t.Fatalf("Len() after Reset = (%d, %d), want (0, 0)", undo, redo)
		}
		if h.CanUndo() || h.CanRedo() {
			t.Fatalf("CanUndo/CanRedo = true after Reset")
		}
		editAssertEqual(t, d, h.Current(), reset)
		reset.Groups[0].Title = "mutated"
		editAssertEqual(t, d, h.Current(), editWant("g1|R||h1 h2 h3", "g2|R||h4 h5 h6"))
	})
	t.Run("Apply records a change", func(t *testing.T) {
		h := NewHistory(start)
		changed, err := h.Apply(func(g Grouping) (Grouping, error) { return Move(g, []string{"h4"}, "g1") })
		if err != nil {
			t.Fatalf("Apply error: %v", err)
		}
		if !changed {
			t.Fatalf("Apply reported no change")
		}
		editAssertEqual(t, d, h.Current(), editWant("g1|F1||h1 h2 h3 h4", "g2|F2||h5 h6"))
		if undo, redo := h.Len(); undo != 1 || redo != 0 {
			t.Fatalf("Len() = (%d, %d), want (1, 0)", undo, redo)
		}
	})
	t.Run("Apply of an error leaves the history unchanged", func(t *testing.T) {
		h := NewHistory(start)
		h.Do(second)
		h.Undo() // one redoable change, so a wrong Apply would show up in Len
		changed, err := h.Apply(func(g Grouping) (Grouping, error) { return Move(g, []string{"h99"}, "g1") })
		if !errors.Is(err, ErrNoHunk) {
			t.Fatalf("Apply error = %v, want ErrNoHunk", err)
		}
		if changed {
			t.Fatalf("Apply reported a change on error")
		}
		if undo, redo := h.Len(); undo != 0 || redo != 1 {
			t.Fatalf("Len() = (%d, %d), want (0, 1)", undo, redo)
		}
		editAssertEqual(t, d, h.Current(), start)
	})
	t.Run("Apply of an unchanged result pushes nothing", func(t *testing.T) {
		h := NewHistory(start)
		changed, err := h.Apply(func(g Grouping) (Grouping, error) { return Move(g, []string{"h1"}, "g1") })
		if err != nil {
			t.Fatalf("Apply error: %v", err)
		}
		if changed {
			t.Fatalf("Apply reported a change for a no-op Move")
		}
		if undo, redo := h.Len(); undo != 0 || redo != 0 {
			t.Fatalf("Len() = (%d, %d), want (0, 0)", undo, redo)
		}
	})
}
