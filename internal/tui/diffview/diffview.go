// Package diffview renders hunks for clump's diff pane: syntax highlighting
// with Chroma by file extension, and word-level highlighting of paired -/+
// lines.
package diffview

import (
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"

	"github.com/k22i/clump/internal/chromalexers"
	"github.com/k22i/clump/internal/gitdiff"
	"github.com/k22i/clump/internal/tui/view"
)

// tabWidth is the tab stop used when expanding tabs in hunk bodies.
const tabWidth = 4

// ellipsisMark ends a line that view.Truncate had to cut.
const ellipsisMark = "…"

// Limits for WordDiff: longer lines or token lists are not worth diffing.
const (
	maxWordTokens    = 200
	maxWordBytes     = 1000
	maxWordChangedPc = 60
)

// HunkOpts configures RenderHunk.
type HunkOpts struct {
	// Width caps every line (<= 0 means no cap).
	Width int
	// Marked draws the hunk's gutter as marked ("x ").
	Marked bool
	// Path picks the syntax lexer by file name ("" disables highlighting).
	Path string
	// Numbers is the digit width of the old/new line-number columns drawn
	// after the mark gutter; 0 draws none. See NumberWidth.
	Numbers int
}

// NumberWidth is the digit width that fits every line number of d (at
// least 2), for HunkOpts.Numbers.
func NumberWidth(d *gitdiff.Diff) int {
	top := 0
	if d != nil {
		for _, h := range d.Hunks {
			top = max(top, h.OldStart+h.OldLines, h.NewStart+h.NewLines)
		}
	}
	w := 2
	for n := 100; n <= top; n *= 10 {
		w++
	}
	return w
}

// numberCols renders the line-number columns: "old new " right-aligned in
// w digits each, blank on the side a line does not have.
func numberCols(w, oldLine, newLine int) string {
	col := func(n int) string {
		if n <= 0 {
			return strings.Repeat(" ", w)
		}
		s := strconv.Itoa(n)
		if len(s) < w {
			s = strings.Repeat(" ", w-len(s)) + s
		}
		return s
	}
	return col(oldLine) + " " + col(newLine) + " "
}

// RenderHunk renders one hunk: its header line, then its body lines.
func RenderHunk(h *gitdiff.Hunk, th view.Theme, o HunkOpts) []view.Line {
	if h == nil {
		return []view.Line{}
	}
	gutter, gutterStyles := "  ", []lipgloss.Style(nil)
	if o.Marked {
		gutter, gutterStyles = "x ", themeStyles(th, th.Marked)
	}

	// lead is the mark gutter plus, when on, the line-number columns.
	lead := func(oldLine, newLine int) []segment {
		segs := []segment{{text: gutter, styles: gutterStyles}}
		if o.Numbers > 0 {
			segs = append(segs, segment{text: numberCols(o.Numbers, oldLine, newLine), styles: themeStyles(th, th.Dim)})
		}
		return segs
	}
	gutterLen := len(gutter)
	if o.Numbers > 0 {
		gutterLen += 2*o.Numbers + 2
	}

	out := make([]view.Line, 0, len(h.Lines)+1)
	// The header starts at the line-number column: it carries the numbers.
	out = append(out, withGutter(renderLine([]segment{
		{text: gutter, styles: gutterStyles},
		{text: h.Header, styles: themeStyles(th, th.HunkHeader)},
	}, o.Width, view.KindHunk, h.ID, h.FileIdx, h.OldStart, h.NewStart), len(gutter)))
	if h.Pseudo {
		return out
	}

	var hl *highlighter
	if !th.Plain && o.Path != "" {
		hl = newHighlighter(th, o.Path)
	}
	delSpans, addSpans := wordSpans(h.Lines, th)

	oldNo, newNo := h.OldStart, h.NewStart
	for i, raw := range h.Lines {
		// The prefix decides what the line is: git writes " ", "+", "-" or
		// "\" first; an empty body line (an empty context line with its
		// trailing space stripped) counts as context.
		prefix, content := " ", ""
		if raw != "" {
			prefix, content = raw[:1], raw[1:]
		}
		kind := view.KindContext
		var oldLine, newLine int
		switch prefix {
		case "-":
			kind, oldLine = view.KindDel, oldNo
			oldNo++
		case "+":
			kind, newLine = view.KindAdd, newNo
			newNo++
		case "\\":
			kind = view.KindNoNewline
		default:
			kind, oldLine, newLine = view.KindContext, oldNo, newNo
			oldNo++
			newNo++
		}

		segs := lead(oldLine, newLine)
		if kind == view.KindNoNewline {
			// git's marker line has no -/+ prefix and no line number; its
			// text is rendered as it is.
			segs = append(segs, segment{text: raw})
		} else {
			content = expandTabs(content)
			body := []segment{{text: prefix, styles: themeStyles(th, baseStyle(th, kind))}}
			runs := contentRuns(content, th, baseStyle(th, kind), hl)
			if spans := spansFor(i, kind, delSpans, addSpans); len(spans) > 0 {
				runs = applyWordSpans(runs, spans, wordStyle(th, kind))
			}
			body = append(body, runs...)
			if bg, ok := lineStyle(th, kind); ok {
				// The tint is the outermost style, so syntax colours and
				// the stronger changed-word background still show on top.
				for j := range body {
					body[j].styles = append(append([]lipgloss.Style{}, body[j].styles...), bg)
				}
			}
			segs = append(segs, body...)
		}
		l := renderLine(segs, o.Width, kind, h.ID, h.FileIdx, oldLine, newLine)
		if bg, ok := lineStyle(th, kind); ok {
			pad := padLine(l.Plain, o.Width)
			l.Text += bg.Render(pad)
			l.Plain += pad
		}
		out = append(out, withGutter(l, gutterLen))
	}
	return out
}

