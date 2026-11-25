package main

// Throwaway unified-diff parser for the spike. Handles `git show` headers (with or
// without lazygit's --stat block), renames/copies, binary files, mode changes,
// new/deleted files and "\ No newline at end of file". Hunk boundaries come from the
// @@ line counts, so a body line such as "--- foo" is never mistaken for a header.

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type Commit struct {
	Hash    string
	Subject string
	Body    string
}

type File struct {
	OldPath, NewPath string
	Status           string // A M D R C T(mode only)
	Similarity       int
	OldMode, NewMode string
	Binary           bool
	Hunks            []*Hunk
}

func (f *File) Path() string {
	if f.NewPath != "" && f.NewPath != "/dev/null" {
		return f.NewPath
	}
	return f.OldPath
}

type Hunk struct {
	ID                 string
	File               *File
	Header             string // the full @@ line
	OldStart, OldLines int
	NewStart, NewLines int
	Lines              []string // body lines including "\ No newline" markers
	Add, Del           int
}

type Diff struct {
	Commits []Commit
	Files   []*File
	Hunks   []*Hunk // all hunks in file order; binary/mode-only files get one pseudo-hunk
}

var hunkRe = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)
var sgrRe = regexp.MustCompile("\x1b\\[[0-9;]*m")

func StripSGR(s string) string { return sgrRe.ReplaceAllString(s, "") }

func atoiDef(s string, d int) int {
	if s == "" {
		return d
	}
	n, _ := strconv.Atoi(s)
	return n
}

// unquote handles git's C-style quoted paths ("a/\303\251.txt").
func unquote(p string) string {
	if len(p) >= 2 && p[0] == '"' && p[len(p)-1] == '"' {
		if u, err := strconv.Unquote(p); err == nil {
			return u
		}
	}
	return p
}

func stripPrefix(p string) string {
	p = unquote(p)
	if strings.HasPrefix(p, "a/") || strings.HasPrefix(p, "b/") {
		return p[2:]
	}
	return p
}

// splitGitLine splits "a/X b/Y" from a "diff --git" line. When X == Y (the common
// case, and the only ambiguous one with spaces) pick the split giving equal halves.
func splitGitLine(rest string) (string, string) {
	if strings.HasPrefix(rest, "\"") {
		if i := strings.Index(rest[1:], "\" "); i >= 0 {
			return stripPrefix(rest[:i+2]), stripPrefix(rest[i+3:])
		}
	}
	if n := len(rest); n%2 == 1 {
		h := n / 2
		if rest[h] == ' ' && rest[2:h] == rest[h+3:] {
			return stripPrefix(rest[:h]), stripPrefix(rest[h+1:])
		}
	}
	if i := strings.Index(rest, " b/"); i >= 0 {
		return stripPrefix(rest[:i]), stripPrefix(rest[i+1:])
	}
	return rest, rest
}

