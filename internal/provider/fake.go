package provider

import (
	"context"
	"sync"
	"time"
)

// Fake is a scripted provider for tests and for the "fake" provider type.
// Its zero value answers every prompt with AutoAnswer(prompt).
type Fake struct {
	// ProviderName is returned by Name; "fake" when empty.
	ProviderName string
	// Responses are returned in order, one per call. After the last one, the
	// last response repeats. Empty means AutoAnswer.
	Responses []string
	// Err, when set, is returned by every call (after Delay).
	Err error
	// Delay is waited before answering; ctx cancellation ends the wait early
	// and returns ctx.Err().
	Delay time.Duration
	// Usage is returned by every successful call. When zero, the usage is
	// InputTokens = len(prompt)/2, OutputTokens = len(text)/2, CostUSD 0,
	// CostSource CostEstimated.
	Usage Usage

	mu      sync.Mutex
	prompts []string
}

// Name implements Provider.
func (f *Fake) Name() string {
	if f.ProviderName == "" {
		return "fake"
	}
	return f.ProviderName
}

// Complete implements Provider. It records the prompt before waiting.
func (f *Fake) Complete(ctx context.Context, prompt string) (string, Usage, error) {
	panic("provider: Fake.Complete not implemented")
}

// Calls returns how many times Complete was called.
func (f *Fake) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.prompts)
}

// Prompts returns a copy of every prompt received, in order.
func (f *Fake) Prompts() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.prompts...)
}

// AutoAnswer produces a deterministic, valid answer to a clump prompt
// without a model:
//
//   - A grouping prompt (its first line is "# clump-task: group") gets
//     {"groups":[...]} with one group per top-level directory of the catalog
//     paths (files at the repo root form the group "(root)"), in order of
//     first appearance. Title "Changes in <dir>", description
//     "Fake grouping of <n> hunks in <dir>.". Catalog lines are the lines
//     matching `^([hc][0-9]+) \| [A-Z] \| (.+?) \| ` between the data markers;
//     for "old -> new" the new path counts.
//   - A summary prompt (first line "# clump-task: summarize") gets
//     {"files":[{"path":p,"summary":"Fake summary of <p>."}...]} for every
//     line matching `^=== file (.+) \(` in order.
//   - Anything else gets {"groups":[]}.
func AutoAnswer(prompt string) string {
	panic("provider: AutoAnswer not implemented")
}