// withGutter sets l.Gutter, capped to what survived truncation.
func withGutter(l view.Line, n int) view.Line {
	l.Gutter = min(n, len(l.Plain))
	return l
}

// RenderGroupLines renders the given hunks grouped by file in diff order:
// for each file a view.RenderFileHeader line, then its hunks, then a blank
// line between files. Unknown IDs are skipped; order of hunkIDs is ignored.
func RenderGroupLines(d *gitdiff.Diff, hunkIDs []string, th view.Theme, width int, marked map[string]bool) []view.Line {
	return RenderGroupLinesOpts(d, hunkIDs, th, width, GroupOpts{Marked: marked})
}

// GroupOpts configures RenderGroupLinesOpts.
type GroupOpts struct {
	// Marked draws these hunks as marked.
	Marked map[string]bool
	// Numbers is HunkOpts.Numbers for every hunk.
	Numbers int
	// File gives each file block's header state from its file index and the
	// hunk IDs listed under it; nil draws every block expanded. A folded
	// block renders its header only.
	File func(fileIdx int, hunkIDs []string) view.FileState
}

// RenderGroupLinesOpts is RenderGroupLines with folding, review marks and
// line numbers.
func RenderGroupLinesOpts(d *gitdiff.Diff, hunkIDs []string, th view.Theme, width int, o GroupOpts) []view.Line {
	out := []view.Line{}
	if d == nil {
		return out
	}
	// A hunk given twice is rendered once; unknown IDs are dropped.
	hunks := make([]*gitdiff.Hunk, 0, len(hunkIDs))
	seen := make(map[string]bool, len(hunkIDs))
	for _, id := range hunkIDs {
		if seen[id] {
			continue
		}
		h := d.Hunk(id)
		if h == nil {
			continue
		}
		seen[id] = true
		hunks = append(hunks, h)
	}
	sort.SliceStable(hunks, func(i, j int) bool { return hunks[i].Index < hunks[j].Index })

	// Hunks carry the same Index order as their files, so grouping the
	// consecutive runs walks the files in increasing order.
	type fileGroup struct {
		fileIdx int
		hunks   []*gitdiff.Hunk
	}
	var groups []fileGroup
	for _, h := range hunks {
		if n := len(groups); n > 0 && groups[n-1].fileIdx == h.FileIdx {
			groups[n-1].hunks = append(groups[n-1].hunks, h)
			continue
		}
		groups = append(groups, fileGroup{fileIdx: h.FileIdx, hunks: []*gitdiff.Hunk{h}})
	}

	for gi, g := range groups {
		if gi > 0 {
			out = append(out, view.Line{Kind: view.KindBlank, FileIdx: -1})
		}
		add, del := 0, 0
		for _, h := range g.hunks {
			add += h.Add
			del += h.Del
		}
		ids := make([]string, len(g.hunks))
		for i, h := range g.hunks {
			ids[i] = h.ID
		}
		var st view.FileState
		if o.File != nil {
			st = o.File(g.fileIdx, ids)
		}
		if g.fileIdx >= 0 && g.fileIdx < len(d.Files) {
			out = append(out, view.RenderFileHeaderState(d.Files[g.fileIdx], g.fileIdx, add, del, ids, st, th, width))
		} else {
			st.Folded = false // nothing to unfold from
		}
		if st.Folded {
			continue
		}
		// A hunk whose FileIdx is out of range (only possible for a
		// hand-built Diff) keeps its hunks but gets no header.
		for _, h := range g.hunks {
			out = append(out, RenderHunk(h, th, HunkOpts{
				Width:   width,
				Marked:  o.Marked[h.ID],
				Path:    hunkPath(h),
				Numbers: o.Numbers,
			})...)
		}
	}
	return out
}

