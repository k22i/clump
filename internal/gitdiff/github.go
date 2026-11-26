package gitdiff

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type PRSpec struct {
	Owner, Repo, Number string
}

func (p PRSpec) slug() string {
	if p.Owner == "" {
		return ""
	}
	return p.Owner + "/" + p.Repo
}

func ParsePRSpec(spec string) (PRSpec, error) {
	s := strings.TrimSpace(spec)
	if strings.HasPrefix(s, "#") || isDigits(s) {
		return parsePRNumber(strings.TrimPrefix(s, "#"), spec, nil)
	}
	u := s
	for _, pfx := range []string{"https://", "http://", "www."} {
		u = strings.TrimPrefix(u, pfx)
	}
	u = strings.TrimPrefix(u, "github.com/")
	u = strings.TrimSuffix(u, "/")
	for _, suf := range []string{".diff", ".patch"} {
		u = strings.TrimSuffix(u, suf)
	}
	if owner, rest, ok := strings.Cut(u, "/"); ok {
		if repo, rest2, ok2 := strings.Cut(rest, "/pull/"); ok2 && isRepoName(owner) && isRepoName(repo) {
			return parsePRNumber(rest2, spec, func(p PRSpec) PRSpec {
				p.Owner, p.Repo = owner, repo
				return p
			})
		}
	}
	if owner, rest, ok := strings.Cut(u, "/"); ok {
		if repo, n, ok2 := strings.Cut(rest, "#"); ok2 && isRepoName(owner) && isRepoName(repo) {
			return parsePRNumber(n, spec, func(p PRSpec) PRSpec {
				p.Owner, p.Repo = owner, repo
				return p
			})
		}
	}
	return PRSpec{}, fmt.Errorf("%q is not a pull request: use 123, #123, OWNER/REPO#123 or a github.com pull request URL", spec)
}

func parsePRNumber(n, spec string, fill func(PRSpec) PRSpec) (PRSpec, error) {
	if !isDigits(n) {
		return PRSpec{}, fmt.Errorf("%q does not name a pull request number", spec)
	}
	num := strings.TrimLeft(n, "0")
	if num == "" {
		return PRSpec{}, fmt.Errorf("pull request number %q is not a positive integer", n)
	}
	p := PRSpec{Number: num}
	if fill != nil {
		p = fill(p)
	}
	return p, nil
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func isRepoName(s string) bool {
	if s == "" || len(s) > 100 || strings.HasPrefix(s, ".") || strings.HasSuffix(s, ".") {
		return false
	}
	for _, c := range s {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_', c == '.':
		default:
			return false
		}
	}
	return true
}

func runGh(ctx context.Context, dir string, args ...string) (string, error) {
	return Gh(ctx, dir, "", args...)
}

func Gh(ctx context.Context, dir, stdin string, args ...string) (string, error) {
	name := "gh"
	if len(args) > 0 {
		name = "gh " + args[0]
	}
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("%s: %w", name, err)
	}
	cmd := exec.CommandContext(ctx, "gh", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(), "NO_COLOR=1", "GH_NO_UPDATE_NOTIFIER=1")
	var stdouts, stderrs strings.Builder
	cmd.Stdout = &stdouts
	cmd.Stderr = &stderrs
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	err := cmd.Run()
	if err == nil {
		return stdouts.String(), nil
	}
	if cerr := ctx.Err(); cerr != nil {
		return "", fmt.Errorf("%s: %w", name, cerr)
	}
	var execErr *exec.Error
	if errors.As(err, &execErr) {
		return "", errors.New("gh is not on PATH: --pr needs the GitHub CLI (https://cli.github.com)")
	}
	return "", fmt.Errorf("%s: %s", strings.Join(args, " "), clip(strings.TrimSpace(stderrs.String()), 500))
}

type prView struct {
	Number  int    `json:"number"`
	Title   string `json:"title"`
	Body    string `json:"body"`
	Commits []struct {
		OID             string `json:"oid"`
		MessageHeadline string `json:"messageHeadline"`
	} `json:"commits"`
}

func LoadPR(ctx context.Context, repo, spec string) (*Loaded, error) {
	p, err := ParsePRSpec(spec)
	if err != nil {
		return nil, err
	}
	slug := p.slug()
	if slug == "" && repo == "" {
		return nil, fmt.Errorf("pr #%s: run inside a clone of the repository, or name it: --pr OWNER/REPO#%s", p.Number, p.Number)
	}
	repoArgs := func(sub string, extra ...string) []string {
		args := []string{"pr", sub, p.Number}
		if sub == "diff" {
			args = append(args, "--color=never")
		}
		if slug != "" {
			args = append(args, "--repo", slug)
		}
		return append(args, extra...)
	}
	raw, err := runGh(ctx, repo, repoArgs("diff")...)
	if err != nil {
		return nil, err
	}
	d, err := Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse pr #%s: %w", p.Number, err)
	}
	viewJSON, err := runGh(ctx, repo, repoArgs("view", "--json", "number,title,body,commits")...)
	if err != nil {
		return nil, err
	}
	var view prView
	if err := json.Unmarshal([]byte(viewJSON), &view); err != nil {
		return nil, fmt.Errorf("gh pr view #%s: %w", p.Number, err)
	}
	number := view.Number
	if number == 0 {
		number, _ = strconv.Atoi(p.Number)
	}
	ref := fmt.Sprintf("pr/%d", number)
	if slug != "" {
		ref = fmt.Sprintf("%s#%d", slug, number)
	}
	title := fmt.Sprintf("#%d %s", number, strings.TrimSpace(view.Title))
	if view.Title == "" {
		title = fmt.Sprintf("#%d", number)
	}
	return &Loaded{
		Diff:    d,
		Raw:     raw,
		Context: capContext(prContext(view)),
		Title:   title,
		Ref:     ref,
	}, nil
}

func prContext(view prView) string {
	ctx := "PULL REQUEST: " + strings.TrimSpace(view.Title) + "\n"
	if body := strings.TrimSpace(view.Body); body != "" {
		ctx += "\n" + body + "\n"
	}
	if len(view.Commits) > 0 {
		ctx += "\nCOMMITS IN THIS PULL REQUEST (oldest first):\n"
		for _, c := range view.Commits {
			ctx += "- " + c.MessageHeadline
			if c.OID != "" {
				ctx += " (" + shortSHA(c.OID) + ")"
			}
			ctx += "\n"
		}
	}
	return ctx
}
