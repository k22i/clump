package tui

import (
	"time"

	"github.com/k22i/clump/internal/cache"
	"github.com/k22i/clump/internal/engine"
	"github.com/k22i/clump/internal/gitdiff"
	"github.com/k22i/clump/internal/group"
	"github.com/k22i/clump/internal/report"
)

type docState int

const (
	stLoading docState = iota
	stRaw
	stGrouping
	stNeedsConfirm
	stGrouped
	stFailed
)

type Doc struct {
	L    *gitdiff.Loaded
	Hash string

	state docState
	err   error

	hist     *group.History
	base     group.Grouping
	baseKey  string
	provider string
	cached   bool
	fallback bool
	notices  []string

	snapshots []cache.Snapshot
	job       *engine.Job
	started   time.Time

	rkeys map[string]string
	numW  int
}

func newDoc(l *gitdiff.Loaded) *Doc {
	fb := group.FallbackPerFile(l.Diff, "")
	rkeys := make(map[string]string, len(l.Diff.Hunks))
	for _, h := range l.Diff.Hunks {
		rkeys[h.ID] = h.ReviewKey()
	}
	return &Doc{L: l, Hash: l.Diff.Hash(), state: stRaw, hist: group.NewHistory(fb.Grouping), rkeys: rkeys}
}

func (d *Doc) grouped() bool { return d.state == stGrouped }

func (d *Doc) edited() bool {
	return d.grouped() && !group.Equal(d.hist.Current(), d.base)
}

func (d *Doc) setBase(g group.Grouping, key, provider string, cached, fallback bool, notices []string) {
	d.base = g.Clone()
	d.baseKey, d.provider, d.cached, d.fallback = key, provider, cached, fallback
	d.notices = notices
	d.hist.Reset(g)
	d.state = stGrouped
	d.job = nil
}

func (d *Doc) report() report.Doc {
	return report.Doc{
		Title: d.L.Title, Ref: d.L.Ref, Diff: d.L.Diff, Grouping: d.hist.Current(),
		Notices: d.notices, Provider: d.provider, DiffHash: d.Hash, Cached: d.cached,
		Edited: d.edited(), Fallback: d.fallback, Snapshots: d.snapshots,
	}
}