// Span is a byte range [Start, End) of changed text.
type Span struct{ Start, End int }

// WordDiff finds the changed spans between a removed line and the added
// line paired with it (both without their -/+ prefix). ok is false when the
// lines are too long or too different to be worth highlighting.
func WordDiff(oldText, newText string) (oldSpans, newSpans []Span, ok bool) {
	if len(oldText) > maxWordBytes || len(newText) > maxWordBytes {
		return nil, nil, false
	}
	oldToks, newToks := wordTokens(oldText), wordTokens(newText)
	if len(oldToks) > maxWordTokens || len(newToks) > maxWordTokens {
		return nil, nil, false
	}
	if oldText == newText {
		return nil, nil, true
	}

	kept := lcsTokens(oldText, oldToks, newText, newToks)
	oldSpans = changedSpans(oldToks, kept.old)
	newSpans = changedSpans(newToks, kept.new)

	changed := 0
	for _, s := range oldSpans {
		changed += s.End - s.Start
	}
	for _, s := range newSpans {
		changed += s.End - s.Start
	}
	if changed*100 > maxWordChangedPc*(len(oldText)+len(newText)) {
		return nil, nil, false
	}
	return oldSpans, newSpans, true
}

// token is one word-diff token as a byte range of its line.
type token struct{ start, end int }

// wordTokens splits s into tokens: a maximal run of [A-Za-z0-9_] is one
// token, any other single rune is one token. Offsets are byte offsets, so
// they always fall on rune boundaries.
func wordTokens(s string) []token {
	toks := make([]token, 0, len(s))
	for i := 0; i < len(s); {
		start := i
		if isWordByte(s[i]) {
			for i < len(s) && isWordByte(s[i]) {
				i++
			}
		} else {
			_, size := utf8.DecodeRuneInString(s[i:])
			i += size
		}
		toks = append(toks, token{start: start, end: i})
	}
	return toks
}

// isWordByte reports whether b is part of a word token.
func isWordByte(b byte) bool {
	switch {
	case b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z', b >= '0' && b <= '9', b == '_':
		return true
	}
	return false
}

// keptTokens says which tokens of each side are part of the longest common
// subsequence of the two token lists.
type keptTokens struct{ old, new []bool }

// lcsTokens marks the tokens of a and b that belong to a longest common
// subsequence of their texts.
func lcsTokens(a string, at []token, b string, bt []token) keptTokens {
	// dp[i][j] is the LCS length of at[i:] and bt[j:].
	dp := make([][]int, len(at)+1)
	for i := range dp {
		dp[i] = make([]int, len(bt)+1)
	}
	for i := len(at) - 1; i >= 0; i-- {
		for j := len(bt) - 1; j >= 0; j-- {
			if a[at[i].start:at[i].end] == b[bt[j].start:bt[j].end] {
				dp[i][j] = dp[i+1][j+1] + 1
				continue
			}
			dp[i][j] = max(dp[i+1][j], dp[i][j+1])
		}
	}

	kept := keptTokens{old: make([]bool, len(at)), new: make([]bool, len(bt))}
	i, j := 0, 0
	for i < len(at) && j < len(bt) {
		if a[at[i].start:at[i].end] == b[bt[j].start:bt[j].end] {
			kept.old[i], kept.new[j] = true, true
			i++
			j++
			continue
		}
		if dp[i+1][j] >= dp[i][j+1] {
			i++
		} else {
			j++
		}
	}
	return kept
}

