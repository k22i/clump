package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/k22i/clump/internal/cache"
	"github.com/k22i/clump/internal/config"
	"github.com/k22i/clump/internal/generated"
	"github.com/k22i/clump/internal/gitdiff"
	"github.com/k22i/clump/internal/group"
	"github.com/k22i/clump/internal/provider"
)

type Engine struct {
	Cfg   *config.Config
	Reg   *provider.Registry
	Store *cache.Store
	Now   func() time.Time

	detOnce sync.Once
	det     *generated.Detector
	detErr  error

	calOnce sync.Once
}

func New(cfg *config.Config, reg *provider.Registry, store *cache.Store) *Engine {
	return &Engine{Cfg: cfg, Reg: reg, Store: store, Now: time.Now}
}

type Input struct {
	Diff    *gitdiff.Diff
	Context string
}

type Estimate struct {
	Provider      string
	Tier          int
	Calls         int
	Tokens        int
	OutTokens     int
	CostUSD       float64
	Ceiling       int
	OverCeiling   bool
	BytesPerToken float64
}

func (e Estimate) String() string {
	s := fmt.Sprintf("~%s tokens, ~$%.4f", thousands(e.Tokens), e.CostUSD)
	if e.Calls > 1 {
		s += fmt.Sprintf(", %d calls", e.Calls)
	}
	return s
}

func thousands(n int) string {
	s := fmt.Sprint(n)
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return b.String()
}

type Job struct {
	In       Input
	Provider string
	PCfg     provider.Config
	DiffHash string
	Key      string
	Plan     *group.Plan
	Est      Estimate
}

type Outcome struct {
	Result   group.Result
	Usage    provider.Usage
	Provider string
	Key      string
	DiffHash string
	Calls    int
	Cached   bool
	Elapsed  time.Duration
	Raw      string
}

func (e *Engine) ResultKey(d *gitdiff.Diff, providerName string) (key, name string, pc provider.Config, err error) {
	name, err = e.Reg.Resolve(providerName)
	if err != nil {
		return "", "", provider.Config{}, err
	}
	pc, _ = e.Cfg.Provider(name)
	return cache.KeyWith(d.Hash(), name, pc.ModelID(), e.maxGroups(), generated.Signature(d)), name, pc, nil
}

func (e *Engine) MarkGenerated(ctx context.Context, d *gitdiff.Diff, repo string) {
	if e == nil || e.Cfg == nil {
		return
	}
	e.detOnce.Do(func() {
		e.det, e.detErr = generated.New(e.Cfg.Generated.Options())
	})
	if e.detErr != nil {
		return
	}
	e.det.Mark(ctx, d, repo)
}

func (e *Engine) maxGroups() int {
	if e.Cfg.MaxGroups > 0 {
		return e.Cfg.MaxGroups
	}
	return group.DefaultMaxGroups
}

func (e *Engine) Lookup(d *gitdiff.Diff, providerName string) (*Outcome, bool, error) {
	key, name, _, err := e.ResultKey(d, providerName)
	if err != nil {
		return nil, false, err
	}
	ent, ok, err := e.Store.GetResult(key)
	if err != nil || !ok {
		return nil, false, err
	}
	if group.Check(d, ent.Result.Grouping) != nil {
		return nil, false, nil
	}
	return &Outcome{Result: ent.Result, Usage: ent.Usage, Provider: name, Key: key, DiffHash: ent.DiffHash, Cached: true}, true, nil
}

func (e *Engine) BytesPerToken(name string, pc provider.Config) float64 {
	if v, ok := e.Store.Calibration(name); ok && v > 0 {
		return v
	}
	if pc.BytesPerToken > 0 {
		return pc.BytesPerToken
	}
	return provider.DefaultBytesPerToken(pc)
}

func (e *Engine) Prepare(in Input, providerName string) (*Job, error) {
	key, name, pc, err := e.ResultKey(in.Diff, providerName)
	if err != nil {
		return nil, err
	}
	bpt := e.BytesPerToken(name, pc)
	plan := group.Build(in.Diff, group.BuildOpts{
		MaxGroups:     e.maxGroups(),
		Budget:        e.Cfg.Budget,
		BytesPerToken: bpt,
		Context:       in.Context,
	})
	j := &Job{In: in, Provider: name, PCfg: pc, DiffHash: in.Diff.Hash(), Key: key, Plan: plan}
	j.Est = e.estimate(j, bpt)
	return j, nil
}

