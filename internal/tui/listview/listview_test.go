package listview

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/k22i/clump/internal/gitdiff"
	"github.com/k22i/clump/internal/tui/view"
	"github.com/k22i/clump/internal/tui/view/viewtest"
)

// basicRows is the fixture for the list goldens: five rows, one of them the
// Ungrouped group and one carrying marks.
func basicRows() []view.ListRow {
	return []view.ListRow{
		{ID: "g1", Title: "Parser hardening", Add: 12, Del: 3, Hunks: 2},
		{ID: "g2", Title: "TUI group list", Add: 48, Del: 9, Hunks: 4, Marks: 2},
		{ID: "g3", Title: "Docs", Add: 1, Del: 0, Hunks: 1},
		{ID: "g4", Title: "Rename provider config", Add: 0, Del: 0, Hunks: 1},
		{ID: "ungrouped", Title: "Ungrouped", Add: 7, Del: 7, Hunks: 3, Ungrouped: true},
	}
}

func TestWindow(t *testing.T) {
	cases := []struct {
		name               string
		n, cursor, height  int
		wantStart, wantEnd int
	}{
		{"all rows fit", 5, 3, 10, 0, 5},
		{"top page", 30, 0, 8, 0, 8},
		{"middle", 30, 25, 8, 21, 29},
		{"last row", 30, 29, 8, 22, 30},
		{"negative cursor", 30, -3, 8, 0, 8},
		{"cursor past the end", 30, 99, 8, 22, 30},
		{"zero height", 30, 4, 0, 0, 0},
		{"negative height", 30, 4, -2, 0, 0},
		{"no rows", 0, 0, 8, 0, 0},
		{"exactly height", 8, 7, 8, 0, 8},
		{"one more than height", 9, 8, 8, 1, 9},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			start, end := Window(c.n, c.cursor, c.height)
			if start != c.wantStart || end != c.wantEnd {
				t.Fatalf("Window(%d, %d, %d) = (%d, %d), want (%d, %d)",
					c.n, c.cursor, c.height, start, end, c.wantStart, c.wantEnd)
			}
		})
	}
}

func TestGoldenList(t *testing.T) {
	const width, height = 40, 10
	got := Render(basicRows(), 1, width, height, view.PlainTheme())
	viewtest.Golden(t, "ls-basic", got+"\n")

	lines := strings.Split(got, "\n")
	if len(lines) != len(basicRows()) {
		t.Fatalf("got %d lines, want %d", len(lines), len(basicRows()))
	}
	for i, l := range lines {
		if w := lipgloss.Width(l); w != width {
			t.Errorf("line %d is %d cells wide, want %d: %q", i, w, width, l)
		}
	}
	if !strings.HasPrefix(lines[1], cursorMark) {
		t.Errorf("cursor row = %q, want it to start with %q", lines[1], cursorMark)
	}
	if !strings.Contains(lines[1], " x2") {
		t.Errorf("cursor row = %q, want the x2 marks suffix", lines[1])
	}
}

func TestGoldenListScroll(t *testing.T) {
	rows := make([]view.ListRow, 30)
	for i := range rows {
		rows[i] = view.ListRow{
			ID:    fmt.Sprintf("g%d", i+1),
			Title: fmt.Sprintf("Group %02d", i+1),
			Add:   i,
			Del:   i / 2,
			Hunks: i%3 + 1,
		}
	}
	got := Render(rows, 25, 32, 8, view.PlainTheme())
	viewtest.Golden(t, "ls-scroll", got+"\n")

	lines := strings.Split(got, "\n")
	if len(lines) != 8 {
		t.Fatalf("got %d lines, want 8: %q", len(lines), got)
	}
	for i, l := range lines {
		if want := fmt.Sprintf("Group %02d", i+22); !strings.Contains(l, want) {
			t.Errorf("line %d = %q, want it to show %q", i, l, want)
		}
	}
	if !strings.HasPrefix(lines[4], cursorMark) {
		t.Errorf("cursor row = %q, want it to start with %q", lines[4], cursorMark)
	}
}

