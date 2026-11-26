package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

var keyRe = regexp.MustCompile(`(?i)(sk-[a-z0-9_\-]{12,}|bearer\s+[a-z0-9_\-\.]{12,})`)

func git(repo string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", repo, "-c", "core.quotepath=off"}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), ee.Stderr)
		}
		return "", err
	}
	return string(out), nil
}

func load(repo, spec string, stdin bool) (diffText, context string, err error) {
	if stdin {
		b, err := io.ReadAll(os.Stdin)
		return string(b), "", err
	}
	common := []string{"--no-color", "--no-ext-diff", "--find-renames", "--unified=3"}
	if base, head, ok := strings.Cut(spec, "..."); ok {
		diffText, err = git(repo, append(append([]string{"diff"}, common...), base+"..."+head)...)
		if err != nil {
			return
		}
		var log string
		log, err = git(repo, "log", "--no-merges", "--format=- %h %s%n%w(0,2,2)%b", "-n", "60", base+".."+head)
		context = "COMMITS IN THIS RANGE (newest first):\n" + truncate(log, 6000)
		return
	}
	diffText, err = git(repo, append(append([]string{"show", "--format="}, common...), spec)...)
	if err != nil {
		return
	}
	var msg string
	msg, err = git(repo, "log", "-1", "--format=%h %s%n%n%b", spec)
	context = "COMMIT MESSAGE:\n" + truncate(msg, 4000)
	return
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "\n[... truncated]\n"
	}
	return s
}

func claudeArgv(model string) []string {
	return []string{"claude", "-p", "--model", model, "--output-format", "json", "--tools", "",
		"--setting-sources", "", "--strict-mcp-config", "--exclude-dynamic-system-prompt-sections",
		"--no-session-persistence", "--system-prompt", "You are a JSON API. Follow the user message exactly and output only JSON."}
}

func makeProvider(spec string) (Provider, error) {
	kind, rest, _ := strings.Cut(spec, ":")
	switch kind {
	case "claude":
		return &commandProvider{name: spec, argv: claudeArgv(rest), format: "claude-json"}, nil
	case "opencode":
		return &commandProvider{name: spec, argv: []string{"opencode", "run", "-m", rest, "--format", "json", "--archived"}, format: "opencode-json"}, nil
	case "cmd":
		return &commandProvider{name: spec, argv: strings.Fields(rest), format: "raw"}, nil
	case "openai":
		p := strings.Split(rest, "|")
		if len(p) != 3 {
			return nil, fmt.Errorf("openai provider wants base|model|KEY_ENV")
		}
		return &openAIProvider{name: spec, baseURL: p[0], model: p[1], keyEnv: p[2]}, nil
	}
	return nil, fmt.Errorf("unknown provider %q", spec)
}

type Stats struct {
	Spec          string   `json:"spec"`
	Provider      string   `json:"provider"`
	Files         int      `json:"files"`
	Hunks         int      `json:"hunks"`
	DiffBytes     int      `json:"diff_bytes"`
	PromptBytes   int      `json:"prompt_bytes"`
	EstTokens     int      `json:"est_tokens"`
	BodyCap       int      `json:"body_cap"`
	TruncHunks    int      `json:"truncated_hunks"`
	LatencyMS     int64    `json:"latency_ms"`
	Usage         Usage    `json:"usage"`
	Groups        int      `json:"groups"`
	Notices       []string `json:"notices"`
	Fallback      bool     `json:"fallback"`
	InvariantOK   bool     `json:"invariant_ok"`
	Error         string   `json:"error,omitempty"`
}

