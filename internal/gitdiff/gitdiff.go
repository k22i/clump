package gitdiff

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

type Status string

const (
	Added    Status = "A"
	Modified Status = "M"
	Deleted  Status = "D"
	Renamed  Status = "R"
	Copied   Status = "C"
	ModeOnly Status = "T"
)

var ErrCombined = errors.New("combined (merge) diff")

type Commit struct {
	Hash    string `json:"hash"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
	Merge   bool   `json:"merge,omitempty"`
}

type File struct {
	OldPath    string  `json:"old_path"`
	NewPath    string  `json:"new_path"`
	Status     Status  `json:"status"`
	Similarity int     `json:"similarity,omitempty"`
	OldMode    string  `json:"old_mode,omitempty"`
	NewMode    string  `json:"new_mode,omitempty"`
	Binary     bool    `json:"binary,omitempty"`
	Index      string  `json:"index,omitempty"`
	Generated  string  `json:"generated,omitempty"`
	Hunks      []*Hunk `json:"-"`
}

func (f *File) Path() string {
	if f.NewPath != "" {
		return f.NewPath
	}
	return f.OldPath
}

func (f *File) DisplayPath() string {
	if (f.Status == Renamed || f.Status == Copied) && f.OldPath != "" && f.OldPath != f.NewPath {
		return f.OldPath + " -> " + f.NewPath
	}
	return f.Path()
}

type Hunk struct {
	ID       string   `json:"id"`
	Index    int      `json:"index"`
	FileIdx  int      `json:"file_index"`
	File     *File    `json:"-"`
	Header   string   `json:"header"`
	OldStart int      `json:"old_start"`
	OldLines int      `json:"old_lines"`
	NewStart int      `json:"new_start"`
	NewLines int      `json:"new_lines"`
	Lines    []string `json:"lines,omitempty"`
	Add      int      `json:"add"`
	Del      int      `json:"del"`
	Pseudo   bool     `json:"pseudo,omitempty"`
}

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

func (h *Hunk) ReviewKey() string {
	var b strings.Builder
	if h.File != nil {
		b.WriteString(h.File.DisplayPath())
	}
	b.WriteByte(0)
	if h.Pseudo {
		b.WriteString(h.Header)
	}
	for _, l := range h.Lines {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	return sha256Hex(b.String())
}

type Diff struct {
	Commits []Commit `json:"commits,omitempty"`
	Files   []*File  `json:"files"`
	Hunks   []*Hunk  `json:"hunks"`
}

func (d *Diff) Hunk(id string) *Hunk {
	for _, h := range d.Hunks {
		if h.ID == id {
			return h
		}
	}
	return nil
}

func (d *Diff) Stats() (add, del int) {
	for _, h := range d.Hunks {
		add += h.Add
		del += h.Del
	}
	return add, del
}

func (d *Diff) Canonical() string {
	var b strings.Builder
	for _, f := range d.Files {
		b.WriteString(fileCanonical(f))
	}
	return b.String()
}

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

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func (d *Diff) Hash() string {
	return sha256Hex(d.Canonical())
}

func (d *Diff) FileHash(fileIdx int) string {
	if fileIdx < 0 || fileIdx >= len(d.Files) {
		return ""
	}
	return sha256Hex(fileCanonical(d.Files[fileIdx]))
}
