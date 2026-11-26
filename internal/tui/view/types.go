package view

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

type Theme struct {
	Plain bool
	Dark  bool

	Title      lipgloss.Style
	Dim        lipgloss.Style
	Selected   lipgloss.Style
	Marked     lipgloss.Style
	FileHeader lipgloss.Style
	HunkHeader lipgloss.Style
	Add        lipgloss.Style
	Del        lipgloss.Style
	Context    lipgloss.Style
	AddLine    lipgloss.Style
	DelLine    lipgloss.Style
	AddWord    lipgloss.Style
	DelWord    lipgloss.Style
	GroupLabel lipgloss.Style
	Match      lipgloss.Style
	Notice     lipgloss.Style
	Error      lipgloss.Style
	Key        lipgloss.Style
	Border     lipgloss.Style

	ChromaStyle string
}

func PlainTheme() Theme {
	return Theme{Plain: true}
}

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
	t.AddLine = s().Background(c("#17301f", "#e6ffec"))
	t.DelLine = s().Background(c("#3b1c21", "#ffebe9"))
	t.AddWord = s().Background(c("#2b5c38", "#abf2bc"))
	t.DelWord = s().Background(c("#73303a", "#ffc1bd"))
	t.GroupLabel = s().Foreground(c("#c678dd", "#8250df")).Bold(true)
	t.Match = s().Reverse(true).Bold(true)
	t.Notice = s().Foreground(c("#e5c07b", "#986801"))
	t.Error = s().Foreground(c("#e06c75", "#a40e26")).Bold(true)
	t.Key = s().Foreground(c("#61afef", "#0550ae")).Bold(true)
	t.Border = s().Faint(true)
	return t
}

type LineKind int

const (
	KindBlank LineKind = iota
	KindFile
	KindHunk
	KindAdd
	KindDel
	KindContext
	KindNoNewline
	KindBoundary
	KindGroup
	KindComment
)

type Line struct {
	Text             string
	Plain            string
	Kind             LineKind
	HunkID           string
	FileIdx          int
	OldLine, NewLine int
	Gutter           int
	BlockHunks       []string
	Folded           bool
}

type FileState struct {
	Folded    bool
	Reviewed  bool
	Generated string
}

type ListRow struct {
	ID        string
	Title     string
	Add, Del  int
	Hunks     int
	Marks     int
	Ungrouped bool
	Reviewed  int
}

type HelpEntry struct {
	Keys string
	Desc string
}

type HelpSection struct {
	Title   string
	Entries []HelpEntry
}
