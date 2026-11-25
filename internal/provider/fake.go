package provider

import (
	"context"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
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
	f.mu.Lock()
	call := len(f.prompts)
	f.prompts = append(f.prompts, prompt)
	f.mu.Unlock()

	if f.Delay > 0 {
		timer := time.NewTimer(f.Delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return "", Usage{}, ctx.Err()
		case <-timer.C:
		}
	}
	if err := ctx.Err(); err != nil {
		return "", Usage{}, err
	}
	if f.Err != nil {
		return "", Usage{}, f.Err
	}

	var text string
	switch {
	case len(f.Responses) == 0:
		text = AutoAnswer(prompt)
	case call < len(f.Responses):
		text = f.Responses[call]
	default:
		text = f.Responses[len(f.Responses)-1]
	}

	u := f.Usage
	if u == (Usage{}) {
		u = Usage{
			InputTokens:  len(prompt) / 2,
			OutputTokens: len(text) / 2,
			CostSource:   CostEstimated,
		}
	}
	return text, u, nil
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

// Prompt markers and answer shapes of the fake provider.
const (
	groupTaskPrefix     = "# clump-task: group"
	summarizeTaskPrefix = "# clump-task: summarize"
	dataOpenPrefix      = "<<<CLUMP-DATA-"
	dataClosePrefix     = "<<<END-CLUMP-DATA-"
	rootGroup           = "(root)"
)

var (
	// catalogLineRe matches a catalog line: `id | status | path | ...`.
	catalogLineRe = regexp.MustCompile(`^([hc][0-9]+) \| [A-Z] \| (.+?) \| `)
	// summaryFileRe matches the heading of a per-file block in a summary prompt.
	summaryFileRe = regexp.MustCompile(`^=== file (.+?) \(`)
)

// fakeGroup is one entry of a fake {"groups":[...]} answer.
type fakeGroup struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Hunks       []string `json:"hunks"`
}

// fakeGroups is the fake answer to a grouping prompt.
type fakeGroups struct {
	Groups []fakeGroup `json:"groups"`
}

// fakeFileSummary is one entry of a fake {"files":[...]} answer.
type fakeFileSummary struct {
	Path    string `json:"path"`
	Summary string `json:"summary"`
}

// fakeSummaries is the fake answer to a summary prompt.
type fakeSummaries struct {
	Files []fakeFileSummary `json:"files"`
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
	data := promptData(prompt)
	switch task := taskLine(prompt); {
	case strings.HasPrefix(task, groupTaskPrefix):
		return marshalGroups(groupFakeAnswer(data))
	case strings.HasPrefix(task, summarizeTaskPrefix):
		return marshalSummaries(summaryFakeAnswer(data))
	default:
		return marshalGroups(fakeGroups{Groups: []fakeGroup{}})
	}
}

// taskLine is the first line of prompt, trimmed.
func taskLine(prompt string) string {
	first, _, _ := strings.Cut(prompt, "\n")
	return strings.TrimSpace(first)
}

// promptData is the text between the line starting with the
// <<<CLUMP-DATA- marker and the line starting with <<<END-CLUMP-DATA- (or the
// end of the prompt). It is empty when the opening marker is missing.
func promptData(prompt string) string {
	lines := strings.Split(prompt, "\n")
	start := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if start < 0 {
			if strings.HasPrefix(trimmed, dataOpenPrefix) {
				start = i + 1
			}
			continue
		}
		if strings.HasPrefix(trimmed, dataClosePrefix) {
			return strings.Join(lines[start:i], "\n")
		}
	}
	if start < 0 {
		return ""
	}
	return strings.Join(lines[start:], "\n")
}

// groupFakeAnswer builds one group per top-level directory, in order of first
// appearance, with the hunk IDs in catalog order.
func groupFakeAnswer(data string) fakeGroups {
	answer := fakeGroups{Groups: []fakeGroup{}}
	var dirs []string
	index := map[string]int{}
	for _, line := range strings.Split(data, "\n") {
		m := catalogLineRe.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		id, dir := m[1], dirOf(catalogPath(m[2]))
		at, ok := index[dir]
		if !ok {
			at = len(answer.Groups)
			index[dir] = at
			dirs = append(dirs, dir)
			answer.Groups = append(answer.Groups, fakeGroup{Title: "Changes in " + dir})
		}
		answer.Groups[at].Hunks = append(answer.Groups[at].Hunks, id)
	}
	for i := range answer.Groups {
		g := &answer.Groups[i]
		g.Description = "Fake grouping of " + strconv.Itoa(len(g.Hunks)) + " hunks in " + dirs[i] + "."
	}
	return answer
}

// summaryFakeAnswer builds one summary per "=== file" block heading.
func summaryFakeAnswer(data string) fakeSummaries {
	answer := fakeSummaries{Files: []fakeFileSummary{}}
	for _, line := range strings.Split(data, "\n") {
		m := summaryFileRe.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		answer.Files = append(answer.Files, fakeFileSummary{
			Path:    m[1],
			Summary: "Fake summary of " + m[1] + ".",
		})
	}
	return answer
}

// catalogPath is the path a catalog line counts for grouping: for a rename the
// new path, and for a collapsed unit (`docs/*.md (45 files)`) the pattern.
func catalogPath(path string) string {
	if _, after, ok := strings.Cut(path, " -> "); ok {
		path = after
	}
	if before, _, ok := strings.Cut(path, " ("); ok {
		path = before
	}
	return path
}

// dirOf is the path's first segment, or rootGroup when it has none.
func dirOf(path string) string {
	dir, _, ok := strings.Cut(path, "/")
	if !ok {
		return rootGroup
	}
	return dir
}

// marshalGroups renders answer as JSON. It cannot fail: fakeGroups has no
// value encoding/json rejects.
func marshalGroups(answer fakeGroups) string {
	b, err := json.Marshal(answer)
	if err != nil {
		return `{"groups":[]}`
	}
	return string(b)
}

// marshalSummaries renders answer as JSON. It cannot fail: fakeSummaries has
// no value encoding/json rejects.
func marshalSummaries(answer fakeSummaries) string {
	b, err := json.Marshal(answer)
	if err != nil {
		return `{"files":[]}`
	}
	return string(b)
}
