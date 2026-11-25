package group

import "errors"

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

// Move moves hunkIDs to the group toID, removing them from wherever they
// are. toID may be UngroupedID, which is created at the end if absent.
func Move(g Grouping, hunkIDs []string, toID string) (Grouping, error) {
	panic("group: Move not implemented")
}

// MoveToNew moves hunkIDs into a new group with the given title, appended at
// the end, and returns the new group's ID.
func MoveToNew(g Grouping, hunkIDs []string, title string) (Grouping, string, error) {
	panic("group: MoveToNew not implemented")
}

// NewGroup appends an empty group with the given title and returns its ID.
func NewGroup(g Grouping, title string) (Grouping, string, error) {
	panic("group: NewGroup not implemented")
}

// Rename sets a group's title and description.
func Rename(g Grouping, id, title, description string) (Grouping, error) {
	panic("group: Rename not implemented")
}

// Delete removes a group; its hunks go to Ungrouped.
func Delete(g Grouping, id string) (Grouping, error) {
	panic("group: Delete not implemented")
}

// Reorder moves a group delta positions (negative is up), clamped to the
// list bounds.
func Reorder(g Grouping, id string, delta int) (Grouping, error) {
	panic("group: Reorder not implemented")
}

// Merge moves every hunk of srcID into dstID and removes srcID.
func Merge(g Grouping, srcID, dstID string) (Grouping, error) {
	panic("group: Merge not implemented")
}

// NextGroupID returns "g<N+1>" where N is the largest number among IDs of the
// form "g<digits>" in g (0 if none).
func NextGroupID(g Grouping) string {
	panic("group: NextGroupID not implemented")
}
