package group

import (
	"github.com/k22i/clump/internal/catalog"
	"github.com/k22i/clump/internal/gitdiff"
)

// BuildOpts configures Build.
type BuildOpts struct {
	// MaxGroups is written into the prompt; <= 0 means DefaultMaxGroups.
	MaxGroups int
	// Budget is the token budget for one prompt; <= 0 means unlimited.
	Budget int
	// BytesPerToken converts prompt bytes to tokens; <= 0 means 3.3.
	BytesPerToken float64
	// Context is the commit message(s), placed inside the data markers.
	Context string
	// CollapseMin is passed to catalog.Units.
	CollapseMin int
	// MinBodyCap is the smallest per-hunk body cap tier 1 accepts before
	// switching to tier 2; <= 0 means 4.
	MinBodyCap int
	// Nonce fixes the data-marker nonce (tests only); "" means random.
	Nonce string
}

// Plan is what Build decided to send.
type Plan struct {
	// Tier is 1 (one grouping call, bodies possibly truncated) or 2
	// (per-file summary calls, then one grouping call over the summaries).
	Tier    int
	Units   []catalog.Unit
	Aliases map[string][]string

	// Prompt is the tier-1 grouping prompt ("" for tier 2 until the
	// summaries are known; see BuildFromSummaries).
	Prompt string
	// EstTokens estimates the whole plan's input tokens (all calls,
	// excluding per-call provider overhead).
	EstTokens int
	// BodyCap is the per-hunk body line cap tier 1 used; -1 is untruncated.
	BodyCap      int
	Truncated    int // hunks whose bodies were cut
	DroppedLines int // body lines cut in total

	// Summaries are the tier-2 summary calls, in file order.
	Summaries []SummaryChunk
	// FinalEstTokens estimates the tier-2 grouping call.
	FinalEstTokens int

	Notices []string
	opts    BuildOpts
}

// SummaryChunk is one tier-2 summary call over one or more whole files.
type SummaryChunk struct {
	Files     []int // indexes into Diff.Files
	Prompt    string
	EstTokens int
	BodyCap   int // -1 is untruncated
}

// Calls is the number of provider calls the plan needs (before the cache).
func (p *Plan) Calls() int {
	if p.Tier == 2 {
		return len(p.Summaries) + 1
	}
	return 1
}

// Build renders the prompt(s) for d. It never truncates the catalog or hunk
// headers. Tier 1 binary-searches the largest per-hunk body cap that fits
// Budget. When that cap would fall below MinBodyCap, or the catalog alone
// exceeds Budget, it plans tier 2. See DESIGN.md "Large diffs".
func Build(d *gitdiff.Diff, o BuildOpts) *Plan {
	panic("group: Build not implemented")
}

// BuildFromSummaries renders the tier-2 grouping prompt: the full catalog plus
// one summary per file (summaries maps a file index to its summary; a
// missing entry is shown as "(no summary)"). It returns the prompt and its
// token estimate.
func (p *Plan) BuildFromSummaries(d *gitdiff.Diff, summaries map[int]string) (prompt string, estTokens int) {
	panic("group: BuildFromSummaries not implemented")
}

// ParseSummaries reads a summary call's answer,
// {"files":[{"path":"...","summary":"..."}]}, into path -> summary.
// Summaries are sanitised like titles and capped at 400 runes.
func ParseSummaries(text string) (map[string]string, error) {
	panic("group: ParseSummaries not implemented")
}
