package group

type History struct {
	past   []Grouping
	cur    Grouping
	future []Grouping
}

func NewHistory(initial Grouping) *History {
	return &History{cur: initial.Clone()}
}

func (h *History) Current() Grouping {
	return h.cur.Clone()
}

func (h *History) Do(next Grouping) bool {
	if Equal(next, h.cur) {
		return false
	}
	h.past = append(h.past, h.cur)
	h.cur = next.Clone()
	h.future = nil
	return true
}

func (h *History) Apply(op func(Grouping) (Grouping, error)) (changed bool, err error) {
	next, err := op(h.Current())
	if err != nil {
		return false, err
	}
	return h.Do(next), nil
}

func (h *History) Undo() bool {
	if len(h.past) == 0 {
		return false
	}
	h.future = append(h.future, h.cur)
	h.cur = h.past[len(h.past)-1]
	h.past = h.past[:len(h.past)-1]
	return true
}

func (h *History) Redo() bool {
	if len(h.future) == 0 {
		return false
	}
	h.past = append(h.past, h.cur)
	h.cur = h.future[len(h.future)-1]
	h.future = h.future[:len(h.future)-1]
	return true
}

func (h *History) CanUndo() bool {
	return len(h.past) > 0
}

func (h *History) CanRedo() bool {
	return len(h.future) > 0
}

func (h *History) Reset(g Grouping) {
	h.cur = g.Clone()
	h.past = nil
	h.future = nil
}

func (h *History) Len() (undo, redo int) {
	return len(h.past), len(h.future)
}