func (e *Engine) estimate(j *Job, bpt float64) Estimate {
	est := Estimate{Provider: j.Provider, Tier: j.Plan.Tier, Ceiling: e.Cfg.Ceiling, BytesPerToken: bpt}
	if len(j.Plan.Units) == 0 {
		return est
	}
	addCall := func(promptTokens, out int) {
		est.Calls++
		est.Tokens += promptTokens + j.PCfg.OverheadTokens
		est.OutTokens += out
	}
	if j.Plan.Tier == 2 {
		for _, c := range j.Plan.Summaries {
			if e.chunkCached(j, c) {
				continue
			}
			addCall(c.EstTokens, 80*len(c.Files))
		}
		addCall(j.Plan.FinalEstTokens, outGuess(len(j.Plan.Units)))
	} else {
		addCall(j.Plan.EstTokens, outGuess(len(j.Plan.Units)))
	}
	est.CostUSD = j.PCfg.Cost(est.Tokens, est.OutTokens)
	est.OverCeiling = est.Ceiling > 0 && est.Tokens > est.Ceiling
	return est
}

func outGuess(units int) int { return min(4000, 120+25*units) }

func (e *Engine) summaryKey(j *Job, fileIdx int) string {
	return cache.SummaryKey(j.In.Diff.FileHash(fileIdx), j.Provider, j.PCfg.ModelID())
}

func (e *Engine) chunkCached(j *Job, c group.SummaryChunk) bool {
	for _, fi := range c.Files {
		if _, ok, err := e.Store.GetSummary(e.summaryKey(j, fi)); err != nil || !ok {
			return false
		}
	}
	return true
}

type Progress struct {
	Step  int
	Steps int
	What  string
}

func (e *Engine) Run(ctx context.Context, j *Job, progress func(Progress)) (*Outcome, error) {
	start := e.Now()
	p, pc, err := e.Reg.Get(j.Provider)
	if err != nil {
		return nil, err
	}
	out := &Outcome{Provider: j.Provider, Key: j.Key, DiffHash: j.DiffHash}
	if res, ok := group.AllGenerated(j.In.Diff); ok {
		out.Result, out.Elapsed = res, e.Now().Sub(start)
		return out, nil
	}
	report := func(what string) {
		if progress != nil {
			progress(Progress{Step: out.Calls + 1, Steps: j.Est.Calls, What: what})
		}
	}
	var notices []string
	call := func(prompt, what string) (string, error) {
		report(what)
		text, u, err := p.Complete(ctx, prompt)
		out.Calls++
		if err != nil {
			return "", fmt.Errorf("%s: %w", what, err)
		}
		out.Usage = out.Usage.Add(u)
		if cerr := e.calibrate(j.Provider, pc, len(prompt), u); cerr != nil {
			if msg, first := e.calibrationNotice(cerr); first {
				notices = append(notices, msg)
			}
		}
		return text, nil
	}
	prompt := j.Plan.Prompt
	if j.Plan.Tier == 2 {
		summaries, sn, err := e.summarise(j, call)
		if err != nil {
			return nil, err
		}
		notices = append(notices, sn...)
		prompt, _ = j.Plan.BuildFromSummaries(j.In.Diff, summaries)
	}
	text, err := call(prompt, "grouping")
	if err != nil {
		return nil, err
	}
	out.Raw = text
	res := group.Validate(j.In.Diff, text, group.ValidateOpts{MaxGroups: e.maxGroups(), Aliases: j.Plan.Aliases})
	res.Notices = append(append(append([]string(nil), j.Plan.Notices...), notices...), res.Notices...)
	out.Result = res
	out.Elapsed = e.Now().Sub(start)
	if err := e.Store.PutResult(cache.Entry{
		Key: j.Key, DiffHash: j.DiffHash, Provider: j.Provider, Model: pc.ModelID(),
		PromptVersion: group.PromptVersion, MaxGroups: e.maxGroups(), CreatedAt: e.Now().UTC(),
		Result: res, Usage: out.Usage,
	}); err != nil {
		out.Result.Notices = append(out.Result.Notices, "cache write failed: "+err.Error())
	}
	return out, nil
}

