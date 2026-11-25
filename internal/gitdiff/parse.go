package gitdiff

// The parser is ported from the spike (spike/groupcli/parse.go). It reads a
// unified diff as git prints it - optionally preceded by a git show / lazygit
// commit header and --stat block - and does not depend on line numbers to find
// hunk boundaries: the "@@" counts do that, so a body line such as
// "--- old rule" is never mistaken for a file header.

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// sgrRe matches ANSI SGR (colour/style) sequences, the only escape sequences
// git and lazygit emit into a patch.
var sgrRe = regexp.MustCompile("\x1b\\[[0-9;]*m")

// hunkRe matches a hunk header: "@@ -a[,b] +c[,d] @@" plus an optional
// context. Omitted counts are 1 (git's rule).
var hunkRe = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

// StripSGR removes ANSI SGR sequences (ESC [ <digits and ;> m) from s.
func StripSGR(s string) string {
	return sgrRe.ReplaceAllString(s, "")
}

// atoiDef parses s as a decimal integer, returning def when s is empty (an
// omitted @@ count) and 0 when it is not a number at all: a malformed count
// makes the hunk fail its counts ("ends early") instead of being accepted, so
// silently reading it as 0 is safe.
func atoiDef(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// unquote decodes one of git's C-style quoted paths ("a/caf\303\251.txt").
// Unquoted paths are returned unchanged, as are quoted ones that do not
// unquote cleanly.
func unquote(p string) string {
	if len(p) >= 2 && p[0] == '"' && p[len(p)-1] == '"' {
		if u, err := strconv.Unquote(p); err == nil {
			return u
		}
	}
	return p
}

// stripPrefix unquotes p and drops git's "a/" or "b/" prefix.
func stripPrefix(p string) string {
	p = unquote(p)
	if strings.HasPrefix(p, "a/") || strings.HasPrefix(p, "b/") {
		return p[2:]
	}
	return p
}

// splitGitLine splits the "a/X b/Y" part of a "diff --git" line into its two
// paths, prefix-stripped. Quoted paths are recognised by their leading quote.
// Otherwise the two halves are equal when the paths are (the common case, and
// the only ambiguous one when a path contains spaces), so an even-length split
// with equal halves wins; failing that the first " b/" marks the boundary.
func splitGitLine(rest string) (oldPath, newPath string) {
	if strings.HasPrefix(rest, `"`) {
		if i := strings.Index(rest[1:], `" `); i >= 0 {
			return stripPrefix(rest[:i+2]), stripPrefix(rest[i+3:])
		}
	}
	if n := len(rest); n%2 == 1 {
		h := n / 2
		if h >= 2 && rest[h] == ' ' && rest[2:h] == rest[h+3:] {
			return stripPrefix(rest[:h]), stripPrefix(rest[h+1:])
		}
	}
	if i := strings.Index(rest, " b/"); i >= 0 {
		return stripPrefix(rest[:i]), stripPrefix(rest[i+1:])
	}
	// Not a shape git produces; treat the whole line as one path.
	return stripPrefix(rest), stripPrefix(rest)
}

// finalizeCommit fills in Subject and Body from the collected message lines of
// c: the first line is the subject, the rest is the body with leading and
// trailing empty lines removed, joined by "\n" and terminated by "\n".
func finalizeCommit(c *Commit, msg []string) {
	if c == nil || len(msg) == 0 {
		return
	}
	c.Subject = msg[0]
	body := msg[1:]
	for len(body) > 0 && body[0] == "" {
		body = body[1:]
	}
	for len(body) > 0 && body[len(body)-1] == "" {
		body = body[:len(body)-1]
	}
	if len(body) > 0 {
		c.Body = strings.Join(body, "\n") + "\n"
	}
}

// Parse parses git's unified diff output. It strips SGR colour first. It
// accepts an optional git show commit header (with or without a --stat block)
// before the first "diff --git" line. Input with no "diff --git" line (for
// example a git log --graph) parses to a Diff with no files and no error.
// A combined merge diff returns an error wrapping ErrCombined.
func Parse(text string) (*Diff, error) {
	text = StripSGR(text)
	lines := strings.Split(text, "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}

	d := &Diff{}
	var (
		cur *File    // file of the current "diff --git" section
		hk  *Hunk    // hunk being read, if any
		com *Commit  // commit of the preamble being read
		msg []string // message lines of com, prefix and quotes removed
	)
	remOld, remNew := 0, 0 // body lines the current hunk still expects
	inMsg := false         // the commit message block has started
	inPayload := false     // skipping a "GIT binary patch" payload

	for i := 0; i < len(lines); i++ {
		l := lines[i]

		// Inside a hunk the counts decide what a line is. This runs before
		// every other rule, so "--- " / "+++ " body lines stay in the hunk.
		if hk != nil && (remOld > 0 || remNew > 0) {
			switch {
			case strings.HasPrefix(l, "+"):
				remNew--
				hk.Add++
			case strings.HasPrefix(l, "-"):
				remOld--
				hk.Del++
			case l == "" || strings.HasPrefix(l, " "):
				remOld--
				remNew--
			case strings.HasPrefix(l, "\\"):
				// "\ No newline at end of file": counts nothing.
			default:
				return nil, fmt.Errorf("line %d: unexpected %q inside hunk %s", i+1, l, hk.Header)
			}
			hk.Lines = append(hk.Lines, l)
			continue
		}
		// A "No newline" marker right after the last counted line still
		// belongs to the hunk.
		if hk != nil && strings.HasPrefix(l, "\\") {
			hk.Lines = append(hk.Lines, l)
			continue
		}

		switch {
		case strings.HasPrefix(l, "diff --cc ") || strings.HasPrefix(l, "diff --combined "):
			return nil, fmt.Errorf("line %d: %w", i+1, ErrCombined)

		case strings.HasPrefix(l, "diff --git "):
			a, b := splitGitLine(l[len("diff --git "):])
			cur = &File{OldPath: a, NewPath: b, Status: Modified}
			d.Files = append(d.Files, cur)
			hk, inMsg, inPayload = nil, false, false

		case inPayload:
			// base85 payload of "GIT binary patch": skipped until the next
			// "diff --git" line.

		case cur == nil && strings.HasPrefix(l, "commit "):
			finalizeCommit(com, msg)
			msg, inMsg = nil, false
			if f := strings.Fields(l); len(f) >= 2 {
				d.Commits = append(d.Commits, Commit{Hash: f[1]})
				com = &d.Commits[len(d.Commits)-1]
			} else {
				com = nil
			}

		case cur == nil && com != nil && !inMsg && strings.HasPrefix(l, "Merge:"):
			com.Merge = true

		case cur == nil && com != nil && strings.HasPrefix(l, "    "):
			inMsg = true
			msg = append(msg, strings.TrimPrefix(l, "    "))

		case cur == nil && com != nil && inMsg && l == "":
			// An empty line between message lines is part of the message.
			msg = append(msg, "")

		case cur == nil:
			// The rest of the preamble: Author:, Date:, "---", the --stat
			// block, "N files changed", lazygit's log decoration.

		case strings.HasPrefix(l, "@@"):
			m := hunkRe.FindStringSubmatch(l)
			if m == nil {
				return nil, fmt.Errorf("line %d: bad hunk header %q", i+1, l)
			}
			hk = &Hunk{
				File:     cur,
				FileIdx:  len(d.Files) - 1,
				Header:   l,
				OldStart: atoiDef(m[1], 0),
				OldLines: atoiDef(m[2], 1),
				NewStart: atoiDef(m[3], 0),
				NewLines: atoiDef(m[4], 1),
			}
			remOld, remNew = hk.OldLines, hk.NewLines
			cur.Hunks = append(cur.Hunks, hk)

		case strings.HasPrefix(l, "new file mode "):
			cur.Status = Added
			cur.NewMode = l[len("new file mode "):]
			cur.OldPath = ""

		case strings.HasPrefix(l, "deleted file mode "):
			cur.Status = Deleted
			cur.OldMode = l[len("deleted file mode "):]
			cur.NewPath = ""

		case strings.HasPrefix(l, "old mode "):
			cur.OldMode = l[len("old mode "):]

		case strings.HasPrefix(l, "new mode "):
			cur.NewMode = l[len("new mode "):]
			if cur.Status == Modified {
				cur.Status = ModeOnly
			}

		case strings.HasPrefix(l, "similarity index "):
			v := l[len("similarity index "):]
			cur.Similarity = atoiDef(strings.TrimSuffix(v, "%"), 0)

		case strings.HasPrefix(l, "rename from "):
			cur.Status = Renamed
			cur.OldPath = unquote(l[len("rename from "):])

		case strings.HasPrefix(l, "rename to "):
			cur.Status = Renamed
			cur.NewPath = unquote(l[len("rename to "):])

		case strings.HasPrefix(l, "copy from "):
			cur.Status = Copied
			cur.OldPath = unquote(l[len("copy from "):])

		case strings.HasPrefix(l, "copy to "):
			cur.Status = Copied
			cur.NewPath = unquote(l[len("copy to "):])

		case strings.HasPrefix(l, "index "):
			cur.Index = l[len("index "):]

		case strings.HasPrefix(l, "Binary files ") || l == "GIT binary patch":
			cur.Binary = true
			inPayload = l == "GIT binary patch"

		case strings.HasPrefix(l, "--- "):
			// git appends a TAB after a path with spaces; "/dev/null" leaves
			// the path as the status rules above set it.
			if p := strings.TrimRight(l[len("--- "):], "\t"); p != "/dev/null" {
				cur.OldPath = stripPrefix(p)
			}

		case strings.HasPrefix(l, "+++ "):
			if p := strings.TrimRight(l[len("+++ "):], "\t"); p != "/dev/null" {
				cur.NewPath = stripPrefix(p)
			}

		default:
			// "index" lines, mode lines of other kinds, anything unknown.
		}
	}
	finalizeCommit(com, msg)
	if hk != nil && (remOld > 0 || remNew > 0) {
		return nil, fmt.Errorf("line %d: hunk %s ends early", len(lines)+1, hk.Header)
	}

	// Number the hunks across the whole diff and link them to their file. A
	// file with no text hunk (binary, pure rename or copy, mode-only, empty
	// new or deleted file) gets one pseudo-hunk describing it.
	for idx, f := range d.Files {
		if len(f.Hunks) == 0 {
			f.Hunks = []*Hunk{{File: f, FileIdx: idx, Header: PseudoHeader(f), Pseudo: true}}
		}
		for _, h := range f.Hunks {
			h.Index = len(d.Hunks)
			h.File = f
			h.FileIdx = idx
			h.ID = "h" + strconv.Itoa(h.Index+1)
			d.Hunks = append(d.Hunks, h)
		}
	}
	return d, nil
}
