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
	panic("group: NewHistory not implemented")
}

// Current returns a deep copy of the current grouping.
func (h *History) Current() Grouping {
	panic("group: History.Current not implemented")
}

// Do makes next current, pushing the old current onto the undo stack and
// clearing the redo stack. If next is Equal to the current grouping it does
// nothing and returns false.
func (h *History) Do(next Grouping) bool {
	panic("group: History.Do not implemented")
}

// Apply runs op on the current grouping and, if it succeeds, Does the
// result. It returns op's error (the history is then unchanged) and whether
// the grouping changed.
func (h *History) Apply(op func(Grouping) (Grouping, error)) (changed bool, err error) {
	panic("group: History.Apply not implemented")
}

// Undo steps back one change; false if there is nothing to undo.
func (h *History) Undo() bool {
	panic("group: History.Undo not implemented")
}

// Redo steps forward one undone change; false if there is nothing to redo.
func (h *History) Redo() bool {
	panic("group: History.Redo not implemented")
}

// CanUndo reports whether Undo would do something.
func (h *History) CanUndo() bool {
	panic("group: History.CanUndo not implemented")
}

// CanRedo reports whether Redo would do something.
func (h *History) CanRedo() bool {
	panic("group: History.CanRedo not implemented")
}

// Reset replaces the current grouping with g (deep-copied) and clears both
// stacks. Used when the model regroups.
func (h *History) Reset(g Grouping) {
	panic("group: History.Reset not implemented")
}

// Len returns the sizes of the undo and redo stacks.
func (h *History) Len() (undo, redo int) {
	panic("group: History.Len not implemented")
}