func (e *Engine) summarise(j *Job, call func(prompt, what string) (string, error)) (map[int]string, []string, error) {
	d := j.In.Diff
	out := map[int]string{}
	var notices []string
	for ci, c := range j.Plan.Summaries {
		if e.chunkCached(j, c) {
			for _, fi := range c.Files {
				if s, ok, _ := e.Store.GetSummary(e.summaryKey(j, fi)); ok {
					out[fi] = s.Summary
				}
			}
			continue
		}
		text, err := call(c.Prompt, fmt.Sprintf("summary %d/%d", ci+1, len(j.Plan.Summaries)))
		if err != nil {
			return nil, nil, err
		}
		got, err := group.ParseSummaries(text)
		if err != nil {
			notices = append(notices, fmt.Sprintf("summaries for %d files unusable: %v", len(c.Files), err))
			continue
		}
		missing := 0
		for _, fi := range c.Files {
			path := d.Files[fi].DisplayPath()
			s, ok := got[path]
			if !ok {
				s, ok = got[d.Files[fi].Path()]
			}
			if !ok {
				missing++
				continue
			}
			out[fi] = s
			if err := e.Store.PutSummary(cache.Summary{Key: e.summaryKey(j, fi), Path: path, Summary: s, CreatedAt: e.Now().UTC()}); err != nil {
				notices = append(notices, "summary cache write failed: "+err.Error())
			}
		}
		if missing > 0 {
			notices = append(notices, fmt.Sprintf("%d files got no summary", missing))
		}
	}
	return out, notices, nil
}

func (e *Engine) calibrate(name string, pc provider.Config, promptBytes int, u provider.Usage) error {
	if pc.Type == provider.TypeFake || (pc.Type == provider.TypeCommand && (pc.Format == "" || pc.Format == provider.FormatRaw)) {
		return nil
	}
	if u.InputTokens <= 0 {
		return nil
	}
	if _, err := e.Store.Calibrate(name, promptBytes, u.InputTokens, pc.OverheadTokens); err != nil {
		return err
	}
	return nil
}

func (e *Engine) calibrationNotice(err error) (string, bool) {
	first := false
	e.calOnce.Do(func() { first = true })
	if !first {
		return "", false
	}
	return "token calibration stopped updating, estimates keep the previous ratio: " + err.Error(), true
}

func (e *Engine) DryRun(j *Job) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# clump dry run: provider %s, tier %d, %s (budget %d, ceiling %d)\n",
		j.Provider, j.Plan.Tier, j.Est, e.Cfg.Budget, e.Cfg.Ceiling)
	for _, n := range j.Plan.Notices {
		fmt.Fprintf(&b, "# %s\n", n)
	}
	for _, f := range j.In.Diff.Files {
		if f.Generated != "" {
			fmt.Fprintf(&b, "# not sent (generated, %s): %s\n", f.Generated, f.DisplayPath())
		}
	}
	if len(j.Plan.Units) == 0 {
		b.WriteString("\n# every file is generated: nothing would be sent\n")
		return b.String()
	}
	if j.Plan.Tier == 2 {
		for i, c := range j.Plan.Summaries {
			cached := ""
			if e.chunkCached(j, c) {
				cached = " (cached, not sent)"
			}
			fmt.Fprintf(&b, "\n# ---- request %d: summary chunk %d/%d%s ----\n%s", i+1, i+1, len(j.Plan.Summaries), cached, c.Prompt)
		}
		pending := map[int]string{}
		for i := range j.In.Diff.Files {
			pending[i] = "(summary pending)"
		}
		final, _ := j.Plan.BuildFromSummaries(j.In.Diff, pending)
		fmt.Fprintf(&b, "\n# ---- request %d: grouping ----\n%s", len(j.Plan.Summaries)+1, final)
		return b.String()
	}
	fmt.Fprintf(&b, "\n# ---- request 1: grouping ----\n%s", j.Plan.Prompt)
	return b.String()
}

var ErrOverCeiling = errors.New("estimate is above the ceiling")
