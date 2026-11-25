package tui

import (
	"time"

	"github.com/k22i/clump/internal/cache"
	"github.com/k22i/clump/internal/engine"
	"github.com/k22i/clump/internal/gitdiff"
	"github.com/k22i/clump/internal/group"
	"github.com/k22i/clump/internal/report"
)

// docState is where a document is in its life.
type docState int

const (
	stLoading      docState = iota // the diff is being loaded
	stRaw                          // the diff is shown, no grouping yet (and none requested)
	stGrouping                     // a provider call is running for it
	stNeedsConfirm                 // above the ceiling: waiting for y / g
	stGrouped                      // a grouping (model, cache or override) is shown
	stFailed                       // loading or grouping failed (err)
)

// Doc is one diff on screen: the loaded patch plus its grouping and edits.
type Doc struct {
	L    *gitdiff.Loaded
	Hash string

	state docState
	err   error

	// hist holds the effective grouping and the user's undo stack. Before
	// a grouping exists it holds the per-file fallback so the raw file list
	// is browsable (with editing disabled).
	hist *group.History
	// base is the grouping the model (or cache) produced; edits are
	// compared with it for the "edited" marker.
	base     group.Grouping
	baseKey  string
	provider string
	cached   bool
	fallback bool
	notices  []string

	snapshots []cache.Snapshot
	// job is the prepared request while stNeedsConfirm (and its estimate).
	job *engine.Job
	// started is when the current grouping call started.
	started time.Time

	// rkeys maps hunk ID to its ReviewKey.
	rkeys map[string]string
	// numW is the line-number width (0 until first needed).
	numW int
}

// newDoc returns a document for a loaded diff, showing its files until a
// grouping arrives.
func newDoc(l *gitdiff.Loaded) *Doc {
	fb := group.FallbackPerFile(l.Diff, "")
	rkeys := make(map[string]string, len(l.Diff.Hunks))
	for _, h := range l.Diff.Hunks {
		rkeys[h.ID] = h.ReviewKey()
	}
	return &Doc{L: l, Hash: l.Diff.Hash(), state: stRaw, hist: group.NewHistory(fb.Grouping), rkeys: rkeys}
}

// grouped reports whether the document shows a real grouping (editing is
// allowed only then).
func (d *Doc) grouped() bool { return d.state == stGrouped }

// edited reports whether the effective grouping differs from the base.
func (d *Doc) edited() bool {
	return d.grouped() && !group.Equal(d.hist.Current(), d.base)
}

// setBase installs a model/cache grouping and clears the undo stack.
func (d *Doc) setBase(g group.Grouping, key, provider string, cached, fallback bool, notices []string) {
	d.base = g.Clone()
	d.baseKey, d.provider, d.cached, d.fallback = key, provider, cached, fallback
	d.notices = notices
	d.hist.Reset(g)
	d.state = stGrouped
	d.job = nil
}

// report converts the document to the report form (for y, --json).
func (d *Doc) report() report.Doc {
	return report.Doc{
		Title: d.L.Title, Ref: d.L.Ref, Diff: d.L.Diff, Grouping: d.hist.Current(),
		Notices: d.notices, Provider: d.provider, DiffHash: d.Hash, Cached: d.cached,
		Edited: d.edited(), Fallback: d.fallback, Snapshots: d.snapshots,
	}
}
