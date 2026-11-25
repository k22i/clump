package group

// History is an unlimited, snapshot-based undo/redo stack over Groupings.
// The zero value is not usable; call NewHistory. Not safe for concurrent use.
type History struct {
	past   []Grouping
	cur    Grouping
	future []Grouping
}

// NewHistory starts a history at initial (deep-copied), with nothing to undo.
func NewHistory(initial Grouping) *History {
	return &History{cur: initial.Clone()}
}

// Current returns a deep copy of the current grouping.
func (h *History) Current() Grouping {
	return h.cur.Clone()
}

// Do makes next current, pushing the old current onto the undo stack and
// clearing the redo stack. If next is Equal to the current grouping it does
// nothing and returns false.
func (h *History) Do(next Grouping) bool {
	if Equal(next, h.cur) {
		return false
	}
	h.past = append(h.past, h.cur)
	h.cur = next.Clone()
	h.future = nil
	return true
}

// Apply runs op on the current grouping and, if it succeeds, Does the
// result. It returns op's error (the history is then unchanged) and whether
// the grouping changed.
func (h *History) Apply(op func(Grouping) (Grouping, error)) (changed bool, err error) {
	next, err := op(h.Current())
	if err != nil {
		return false, err
	}
	return h.Do(next), nil
}

// Undo steps back one change; false if there is nothing to undo.
func (h *History) Undo() bool {
	if len(h.past) == 0 {
		return false
	}
	h.future = append(h.future, h.cur)
	h.cur = h.past[len(h.past)-1]
	h.past = h.past[:len(h.past)-1]
	return true
}

// Redo steps forward one undone change; false if there is nothing to redo.
func (h *History) Redo() bool {
	if len(h.future) == 0 {
		return false
	}
	h.past = append(h.past, h.cur)
	h.cur = h.future[len(h.future)-1]
	h.future = h.future[:len(h.future)-1]
	return true
}

// CanUndo reports whether Undo would do something.
func (h *History) CanUndo() bool {
	return len(h.past) > 0
}

// CanRedo reports whether Redo would do something.
func (h *History) CanRedo() bool {
	return len(h.future) > 0
}

// Reset replaces the current grouping with g (deep-copied) and clears both
// stacks. Used when the model regroups.
func (h *History) Reset(g Grouping) {
	h.cur = g.Clone()
	h.past = nil
	h.future = nil
}

// Len returns the sizes of the undo and redo stacks.
func (h *History) Len() (undo, redo int) {
	return len(h.past), len(h.future)
}
