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

type Fake struct {
	ProviderName string
	Responses    []string
	Err          error
	Delay        time.Duration
	Usage        Usage

	mu      sync.Mutex
	prompts []string
}

func (f *Fake) Name() string {
	if f.ProviderName == "" {
		return "fake"
	}
	return f.ProviderName
}

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

func (f *Fake) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.prompts)
}

func (f *Fake) Prompts() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.prompts...)
}

const (
	groupTaskPrefix     = "# clump-task: group"
	summarizeTaskPrefix = "# clump-task: summarize"
	dataOpenPrefix      = "<<<CLUMP-DATA-"
	dataClosePrefix     = "<<<END-CLUMP-DATA-"
	rootGroup           = "(root)"
)

var (
	catalogLineRe = regexp.MustCompile(`^([hc][0-9]+) \| [A-Z] \| (.+?) \| `)
	summaryFileRe = regexp.MustCompile(`^=== file (.+?) \(`)
)

type fakeGroup struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Hunks       []string `json:"hunks"`
}

type fakeGroups struct {
	Groups []fakeGroup `json:"groups"`
}

type fakeFileSummary struct {
	Path    string `json:"path"`
	Summary string `json:"summary"`
}

type fakeSummaries struct {
	Files []fakeFileSummary `json:"files"`
}

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

func taskLine(prompt string) string {
	first, _, _ := strings.Cut(prompt, "\n")
	return strings.TrimSpace(first)
}

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

func catalogPath(path string) string {
	if _, after, ok := strings.Cut(path, " -> "); ok {
		path = after
	}
	if before, _, ok := strings.Cut(path, " ("); ok {
		path = before
	}
	return path
}

func dirOf(path string) string {
	dir, _, ok := strings.Cut(path, "/")
	if !ok {
		return rootGroup
	}
	return dir
}

func marshalGroups(answer fakeGroups) string {
	b, err := json.Marshal(answer)
	if err != nil {
		return `{"groups":[]}`
	}
	return string(b)
}

func marshalSummaries(answer fakeSummaries) string {
	b, err := json.Marshal(answer)
	if err != nil {
		return `{"files":[]}`
	}
	return string(b)
}
