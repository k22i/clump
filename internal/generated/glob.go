package generated

import (
	"fmt"
	"regexp"
	"strings"
)

// Pattern is one compiled path pattern.
type Pattern struct {
	// Text is the pattern as written.
	Text string
	re   *regexp.Regexp
}

// Match reports whether the slash-separated, repo-relative path p matches.
func (p Pattern) Match(path string) bool { return p.re.MatchString(path) }

// Compile compiles a gitignore-style path pattern:
//
//   - A pattern with no "/" (other than a trailing one) matches a file or
//     directory name at any depth: "go.sum", "*.min.js", "vendor/".
//   - A pattern with a "/" is anchored at the repository root (a leading "/"
//     is optional): "db/schema.rb". Start it with "**/" to match at any depth.
//   - "*" matches within one path segment, "?" one character, "**" any
//     number of segments.
//   - A pattern that matches a directory matches everything below it, so
//     "vendor/" and "**/__generated__" skip whole trees.
//
// Character classes and braces are not supported; "[", "]", "{" and "}"
// match themselves.
func Compile(text string) (Pattern, error) {
	p := strings.TrimSpace(text)
	if p == "" {
		return Pattern{}, fmt.Errorf("empty pattern")
	}
	if strings.Contains(p, "\\") {
		return Pattern{}, fmt.Errorf("pattern %q: use / as the path separator", text)
	}
	dir := strings.HasSuffix(p, "/")
	p = strings.TrimRight(p, "/")
	anchored := strings.Contains(p, "/")
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return Pattern{}, fmt.Errorf("pattern %q matches everything", text)
	}

	var b strings.Builder
	b.WriteString("^")
	if !anchored {
		b.WriteString("(?:.*/)?")
	}
	for i := 0; i < len(p); i++ {
		c := p[i]
		switch {
		case c == '*' && i+1 < len(p) && p[i+1] == '*':
			i++
			if i+1 < len(p) && p[i+1] == '/' {
				b.WriteString("(?:.*/)?")
				i++
			} else {
				b.WriteString(".*")
			}
		case c == '*':
			b.WriteString("[^/]*")
		case c == '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	if dir {
		b.WriteString("/.*")
	} else {
		b.WriteString("(?:/.*)?")
	}
	b.WriteString("$")
	re, err := regexp.Compile(b.String())
	if err != nil {
		return Pattern{}, fmt.Errorf("pattern %q: %w", text, err)
	}
	return Pattern{Text: strings.TrimSpace(text), re: re}, nil
}

// CompileAll compiles every pattern, failing on the first bad one.
func CompileAll(texts []string) ([]Pattern, error) {
	out := make([]Pattern, 0, len(texts))
	for _, t := range texts {
		p, err := Compile(t)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// firstMatch returns the first pattern matching path.
func firstMatch(ps []Pattern, path string) (Pattern, bool) {
	for _, p := range ps {
		if p.Match(path) {
			return p, true
		}
	}
	return Pattern{}, false
}