func TestListNarrow(t *testing.T) {
	rows := []view.ListRow{
		{ID: "g1", Title: "A very long group title that cannot fit", Add: 1234, Del: 567, Hunks: 42},
		{ID: "g2", Title: "Short", Add: 1, Del: 1, Hunks: 1},
	}
	const width = 12
	got := Render(rows, 0, width, 5, view.PlainTheme())
	lines := strings.Split(got, "\n")
	if len(lines) != len(rows) {
		t.Fatalf("got %d lines, want %d", len(lines), len(rows))
	}
	for i, l := range lines {
		if w := lipgloss.Width(l); w > width {
			t.Errorf("line %d is %d cells wide, want at most %d: %q", i, w, width, l)
		}
		if l == "" {
			t.Errorf("line %d is empty; a narrow row must still show something", i)
		}
		// A row too narrow for the counts is the whole line truncated; a row
		// with room for one title cell keeps the counts flush right.
		prefix := "  "
		if i == 0 {
			prefix = cursorMark
		}
		r := rows[i]
		right := fmt.Sprintf(" +%d −%d %dh", r.Add, r.Del, r.Hunks)
		want := ""
		if room := width - 2 - lipgloss.Width(right); room >= 1 {
			title := view.Truncate(r.Title, room)
			want = prefix + title + strings.Repeat(" ", room-lipgloss.Width(title)) + right
		} else {
			want = view.Truncate(prefix+r.Title+right, width)
		}
		if l != want {
			t.Errorf("line %d = %q, want %q", i, l, want)
		}
	}
	if !strings.Contains(lines[0], cursorMark) {
		t.Errorf("cursor row = %q, want it to contain %q", lines[0], cursorMark)
	}
}

func TestListLongTitle(t *testing.T) {
	title := strings.Repeat("x", 100)
	row := view.ListRow{ID: "g1", Title: title, Add: 3, Del: 1, Hunks: 2}
	got := Render([]view.ListRow{row}, 0, 40, 10, view.PlainTheme())
	if strings.Contains(got, "\n") {
		t.Fatalf("one row rendered as %q, want a single line", got)
	}
	if !strings.HasSuffix(got, "… +3 −1 2h") {
		t.Fatalf("row = %q, want it to end with %q", got, "… +3 −1 2h")
	}
	if w := lipgloss.Width(got); w != 40 {
		t.Fatalf("row is %d cells wide, want exactly 40", w)
	}
}

func TestListColoured(t *testing.T) {
	const width, height = 40, 10
	rows := basicRows()
	plain := Render(rows, 1, width, height, view.PlainTheme())
	coloured := Render(rows, 1, width, height, view.NewTheme(false))
	if len(plain) == 0 || coloured == "" {
		t.Fatal("empty render")
	}
	plainLines := strings.Split(plain, "\n")
	colouredLines := strings.Split(coloured, "\n")
	if len(colouredLines) != len(plainLines) {
		t.Fatalf("coloured render has %d lines, plain has %d", len(colouredLines), len(plainLines))
	}
	for i := range plainLines {
		if got := gitdiff.StripSGR(colouredLines[i]); got != plainLines[i] {
			t.Errorf("line %d: StripSGR(coloured) = %q, want %q", i, got, plainLines[i])
		}
	}
	if !strings.Contains(colouredLines[1], "\x1b[") {
		t.Errorf("cursor row %q is not styled", colouredLines[1])
	}

	// A narrow row must strip back to its plain text too: the parts are cut
	// separately before styling, so the escape codes never split a rune.
	narrowPlain := Render(rows, 0, 12, 5, view.PlainTheme())
	narrowColoured := Render(rows, 0, 12, 5, view.NewTheme(false))
	np := strings.Split(narrowPlain, "\n")
	nc := strings.Split(narrowColoured, "\n")
	if len(np) != len(nc) {
		t.Fatalf("narrow coloured render has %d lines, plain has %d", len(nc), len(np))
	}
	for i := range np {
		if got := gitdiff.StripSGR(nc[i]); got != np[i] {
			t.Errorf("narrow line %d: StripSGR(coloured) = %q, want %q", i, got, np[i])
		}
	}
}

func TestListEmpty(t *testing.T) {
	cases := []struct {
		name   string
		rows   []view.ListRow
		height int
	}{
		{"no rows", nil, 10},
		{"zero height", basicRows(), 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Render(c.rows, 0, 40, c.height, view.PlainTheme()); got != "" {
				t.Fatalf("Render = %q, want \"\"", got)
			}
		})
	}
}
