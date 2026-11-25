// Package gitdiff parses unified diffs produced by git (git show, git diff,
// git stash show, with or without lazygit's --stat block and colour) and
// loads diffs from a repository by shelling out to git.
//
// The same parser serves the one-shot, watch and push paths.
package gitdiff

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// Status is a file's change kind, as one upper-case letter.
type Status string

// File statuses.
const (
	Added    Status = "A"
	Modified Status = "M"
	Deleted  Status = "D"
	Renamed  Status = "R"
	Copied   Status = "C"
	ModeOnly Status = "T" // only the file mode changed
)

// ErrCombined is returned (wrapped) by Parse when the input contains a
// combined merge diff ("diff --cc" or "diff --combined"). Callers re-load the
// commit with LoadRev, which diffs a merge against its first parent.
var ErrCombined = errors.New("combined (merge) diff")

// Commit is one "commit <sha>" header block from git show / git log output.
type Commit struct {
	Hash    string `json:"hash"`    // full sha as printed after "commit "
	Subject string `json:"subject"` // first indented message line
	Body    string `json:"body"`    // remaining indented message lines, each followed by "\n"
	// Merge is true when the header has a "Merge:" line.
	Merge bool `json:"merge,omitempty"`
}

// File is one "diff --git" section.
type File struct {
	OldPath    string `json:"old_path"` // path before the change ("" when Added)
	NewPath    string `json:"new_path"` // path after the change ("" when Deleted)
	Status     Status `json:"status"`
	Similarity int    `json:"similarity,omitempty"` // "similarity index N%" for R and C
	OldMode    string `json:"old_mode,omitempty"`
	NewMode    string `json:"new_mode,omitempty"`
	Binary     bool   `json:"binary,omitempty"`
	// Index is the value of the "index" line ("abc123..def456 100644"), "" if absent.
	Index string `json:"index,omitempty"`
	// Generated, when not "", says why the file is skipped from review
	// (set by generated.Detector.Mark, never by the parser): e.g.
	// "pattern go.sum", "linguist-generated", "marker @generated". Its
	// hunks are not sent to the model. It is not part of Canonical.
	Generated string  `json:"generated,omitempty"`
	Hunks     []*Hunk `json:"-"`
}

// Path is the file's current path: NewPath, or OldPath for a deleted file.
func (f *File) Path() string {
	if f.NewPath != "" {
		return f.NewPath
	}
	return f.OldPath
}

// DisplayPath is Path, or "old -> new" for renames and copies.
func (f *File) DisplayPath() string {
	if (f.Status == Renamed || f.Status == Copied) && f.OldPath != "" && f.OldPath != f.NewPath {
		return f.OldPath + " -> " + f.NewPath
	}
	return f.Path()
}

// Hunk is one "@@" hunk, or a pseudo-hunk standing in for a file that has no
// text hunks (binary, pure rename or copy, mode-only, empty new/deleted file).
type Hunk struct {
	ID      string `json:"id"`         // "h1".."hN" in file order across the whole diff
	Index   int    `json:"index"`      // 0-based position in Diff.Hunks
	FileIdx int    `json:"file_index"` // 0-based position of File in Diff.Files
	File    *File  `json:"-"`
	// Header is the full "@@ ... @@ context" line, or for a pseudo-hunk one of
	// (see PseudoHeader): "(binary new file)", "(mode change 100644 -> 100755)", ...
	Header   string `json:"header"`
	OldStart int    `json:"old_start"`
	OldLines int    `json:"old_lines"`
	NewStart int    `json:"new_start"`
	NewLines int    `json:"new_lines"`
	// Lines is the hunk body: lines starting with ' ', '+', '-' or '\'
	// ("\ No newline at end of file"), without trailing "\n".
	Lines  []string `json:"lines,omitempty"`
	Add    int      `json:"add"`
	Del    int      `json:"del"`
	Pseudo bool     `json:"pseudo,omitempty"`
}