func Parse(text string) (*Diff, error) {
	text = StripSGR(text)
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	d := &Diff{}
	var cur *File
	var hk *Hunk
	remOld, remNew := 0, 0
	var commit *Commit
	inMsg := false
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		// inside a hunk body, counts decide what a line is
		if hk != nil && (remOld > 0 || remNew > 0) {
			switch {
			case strings.HasPrefix(l, "+"):
				remNew--
				hk.Add++
			case strings.HasPrefix(l, "-"):
				remOld--
				hk.Del++
			case strings.HasPrefix(l, " ") || l == "":
				remOld--
				remNew--
			case strings.HasPrefix(l, "\\"):
			default:
				return nil, fmt.Errorf("line %d: unexpected %q inside hunk %s", i+1, l, hk.Header)
			}
			hk.Lines = append(hk.Lines, l)
			continue
		}
		if hk != nil && strings.HasPrefix(l, "\\") { // "\ No newline" after last counted line
			hk.Lines = append(hk.Lines, l)
			continue
		}
		switch {
		case strings.HasPrefix(l, "diff --git "):
			a, b := splitGitLine(l[len("diff --git "):])
			cur = &File{OldPath: a, NewPath: b, Status: "M"}
			d.Files = append(d.Files, cur)
			hk, inMsg = nil, false
		case strings.HasPrefix(l, "diff --cc ") || strings.HasPrefix(l, "diff --combined "):
			return nil, fmt.Errorf("combined (merge) diffs are not supported: %q", l)
		case cur == nil && strings.HasPrefix(l, "commit "):
			f := strings.Fields(l)
			d.Commits = append(d.Commits, Commit{Hash: f[1]})
			commit = &d.Commits[len(d.Commits)-1]
			inMsg = false
		case cur == nil && commit != nil && strings.HasPrefix(l, "    "):
			inMsg = true
			m := strings.TrimPrefix(l, "    ")
			if commit.Subject == "" {
				commit.Subject = m
			} else {
				commit.Body += m + "\n"
			}
		case cur == nil:
			_ = inMsg // preamble: Author/Date/stat block/"---" etc.
		case strings.HasPrefix(l, "@@"):
			m := hunkRe.FindStringSubmatch(l)
			if m == nil {
				return nil, fmt.Errorf("line %d: bad hunk header %q", i+1, l)
			}
			hk = &Hunk{File: cur, Header: l, OldStart: atoiDef(m[1], 0), OldLines: atoiDef(m[2], 1),
				NewStart: atoiDef(m[3], 0), NewLines: atoiDef(m[4], 1)}
			remOld, remNew = hk.OldLines, hk.NewLines
			cur.Hunks = append(cur.Hunks, hk)
		case strings.HasPrefix(l, "new file mode "):
			cur.Status, cur.NewMode = "A", l[len("new file mode "):]
		case strings.HasPrefix(l, "deleted file mode "):
			cur.Status, cur.OldMode = "D", l[len("deleted file mode "):]
		case strings.HasPrefix(l, "old mode "):
			cur.OldMode = l[len("old mode "):]
		case strings.HasPrefix(l, "new mode "):
			cur.NewMode = l[len("new mode "):]
			if cur.Status == "M" {
				cur.Status = "T"
			}
		case strings.HasPrefix(l, "similarity index "):
			cur.Similarity = atoiDef(strings.TrimSuffix(l[len("similarity index "):], "%"), 0)
		case strings.HasPrefix(l, "rename from "):
			cur.Status, cur.OldPath = "R", unquote(l[len("rename from "):])
		case strings.HasPrefix(l, "rename to "):
			cur.NewPath = unquote(l[len("rename to "):])
		case strings.HasPrefix(l, "copy from "):
			cur.Status, cur.OldPath = "C", unquote(l[len("copy from "):])
		case strings.HasPrefix(l, "copy to "):
			cur.NewPath = unquote(l[len("copy to "):])
		case strings.HasPrefix(l, "Binary files ") || l == "GIT binary patch":
			cur.Binary = true
		case strings.HasPrefix(l, "--- "):
			if p := l[4:]; p != "/dev/null" {
				cur.OldPath = stripPrefix(strings.TrimRight(p, "\t"))
			}
		case strings.HasPrefix(l, "+++ "):
			if p := l[4:]; p != "/dev/null" {
				cur.NewPath = stripPrefix(strings.TrimRight(p, "\t"))
			}
		default:
			// index lines, binary patch payload, etc.
		}
	}
	// assign IDs; files without text hunks (binary, pure rename, mode change) get one pseudo-hunk
	n := 0
	for _, f := range d.Files {
		if len(f.Hunks) == 0 {
			desc := map[string]string{"A": "new file", "D": "deleted file", "R": "rename", "C": "copy", "T": "mode change"}[f.Status]
			if f.Binary {
				desc = "binary " + desc
			}
			if f.OldMode != "" && f.NewMode != "" && f.OldMode != f.NewMode {
				desc += fmt.Sprintf(" (mode %s -> %s)", f.OldMode, f.NewMode)
			}
			f.Hunks = []*Hunk{{File: f, Header: "(" + strings.TrimSpace(desc) + ")"}}
		}
		for _, h := range f.Hunks {
			n++
			h.ID = fmt.Sprintf("h%d", n)
			d.Hunks = append(d.Hunks, h)
		}
	}
	return d, nil
}
