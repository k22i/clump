package group

import (
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"unicode"

	"github.com/k22i/clump/internal/catalog"
	"github.com/k22i/clump/internal/gitdiff"
)

//go:embed prompt_v1.txt
var buildGroupPrompt string

//go:embed prompt_summary_v1.txt
var buildSummaryPrompt string

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

func (o BuildOpts) buildNorm() BuildOpts {
	if o.MaxGroups <= 0 {
		o.MaxGroups = DefaultMaxGroups
	}
	if o.BytesPerToken <= 0 {
		o.BytesPerToken = 3.3
	}
	if o.MinBodyCap <= 0 {
		o.MinBodyCap = 4
	}
	return o
}

func (o BuildOpts) buildNonce() string {
	if o.Nonce != "" {
		return o.Nonce
	}
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("group: crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}

func (o BuildOpts) buildEst(prompt string) int {
	return int(math.Ceil(float64(len(prompt)) / o.BytesPerToken))
}

func (o BuildOpts) fits(prompt string) bool { return o.Budget <= 0 || o.buildEst(prompt) <= o.Budget }

func buildCatalog(units []catalog.Unit) string {
	var b strings.Builder
	for _, u := range units {
		b.WriteString(catalog.Line(u))
		b.WriteByte('\n')
	}
	return b.String()
}

func buildContext(ctx string) string {
	return strings.TrimRight(ctx, "\n")
}

// buildRenderGroup fills prompt_v1.txt. hunks is the whole HUNKS section.
func (o BuildOpts) buildRenderGroup(nonce, cat, hunks string) string {
	r := strings.NewReplacer("{{NONCE}}", nonce, "{{MAX_GROUPS}}", fmt.Sprint(o.MaxGroups),
		"{{CONTEXT}}", buildContext(o.Context), "{{CATALOG}}", cat, "{{HUNKS}}", hunks)
	return r.Replace(buildGroupPrompt)
}

func (o BuildOpts) buildRenderSummary(nonce, files string) string {
	r := strings.NewReplacer("{{NONCE}}", nonce, "{{CONTEXT}}", buildContext(o.Context), "{{FILES}}", files)
	return r.Replace(buildSummaryPrompt)
}

// buildHunks renders the HUNKS section with a per-unit body cap.
func buildHunks(units []catalog.Unit, capLines int) (text string, truncated, dropped int) {
	var b strings.Builder
	b.WriteString("HUNKS:\n")
	for _, u := range units {
		t, dr := catalog.Block(u, capLines)
		b.WriteString(t)
		if dr > 0 {
			truncated++
			dropped += dr
		}
	}
	return b.String(), truncated, dropped
}

// Build renders the prompt(s) for d. It never truncates the catalog or hunk
// headers. Tier 1 binary-searches the largest per-hunk body cap that fits
// Budget. When that cap would fall below MinBodyCap, or the catalog alone
// exceeds Budget, it plans tier 2. See DESIGN.md "Large diffs".
func Build(d *gitdiff.Diff, o BuildOpts) *Plan {
	o = o.buildNorm()
	o.Context = withGeneratedNote(o.Context, d)
	units := catalog.Units(d, catalog.Options{CollapseMin: o.CollapseMin})
	p := &Plan{Tier: 1, Units: units, Aliases: catalog.Aliases(units), BodyCap: -1, opts: o}
	cat := buildCatalog(units)
	nonce := o.buildNonce()
	render := func(capLines int) (string, int, int) {
		h, tr, dr := buildHunks(units, capLines)
		return o.buildRenderGroup(nonce, cat, h), tr, dr
	}
	full, _, _ := render(-1)
	if o.fits(full) {
		p.Prompt, p.EstTokens = full, o.buildEst(full)
		return p
	}
	zero, tr0, dr0 := render(0)
	best, bestCap, bestTr, bestDr := "", -1, 0, 0
	if o.fits(zero) {
		best, bestCap, bestTr, bestDr = zero, 0, tr0, dr0
		lo, hi := 1, 0
		for _, u := range units {
			hi = max(hi, len(u.Hunks[0].Lines))
		}
		for lo <= hi {
			mid := (lo + hi) / 2
			pr, tr, dr := render(mid)
			if o.fits(pr) {
				best, bestCap, bestTr, bestDr = pr, mid, tr, dr
				lo = mid + 1
			} else {
				hi = mid - 1
			}
		}
	}
	if bestCap >= o.MinBodyCap {
		p.Prompt, p.EstTokens, p.BodyCap, p.Truncated, p.DroppedLines = best, o.buildEst(best), bestCap, bestTr, bestDr
		p.Notices = append(p.Notices, fmt.Sprintf("large diff: hunk bodies cut to %d lines (%d lines omitted) to fit the budget of %d tokens",
			bestCap, bestDr, o.Budget))
		return p
	}
	buildTier2(d, p, o, zero)
	return p
}

// buildTier2 plans the per-file summary calls and estimates the final call.
func buildTier2(d *gitdiff.Diff, p *Plan, o BuildOpts, catalogOnly string) {
	p.Tier, p.Prompt, p.BodyCap = 2, "", -1
	if !o.fits(catalogOnly) {
		p.Notices = append(p.Notices, fmt.Sprintf("the catalog alone (~%d tokens) exceeds the budget of %d tokens; the grouping call will go over it",
			o.buildEst(catalogOnly), o.Budget))
	}
	nonce := o.buildNonce()
	// Each file's cap: untruncated if it fits a chunk alone, else the largest cap that does.
	type fb struct {
		idx  int
		text string
		cap  int
	}
	var blocks []fb
	for i, f := range d.Files {
		if f.Generated != "" {
			continue
		}
		t, _ := catalog.FileBlock(d, i, -1)
		capLines := -1
		if !o.fits(o.buildRenderSummary(nonce, t)) {
			lo, hi, best := 0, 0, 0
			for _, h := range f.Hunks {
				hi = max(hi, len(h.Lines))
			}
			t, _ = catalog.FileBlock(d, i, 0)
			for lo <= hi {
				mid := (lo + hi) / 2
				tt, _ := catalog.FileBlock(d, i, mid)
				if o.fits(o.buildRenderSummary(nonce, tt)) {
					best, t, lo = mid, tt, mid+1
				} else {
					hi = mid - 1
				}
			}
			capLines = best
		}
		blocks = append(blocks, fb{i, t, capLines})
	}
	var cur []fb
	flush := func() {
		if len(cur) == 0 {
			return
		}
		var b strings.Builder
		ch := SummaryChunk{BodyCap: -1}
		for _, x := range cur {
			b.WriteString(x.text)
			ch.Files = append(ch.Files, x.idx)
			if x.cap >= 0 && (ch.BodyCap < 0 || x.cap < ch.BodyCap) {
				ch.BodyCap = x.cap
			}
		}
		ch.Prompt = o.buildRenderSummary(o.buildNonce(), b.String())
		ch.EstTokens = o.buildEst(ch.Prompt)
		p.Summaries = append(p.Summaries, ch)
		cur = nil
	}
	for _, x := range blocks {
		var b strings.Builder
		for _, y := range cur {
			b.WriteString(y.text)
		}
		b.WriteString(x.text)
		if len(cur) > 0 && !o.fits(o.buildRenderSummary(nonce, b.String())) {
			flush()
		}
		cur = append(cur, x)
	}
	flush()
	pending := map[int]string{}
	for i := range d.Files {
		pending[i] = strings.Repeat("x", 200) // a typical summary's size, for the estimate
	}
	final, _ := p.BuildFromSummaries(d, pending)
	p.FinalEstTokens = o.buildEst(final)
	p.EstTokens = p.FinalEstTokens
	files := 0
	for _, c := range p.Summaries {
		p.EstTokens += c.EstTokens
		files += len(c.Files)
	}
	p.Notices = append(p.Notices, fmt.Sprintf("large diff: summarising %d files in %d calls first, then grouping over the summaries",
		files, len(p.Summaries)))
}

// maxGeneratedNote caps the generated paths listed in the prompt context.
const maxGeneratedNote = 40

// withGeneratedNote appends to the prompt context the paths of the generated
// files left out of the request, so the model knows they changed (a schema
// change and its migration, say) without paying for their bodies.
func withGeneratedNote(ctx string, d *gitdiff.Diff) string {
	var paths []string
	for _, f := range d.Files {
		if f.Generated != "" {
			paths = append(paths, f.DisplayPath())
		}
	}
	if len(paths) == 0 {
		return ctx
	}
	more := ""
	if len(paths) > maxGeneratedNote {
		more = fmt.Sprintf(", and %d more", len(paths)-maxGeneratedNote)
		paths = paths[:maxGeneratedNote]
	}
	note := "Generated files also changed (left out of this request; do not group them): " + strings.Join(paths, ", ") + more
	if strings.TrimSpace(ctx) == "" {
		return note
	}
	return strings.TrimRight(ctx, "\n") + "\n\n" + note
}

// BuildFromSummaries renders the tier-2 grouping prompt: the full catalog plus
// one summary per file (summaries maps a file index to its summary; a
// missing entry is shown as "(no summary)"). It returns the prompt and its
// token estimate.
func (p *Plan) BuildFromSummaries(d *gitdiff.Diff, summaries map[int]string) (prompt string, estTokens int) {
	o := p.opts.buildNorm()
	var b strings.Builder
	b.WriteString("FILE SUMMARIES (written by an earlier pass; hunk bodies omitted):\n")
	for i, f := range d.Files {
		if f.Generated != "" {
			continue
		}
		s, ok := summaries[i]
		if !ok || strings.TrimSpace(s) == "" {
			s = "(no summary)"
		}
		fmt.Fprintf(&b, "- %s: %s\n", f.DisplayPath(), s)
	}
	prompt = o.buildRenderGroup(o.buildNonce(), buildCatalog(p.Units), b.String())
	return prompt, o.buildEst(prompt)
}

// ParseSummaries reads a summary call's answer,
// {"files":[{"path":"...","summary":"..."}]}, into path -> summary.
// Summaries are sanitised like titles and capped at 400 runes.
func ParseSummaries(text string) (map[string]string, error) {
	js, ok := ExtractJSON(text)
	if !ok {
		return nil, fmt.Errorf("no JSON object in summary output")
	}
	var raw struct {
		Files []struct {
			Path    string `json:"path"`
			Summary string `json:"summary"`
		} `json:"files"`
	}
	if err := json.Unmarshal([]byte(js), &raw); err != nil {
		return nil, fmt.Errorf("invalid summary JSON: %w", err)
	}
	out := map[string]string{}
	for _, f := range raw.Files {
		s := buildCleanLine(f.Summary, 400)
		if p := strings.TrimSpace(f.Path); p != "" && s != "" {
			out[p] = s
		}
	}
	return out, nil
}

// buildCleanLine turns control runes into spaces, collapses whitespace and
// cuts to n runes (n-1 plus an ellipsis).
func buildCleanLine(s string, n int) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		s = string(r[:n-1]) + "…"
	}
	return s
}