// changedSpans merges the tokens that are not kept into spans of byte
// offsets, joining neighbours that touch.
func changedSpans(toks []token, kept []bool) []Span {
	var spans []Span
	for i, t := range toks {
		if kept[i] {
			continue
		}
		if n := len(spans); n > 0 && spans[n-1].End == t.start {
			spans[n-1].End = t.end
			continue
		}
		spans = append(spans, Span{Start: t.start, End: t.end})
	}
	return spans
}

// segment is one run of a rendered line: its plain text and the styles to
// wrap around it, innermost first.
type segment struct {
	text   string
	styles []lipgloss.Style
}

// renderLine joins segs into one view.Line, cutting the plain text to width
// (with view.Truncate) and styling only what survives the cut.
func renderLine(segs []segment, width int, kind view.LineKind, hunkID string, fileIdx, oldLine, newLine int) view.Line {
	text, plain := renderSegments(segs, width)
	return view.Line{
		Text:    text,
		Plain:   plain,
		Kind:    kind,
		HunkID:  hunkID,
		FileIdx: fileIdx,
		OldLine: oldLine,
		NewLine: newLine,
	}
}

// renderSegments returns the styled and the plain text of segs. Truncation
// happens on the plain text, so styling never changes the visible characters.
func renderSegments(segs []segment, width int) (text, plain string) {
	var whole strings.Builder
	for _, s := range segs {
		whole.WriteString(s.text)
	}
	plain = whole.String()

	cut := false
	if width > 0 {
		if t := view.Truncate(plain, width); t != plain {
			plain, cut = t, true
		}
	}
	kept := plain
	if cut {
		// Truncate ends the text with ellipsisMark; only the bytes before
		// it survive, and the styled text gets the mark back below.
		kept = plain[:len(plain)-len(ellipsisMark)]
		plain = kept
	}

	var b strings.Builder
	rest := len(kept)
	var lastStyles []lipgloss.Style
	for _, s := range segs {
		if rest <= 0 {
			break
		}
		n := min(len(s.text), rest)
		if n > 0 {
			b.WriteString(applyStyles(s.text[:n], s.styles))
			rest -= n
			lastStyles = s.styles
		}
	}
	if cut {
		b.WriteString(applyStyles(ellipsisMark, lastStyles))
		plain += ellipsisMark
	}
	return b.String(), plain
}

// applyStyles wraps s in styles, innermost first.
func applyStyles(s string, styles []lipgloss.Style) string {
	for _, st := range styles {
		s = st.Render(s)
	}
	return s
}

// themeStyles is st as a style list, or none under the plain theme.
func themeStyles(th view.Theme, st lipgloss.Style) []lipgloss.Style {
	if th.Plain {
		return nil
	}
	return []lipgloss.Style{st}
}

// baseStyle is the theme's style for a body line of the given kind.
func baseStyle(th view.Theme, kind view.LineKind) lipgloss.Style {
	switch kind {
	case view.KindAdd:
		return th.Add
	case view.KindDel:
		return th.Del
	default:
		return th.Context
	}
}

// lineStyle is the background tint for a whole "+" or "-" line; false for
// other kinds and under the plain theme.
func lineStyle(th view.Theme, kind view.LineKind) (lipgloss.Style, bool) {
	if th.Plain {
		return lipgloss.Style{}, false
	}
	switch kind {
	case view.KindAdd:
		return th.AddLine, true
	case view.KindDel:
		return th.DelLine, true
	}
	return lipgloss.Style{}, false
}

// padLine is the run of spaces that carries a tinted line's background to
// width columns ("" when width <= 0).
func padLine(plain string, width int) string {
	n := width - lipgloss.Width(plain)
	if width <= 0 || n <= 0 {
		return ""
	}
	return strings.Repeat(" ", n)
}

// wordStyle is the theme's style for changed words on a body line.
func wordStyle(th view.Theme, kind view.LineKind) lipgloss.Style {
	if kind == view.KindAdd {
		return th.AddWord
	}
	return th.DelWord
}

// expandTabs replaces every tab in s with spaces up to the next multiple of
// tabWidth columns, counting columns from the start of s.
func expandTabs(s string) string {
	if !strings.ContainsRune(s, '\t') {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + tabWidth)
	col := 0
	for _, r := range s {
		if r == '\t' {
			n := tabWidth - col%tabWidth
			b.WriteString(strings.Repeat(" ", n))
			col += n
			continue
		}
		b.WriteRune(r)
		col += lipgloss.Width(string(r))
	}
	return b.String()
}