func markdown(d *Diff, r Result) string {
	byID := map[string]*Hunk{}
	for _, h := range d.Hunks {
		byID[h.ID] = h
	}
	var b strings.Builder
	for i, g := range r.Groups {
		add, del := 0, 0
		files := []string{}
		seenF := map[string]bool{}
		for _, id := range g.Hunks {
			h := byID[id]
			add += h.Add
			del += h.Del
			if !seenF[h.File.Path()] {
				seenF[h.File.Path()] = true
				files = append(files, h.File.Path())
			}
		}
		fmt.Fprintf(&b, "### %d. %s  (+%d -%d, %d hunks)\n\n%s\n\n", i+1, g.Title, add, del, len(g.Hunks), g.Description)
		for _, f := range files {
			var ids []string
			for _, id := range g.Hunks {
				if byID[id].File.Path() == f {
					ids = append(ids, id)
				}
			}
			fmt.Fprintf(&b, "- `%s` (%s)\n", f, strings.Join(ids, " "))
		}
		b.WriteString("\n")
	}
	for _, n := range r.Notices {
		fmt.Fprintf(&b, "> notice: %s\n", n)
	}
	return b.String()
}

func main() {
	repo := flag.String("repo", ".", "repository")
	prov := flag.String("provider", "claude:haiku", "claude:<model> | opencode:<model> | openai:<base>|<model>|<ENV> | cmd:<argv>")
	maxGroups := flag.Int("max-groups", 12, "group cap")
	budget := flag.Int("budget", 60000, "prompt token budget (bytes/4 estimate)")
	ceiling := flag.Int("ceiling", 150000, "refuse to send above this many estimated tokens")
	stdin := flag.Bool("stdin", false, "read a patch from stdin")
	dry := flag.Bool("dry-run", false, "print the prompt and exit")
	statsOut := flag.String("stats", "", "append a JSON stats line to this file")
	forceCap := flag.Int("force-cap", 0, "force a per-hunk line cap (testing truncation)")
	flag.Parse()
	spec := flag.Arg(0)
	diffText, ctxText, err := load(*repo, spec, *stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	d, err := Parse(diffText)
	if err != nil {
		fmt.Fprintln(os.Stderr, "parse:", err)
		os.Exit(1)
	}
	b := Build(d, BuildOpts{MaxGroups: *maxGroups, TokenBudget: *budget, Context: ctxText, ForceMaxBody: *forceCap})
	st := Stats{Spec: spec, Provider: *prov, Files: len(d.Files), Hunks: len(d.Hunks), DiffBytes: len(diffText),
		PromptBytes: len(b.Prompt), EstTokens: b.EstTokens, BodyCap: b.BodyCap, TruncHunks: b.Truncated}
	if *dry {
		fmt.Print(b.Prompt)
		fmt.Fprintf(os.Stderr, "files=%d hunks=%d est_tokens=%d body_cap=%d truncated=%d\n", st.Files, st.Hunks, st.EstTokens, st.BodyCap, st.TruncHunks)
		return
	}
	fmt.Fprintf(os.Stderr, "%s: %d files, %d hunks, ~%d tokens (cap %d, %d hunks truncated) -> %s\n", spec, st.Files, st.Hunks, st.EstTokens, st.BodyCap, st.TruncHunks, *prov)
	var r Result
	if b.EstTokens > *ceiling {
		r = FallbackPerFile(d, fmt.Sprintf("~%d tokens is above the ceiling %d", b.EstTokens, *ceiling))
	} else {
		p, err := makeProvider(*prov)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		t0 := time.Now()
		text, u, err := p.Complete(ctx, b.Prompt)
		cancel()
		st.LatencyMS = time.Since(t0).Milliseconds()
		st.Usage = u
		if err != nil {
			st.Error = err.Error()
			r = FallbackPerFile(d, "provider error: "+err.Error())
		} else {
			r = Validate(d, text, *maxGroups)
		}
	}
	st.Groups, st.Notices, st.Fallback = len(r.Groups), r.Notices, r.Fallback
	st.InvariantOK = Check(d, r) == nil
	fmt.Print(markdown(d, r))
	fmt.Fprintf(os.Stderr, "latency=%dms in=%d out=%d cost=$%.5f (%s) groups=%d notices=%d\n", st.LatencyMS, st.Usage.InputTokens, st.Usage.OutputTokens, st.Usage.CostUSD, st.Usage.CostSource, st.Groups, len(st.Notices))
	if *statsOut != "" {
		f, err := os.OpenFile(*statsOut, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err == nil {
			json.NewEncoder(f).Encode(st)
			f.Close()
		}
	}
}