// PseudoHeader is the Header of a file's pseudo-hunk:
//
//	"(" + desc + modes + ")"
//
// desc by status, prefixed with "binary " when f.Binary:
// A "new file", D "deleted file", R "rename", C "copy", T "mode change",
// M "change" when binary, else "no text changes".
// modes, when OldMode and NewMode are both set and differ: " OLD -> NEW" for
// status T, ", mode OLD -> NEW" otherwise. Examples: "(binary new file)",
// "(mode change 100644 -> 100755)", "(rename, mode 100644 -> 100755)",
// "(binary change)", "(new file)".
func PseudoHeader(f *File) string {
	desc := "no text changes"
	switch f.Status {
	case Added:
		desc = "new file"
	case Deleted:
		desc = "deleted file"
	case Renamed:
		desc = "rename"
	case Copied:
		desc = "copy"
	case ModeOnly:
		desc = "mode change"
	}
	if f.Binary {
		if f.Status == Added || f.Status == Deleted || f.Status == Renamed ||
			f.Status == Copied || f.Status == ModeOnly {
			desc = "binary " + desc
		} else {
			desc = "binary change"
		}
	}
	modes := ""
	if f.OldMode != "" && f.NewMode != "" && f.OldMode != f.NewMode {
		if f.Status == ModeOnly {
			modes = fmt.Sprintf(" %s -> %s", f.OldMode, f.NewMode)
		} else {
			modes = fmt.Sprintf(", mode %s -> %s", f.OldMode, f.NewMode)
		}
	}
	return "(" + desc + modes + ")"
}

// Diff is a parsed patch.
type Diff struct {
	Commits []Commit `json:"commits,omitempty"`
	Files   []*File  `json:"files"`
	Hunks   []*Hunk  `json:"hunks"` // every hunk in file order; Hunks[i].Index == i
}

// Hunk returns the hunk with the given ID, or nil.
func (d *Diff) Hunk(id string) *Hunk {
	for _, h := range d.Hunks {
		if h.ID == id {
			return h
		}
	}
	return nil
}

// Stats returns the total added and deleted line counts.
func (d *Diff) Stats() (add, del int) {
	for _, h := range d.Hunks {
		add += h.Add
		del += h.Del
	}
	return add, del
}

// Canonical renders the diff in clump's canonical text form, which ignores
// everything that does not change the patch itself: colour, the git show
// commit header, the --stat block, "index" lines of text files, and git's
// path quoting. For each file, in order:
//
//	diff --git a/<OldPath or NewPath> b/<NewPath or OldPath>
//	status <Status> <Similarity>
//	mode <OldMode> <NewMode>
//	binary <Index>            (only when Binary)
//	<Header>                  (each non-pseudo hunk, followed by its Lines)
//	<Lines...>
//
// Every line, including the last, ends in "\n". Paths are written unquoted.
// The one-shot loaders and lazygit's output for the same change therefore
// produce the same Canonical text and the same Hash.
func (d *Diff) Canonical() string {
	var b strings.Builder
	for _, f := range d.Files {
		b.WriteString(fileCanonical(f))
	}
	return b.String()
}

// fileCanonical renders one file's section of Canonical.
func fileCanonical(f *File) string {
	oldPath, newPath := f.OldPath, f.NewPath
	if oldPath == "" {
		oldPath = f.NewPath
	}
	if newPath == "" {
		newPath = f.OldPath
	}
	var b strings.Builder
	fmt.Fprintf(&b, "diff --git a/%s b/%s\n", oldPath, newPath)
	fmt.Fprintf(&b, "status %s %d\n", f.Status, f.Similarity)
	fmt.Fprintf(&b, "mode %s %s\n", f.OldMode, f.NewMode)
	if f.Binary {
		fmt.Fprintf(&b, "binary %s\n", f.Index)
	}
	for _, h := range f.Hunks {
		if h.Pseudo {
			continue
		}
		b.WriteString(h.Header)
		b.WriteString("\n")
		for _, l := range h.Lines {
			b.WriteString(l)
			b.WriteString("\n")
		}
	}
	return b.String()
}

// sha256Hex is the lower-case hex SHA-256 of s.
func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// Hash is the lower-case hex SHA-256 of Canonical(). It is the diff hash used
// for cache keys and overrides.
func (d *Diff) Hash() string {
	return sha256Hex(d.Canonical())
}

// FileHash is the lower-case hex SHA-256 of one file's section of
// Canonical(). It keys per-file summaries.
func (d *Diff) FileHash(fileIdx int) string {
	if fileIdx < 0 || fileIdx >= len(d.Files) {
		return ""
	}
	return sha256Hex(fileCanonical(d.Files[fileIdx]))
}