// highlighter tokenises hunk body lines with one lexer and one style.
type highlighter struct {
	lexer chroma.Lexer
	style *chroma.Style
	// tokenStyles caches the foreground style per token type; a nil entry
	// means the style gives the type no colour.
	tokenStyles map[chroma.TokenType]*lipgloss.Style
}

// newHighlighter returns the highlighter for path, whose lexer is nil when
// the path has no known lexer.
func newHighlighter(th view.Theme, path string) *highlighter {
	lx := chromalexers.Match(path)
	if lx == nil {
		return &highlighter{}
	}
	st := palette(strings.Contains(th.ChromaStyle, "dark"))
	return &highlighter{
		lexer:       chroma.Coalesce(lx),
		style:       st,
		tokenStyles: make(map[chroma.TokenType]*lipgloss.Style),
	}
}

// contentRuns splits one body line's content into styled segments: the
// theme's base colour, with the lexer's colours on top when hl has a lexer.
func contentRuns(content string, th view.Theme, base lipgloss.Style, hl *highlighter) []segment {
	if content == "" {
		return nil
	}
	if th.Plain {
		return []segment{{text: content}}
	}
	if hl == nil || hl.lexer == nil {
		return []segment{{text: content, styles: []lipgloss.Style{base}}}
	}
	toks, ok := hl.tokenise(content)
	if !ok {
		return []segment{{text: content, styles: []lipgloss.Style{base}}}
	}
	segs := make([]segment, 0, len(toks))
	for _, t := range toks {
		styles := []lipgloss.Style{base}
		if tokStyle := hl.styleFor(t.Type); tokStyle != nil {
			// The token colour goes inside the base colour, so it wins.
			styles = []lipgloss.Style{*tokStyle, base}
		}
		segs = append(segs, segment{text: t.Value, styles: styles})
	}
	return segs
}

// tokenise returns the tokens of one line. ok is false when the lexer fails
// or when it does not reproduce content byte for byte (so that highlighting
// can never change the visible text).
func (hl *highlighter) tokenise(content string) ([]chroma.Token, bool) {
	it, err := hl.lexer.Tokenise(nil, content)
	if err != nil {
		return nil, false
	}
	var toks []chroma.Token
	var got strings.Builder
	got.Grow(len(content))
	for t := it(); t != chroma.EOF; t = it() {
		if t.Value == "" {
			continue
		}
		toks = append(toks, t)
		got.WriteString(t.Value)
	}
	if got.String() != content {
		return nil, false
	}
	return toks, true
}

// styleFor returns the foreground style for a token type, or nil when the
// style gives the type no colour.
func (hl *highlighter) styleFor(t chroma.TokenType) *lipgloss.Style {
	if st, ok := hl.tokenStyles[t]; ok {
		return st
	}
	var st *lipgloss.Style
	if entry := hl.style.Get(t); entry.Colour.IsSet() {
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(entry.Colour.String()))
		if entry.Bold == chroma.Yes {
			style = style.Bold(true)
		}
		st = &style
	}
	hl.tokenStyles[t] = st
	return st
}

// wordSpans computes the word-diff spans of every paired -/+ run in lines,
// keyed by body line index. It returns nothing under the plain theme.
func wordSpans(lines []string, th view.Theme) (delSpans, addSpans map[int][]Span) {
	if th.Plain {
		return nil, nil
	}
	delSpans = make(map[int][]Span)
	addSpans = make(map[int][]Span)
	for i := 0; i < len(lines); {
		if !isDelLine(lines[i]) {
			i++
			continue
		}
		delStart := i
		for i < len(lines) && isDelLine(lines[i]) {
			i++
		}
		delCount := i - delStart
		addStart := i
		for i < len(lines) && isAddLine(lines[i]) {
			i++
		}
		addCount := i - addStart
		for k := 0; k < min(delCount, addCount); k++ {
			di, ai := delStart+k, addStart+k
			oldText := expandTabs(lines[di][1:])
			newText := expandTabs(lines[ai][1:])
			oldS, newS, ok := WordDiff(oldText, newText)
			if !ok {
				continue
			}
			delSpans[di] = oldS
			addSpans[ai] = newS
		}
	}
	return delSpans, addSpans
}

// isDelLine reports whether a body line is a "-" line.
func isDelLine(l string) bool { return strings.HasPrefix(l, "-") }

