package gitdiff

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var sgrRe = regexp.MustCompile("\x1b\\[[0-9;]*m")

var hunkRe = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

func StripSGR(s string) string {
	return sgrRe.ReplaceAllString(s, "")
}

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
	return stripPrefix(rest), stripPrefix(rest)
}

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

func Parse(text string) (*Diff, error) {
	text = StripSGR(text)
	lines := strings.Split(text, "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}

	d := &Diff{}
	var (
		cur *File
		hk  *Hunk
		com *Commit
		msg []string
	)
	remOld, remNew := 0, 0
	inMsg := false
	inPayload := false

	for i := 0; i < len(lines); i++ {
		l := lines[i]

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
			default:
				return nil, fmt.Errorf("line %d: unexpected %q inside hunk %s", i+1, l, hk.Header)
			}
			hk.Lines = append(hk.Lines, l)
			continue
		}
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
			msg = append(msg, "")

		case cur == nil:

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
			if p := strings.TrimRight(l[len("--- "):], "\t"); p != "/dev/null" {
				cur.OldPath = stripPrefix(p)
			}

		case strings.HasPrefix(l, "+++ "):
			if p := strings.TrimRight(l[len("+++ "):], "\t"); p != "/dev/null" {
				cur.NewPath = stripPrefix(p)
			}

		default:
		}
	}
	finalizeCommit(com, msg)
	if hk != nil && (remOld > 0 || remNew > 0) {
		return nil, fmt.Errorf("line %d: hunk %s ends early", len(lines)+1, hk.Header)
	}

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
