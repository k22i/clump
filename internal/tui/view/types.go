// Package view holds the types and small helpers shared by clump's TUI
// render packages: the Theme, the rendered Line model, list rows and help
// entries.
package view

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// Theme holds every style the render functions use. PlainTheme renders no
// escape sequences at all (golden tests compare plain text).
type Theme struct {
	// Plain disables all styling, including syntax highlighting.
	Plain bool
	// Dark selects dark-background colours and the dark Chroma style.
	Dark bool

	Title      lipgloss.Style // header line, group titles in the diff pane
	Dim        lipgloss.Style // secondary text, dimmed (unfollowed) results
	Selected   lipgloss.Style // the cursor row in the group list
	Marked     lipgloss.Style // hunk marked with x
	FileHeader lipgloss.Style // "── path ──" lines
	HunkHeader lipgloss.Style // "@@ ... @@" lines
	Add        lipgloss.Style // "+" lines (base colour; syntax colours go on top)
	Del        lipgloss.Style // "-" lines
	Context    lipgloss.Style // " " lines
	AddWord    lipgloss.Style // changed words inside a paired "+" line
	DelWord    lipgloss.Style // changed words inside a paired "-" line
	GroupLabel lipgloss.Style // "[g2 Title]" labels and boundaries in the raw view
	Match      lipgloss.Style // search matches
	Notice     lipgloss.Style // the notices bar
	Error      lipgloss.Style // errors
	Key        lipgloss.Style // key names in the help overlay
	Border     lipgloss.Style // separators

	// ChromaStyle is the Chroma style name for syntax colours
	// ("github-dark" when Dark, else "github"); unused when Plain.
	ChromaStyle string
}

// PlainTheme is the theme with no styling.
func PlainTheme() Theme {
	return Theme{Plain: true}
}

// NewTheme returns the coloured theme for a dark or light background.
func NewTheme(dark bool) Theme {
	c := func(darkHex, lightHex string) color.Color {
		if dark {
			return lipgloss.Color(darkHex)
		}
		return lipgloss.Color(lightHex)
	}
	s := lipgloss.NewStyle
	t := Theme{Dark: dark, ChromaStyle: "github"}
	if dark {
		t.ChromaStyle = "github-dark"
	}
	t.Title = s().Bold(true)
	t.Dim = s().Faint(true)
	t.Selected = s().Reverse(true)
	t.Marked = s().Foreground(c("#e5c07b", "#986801")).Bold(true)
	t.FileHeader = s().Foreground(c("#61afef", "#0550ae")).Bold(true)
	t.HunkHeader = s().Foreground(c("#56b6c2", "#0e7490"))
	t.Add = s().Foreground(c("#98c379", "#116329"))
	t.Del = s().Foreground(c("#e06c75", "#a40e26"))
	t.Context = s()
	t.AddWord = s().Background(c("#2d4a2b", "#ccffd8"))
	t.DelWord = s().Background(c("#5a2a2e", "#ffd7d5"))
	t.GroupLabel = s().Foreground(c("#c678dd", "#8250df")).Bold(true)
	t.Match = s().Reverse(true).Bold(true)
	t.Notice = s().Foreground(c("#e5c07b", "#986801"))
	t.Error = s().Foreground(c("#e06c75", "#a40e26")).Bold(true)
	t.Key = s().Foreground(c("#61afef", "#0550ae")).Bold(true)
	t.Border = s().Faint(true)
	return t
}

// LineKind says what a rendered diff line is.
type LineKind int

// Line kinds.
const (
	KindBlank     LineKind = iota // an empty spacer line
	KindFile                      // file header
	KindHunk                      // "@@" header or pseudo-hunk description
	KindAdd                       // "+" body line
	KindDel                       // "-" body line
	KindContext                   // " " body line
	KindNoNewline                 // "\ No newline at end of file"
	KindBoundary                  // raw view: a group boundary marker
	KindGroup                     // diff pane: the group's title/description block
)

// Line is one rendered line of a diff view.
type Line struct {
	// Text is the styled text (equal to Plain under PlainTheme). It never
	// contains a newline and is at most the requested width wide.
	Text string
	// Plain is the same text without styling; search runs over it.
	Plain string
	Kind  LineKind
	// HunkID is the hunk the line belongs to ("" for file headers, blanks,
	// boundaries and group blocks).
	HunkID string
	// FileIdx is the file's index in Diff.Files, or -1.
	FileIdx int
	// OldLine and NewLine are the 1-based line numbers in the old and new
	// file (0 when the line has none on that side). For a KindHunk line
	// they are the hunk's OldStart/NewStart.
	OldLine, NewLine int
}

// ListRow is one row of the group list.
type ListRow struct {
	ID       string
	Title    string
	Add, Del int
	Hunks    int
	// Marks is how many of the group's hunks are marked with x.
	Marks int
	// Ungrouped is set for the Ungrouped group (rendered last-ish, italic).
	Ungrouped bool
}

// HelpEntry is one key in the help overlay.
type HelpEntry struct {
	Keys string // e.g. "j/k", "ctrl+r"
	Desc string
}

// HelpSection is a titled block of keys.
type HelpSection struct {
	Title   string
	Entries []HelpEntry
}