// isAddLine reports whether a body line is a "+" line.
func isAddLine(l string) bool { return strings.HasPrefix(l, "+") }

// spansFor returns the word-diff spans of body line i, which apply to the
// content of a "-" line when it is the old side of a pair and to a "+" line
// when it is the new side.
func spansFor(i int, kind view.LineKind, delSpans, addSpans map[int][]Span) []Span {
	if kind == view.KindAdd {
		return addSpans[i]
	}
	return delSpans[i]
}

// applyWordSpans copies segs, splitting them so that every byte inside a
// span also carries the word style.
func applyWordSpans(segs []segment, spans []Span, word lipgloss.Style) []segment {
	if len(spans) == 0 {
		return segs
	}
	out := make([]segment, 0, len(segs)+len(spans))
	pos, si := 0, 0
	for _, s := range segs {
		end := pos + len(s.text)
		for cur := pos; cur < end; {
			for si < len(spans) && spans[si].End <= cur {
				si++
			}
			next, changed := end, false
			if si < len(spans) && spans[si].Start <= cur {
				next, changed = min(spans[si].End, end), true
			} else if si < len(spans) {
				next = min(spans[si].Start, end)
			}
			styles := s.styles
			if changed {
				styles = append(append([]lipgloss.Style{}, s.styles...), word)
			}
			out = append(out, segment{text: s.text[cur-pos : next-pos], styles: styles})
			cur = next
		}
		pos = end
	}
	return out
}

// hunkPath is the file path of a hunk, which picks its lexer.
func hunkPath(h *gitdiff.Hunk) string {
	if h.File == nil {
		return ""
	}
	return h.File.Path()
}

// clump builds its two syntax palettes in code (after GitHub's light and dark
// themes) instead of importing chroma/v2/styles, whose package init parses
// every bundled style (~3.6 ms on every start, including `clump push`).
var (
	paletteOnce           sync.Once
	darkStyle, lightStyle *chroma.Style
)

func palette(dark bool) *chroma.Style {
	paletteOnce.Do(func() {
		darkStyle = chroma.MustNewStyle("clump-dark", chroma.StyleEntries{
			chroma.Comment:           "italic #8b949e",
			chroma.CommentPreproc:    "#ff7b72",
			chroma.Keyword:           "#ff7b72",
			chroma.KeywordConstant:   "#79c0ff",
			chroma.KeywordType:       "#ffa657",
			chroma.Operator:          "#ff7b72",
			chroma.Name:              "",
			chroma.NameBuiltin:       "#ffa657",
			chroma.NameClass:         "bold #f0883e",
			chroma.NameFunction:      "#d2a8ff",
			chroma.NameTag:           "#7ee787",
			chroma.NameAttribute:     "#79c0ff",
			chroma.NameConstant:      "#79c0ff",
			chroma.NameDecorator:     "#d2a8ff",
			chroma.LiteralString:     "#a5d6ff",
			chroma.LiteralNumber:     "#79c0ff",
			chroma.GenericHeading:    "bold #79c0ff",
			chroma.GenericSubheading: "bold #79c0ff",
			chroma.GenericEmph:       "italic",
			chroma.GenericStrong:     "bold",
		})
		lightStyle = chroma.MustNewStyle("clump-light", chroma.StyleEntries{
			chroma.Comment:           "italic #6e7781",
			chroma.CommentPreproc:    "#cf222e",
			chroma.Keyword:           "#cf222e",
			chroma.KeywordConstant:   "#0550ae",
			chroma.KeywordType:       "#953800",
			chroma.Operator:          "#cf222e",
			chroma.Name:              "",
			chroma.NameBuiltin:       "#953800",
			chroma.NameClass:         "bold #953800",
			chroma.NameFunction:      "#8250df",
			chroma.NameTag:           "#116329",
			chroma.NameAttribute:     "#0550ae",
			chroma.NameConstant:      "#0550ae",
			chroma.NameDecorator:     "#8250df",
			chroma.LiteralString:     "#0a3069",
			chroma.LiteralNumber:     "#0550ae",
			chroma.GenericHeading:    "bold #0550ae",
			chroma.GenericSubheading: "bold #0550ae",
			chroma.GenericEmph:       "italic",
			chroma.GenericStrong:     "bold",
		})
	})
	if dark {
		return darkStyle
	}
	return lightStyle
}
