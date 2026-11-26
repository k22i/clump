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

const tabWidth = 4

const ellipsisMark = "…"

const (
	maxWordTokens    = 200
	maxWordBytes     = 1000
	maxWordChangedPc = 60
)

type HunkOpts struct {
	Width   int
	Marked  bool
	Path    string
	Numbers int
	Split   bool
}

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

func numberCol(w, n int) string {
	if n <= 0 {
		return strings.Repeat(" ", w)
	}
	s := strconv.Itoa(n)
	if len(s) < w {
		s = strings.Repeat(" ", w-len(s)) + s
	}
	return s
}

func numberCols(w, oldLine, newLine int) string {
	return numberCol(w, oldLine) + " " + numberCol(w, newLine) + " "
}

func RenderHunk(h *gitdiff.Hunk, th view.Theme, o HunkOpts) []view.Line {
	if h == nil {
		return []view.Line{}
	}
	gutter, gutterStyles := "  ", []lipgloss.Style(nil)
	if o.Marked {
		gutter, gutterStyles = "x ", themeStyles(th, th.Marked)
	}

	out := make([]view.Line, 0, len(h.Lines)+1)
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
	if o.Split && o.Width > 2 {
		return append(out, renderHunkSplit(h, th, o, gutter, gutterStyles, hl, delSpans, addSpans)...)
	}

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

	oldNo, newNo := h.OldStart, h.NewStart
	for i, raw := range h.Lines {
		kind, body := bodySegments(raw, i, th, hl, delSpans, addSpans)
		var oldLine, newLine int
		switch kind {
		case view.KindDel:
			oldLine = oldNo
			oldNo++
		case view.KindAdd:
			newLine = newNo
			newNo++
		case view.KindContext:
			oldLine, newLine = oldNo, newNo
			oldNo++
			newNo++
		}

		segs := lead(oldLine, newLine)
		if kind == view.KindNoNewline {
			segs = append(segs, segment{text: raw})
		} else {
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

func withGutter(l view.Line, n int) view.Line {
	l.Gutter = min(n, len(l.Plain))
	return l
}

func bodySegments(raw string, i int, th view.Theme, hl *highlighter, delSpans, addSpans map[int][]Span) (view.LineKind, []segment) {
	prefix, content := " ", ""
	if raw != "" {
		prefix, content = raw[:1], raw[1:]
	}
	kind := view.KindContext
	switch prefix {
	case "-":
		kind = view.KindDel
	case "+":
		kind = view.KindAdd
	case "\\":
		return view.KindNoNewline, nil
	}
	base := baseStyle(th, kind)
	content = expandTabs(content)
	body := []segment{{text: prefix, styles: themeStyles(th, base)}}
	runs := contentRuns(content, th, base, hl)
	if spans := spansFor(i, kind, delSpans, addSpans); len(spans) > 0 {
		runs = applyWordSpans(runs, spans, wordStyle(th, kind))
	}
	body = append(body, runs...)
	if bg, ok := lineStyle(th, kind); ok {
		for j := range body {
			body[j].styles = append(append([]lipgloss.Style{}, body[j].styles...), bg)
		}
	}
	return kind, body
}

type splitCell struct {
	kind         view.LineKind
	raw          string
	idx          int
	oldNo, newNo int
}

type splitRow struct{ left, right []*splitCell }

func renderHunkSplit(h *gitdiff.Hunk, th view.Theme, o HunkOpts, gutter string, gutterStyles []lipgloss.Style, hl *highlighter, delSpans, addSpans map[int][]Span) []view.Line {
	lw := (o.Width - 1) / 2
	rw := o.Width - 1 - lw
	leadLen := len(gutter)
	if o.Numbers > 0 {
		leadLen += o.Numbers + 1
	}

	sideSegs := func(c *splitCell, left bool) []segment {
		var segs []segment
		if left {
			segs = append(segs, segment{text: gutter, styles: gutterStyles})
		}
		if c == nil {
			return segs
		}
		if o.Numbers > 0 {
			n := c.newNo
			if left {
				n = c.oldNo
			}
			segs = append(segs, segment{text: numberCol(o.Numbers, n) + " ", styles: themeStyles(th, th.Dim)})
		}
		if c.kind == view.KindNoNewline {
			return append(segs, segment{text: c.raw})
		}
		_, body := bodySegments(c.raw, c.idx, th, hl, delSpans, addSpans)
		return append(segs, body...)
	}
	cellStyle := func(c *splitCell) (lipgloss.Style, bool) {
		if c == nil || c.kind == view.KindNoNewline {
			return lipgloss.Style{}, false
		}
		return lineStyle(th, c.kind)
	}

	out := make([]view.Line, 0, len(h.Lines)+1)
	for _, r := range pairSplitRows(h) {
		for lvl := 0; lvl < max(len(r.left), len(r.right)); lvl++ {
			var lc, rc *splitCell
			if lvl < len(r.left) {
				lc = r.left[lvl]
			}
			if lvl < len(r.right) {
				rc = r.right[lvl]
			}
			kind := view.KindContext
			if lc == nil && rc != nil {
				kind = rc.kind
			}
			if rc == nil && lc != nil {
				kind = lc.kind
			}
			lt, lp := renderSegments(sideSegs(lc, true), lw)
			rt, rp := renderSegments(sideSegs(rc, false), rw)
			lpad, rpad := padLine(lp, lw), padLine(rp, rw)
			if bg, ok := cellStyle(lc); ok {
				lt += bg.Render(lpad)
			} else {
				lt += lpad
			}
			if bg, ok := cellStyle(rc); ok {
				rt += bg.Render(rpad)
			} else {
				rt += rpad
			}
			lp, rp = lp+lpad, rp+rpad
			oldLine, newLine := 0, 0
			if lc != nil {
				oldLine = lc.oldNo
			}
			if rc != nil {
				newLine = rc.newNo
			}
			out = append(out, withGutter(view.Line{
				Text:    lt + applyStyles("│", themeStyles(th, th.Border)) + rt,
				Plain:   lp + "│" + rp,
				Kind:    kind,
				HunkID:  h.ID,
				FileIdx: h.FileIdx,
				OldLine: oldLine,
				NewLine: newLine,
			}, leadLen))
		}
	}
	return out
}

func pairSplitRows(h *gitdiff.Hunk) []splitRow {
	lines := h.Lines
	oldNo, newNo := h.OldStart, h.NewStart
	nextOld := func() int { n := oldNo; oldNo++; return n }
	nextNew := func() int { n := newNo; newNo++; return n }

	runStacks := func(i int, isRun func(string) bool, kind view.LineKind, next func() int) ([][]*splitCell, int) {
		var stacks [][]*splitCell
		for i < len(lines) && isRun(lines[i]) {
			c := &splitCell{kind: kind, raw: lines[i], idx: i}
			if kind == view.KindDel {
				c.oldNo = next()
			} else {
				c.newNo = next()
			}
			stack := []*splitCell{c}
			i++
			for i < len(lines) && strings.HasPrefix(lines[i], "\\") {
				stack = append(stack, &splitCell{kind: view.KindNoNewline, raw: lines[i], idx: i})
				i++
			}
			stacks = append(stacks, stack)
		}
		return stacks, i
	}

	var rows []splitRow
	for i := 0; i < len(lines); {
		switch {
		case isDelLine(lines[i]):
			dels, next := runStacks(i, isDelLine, view.KindDel, nextOld)
			i = next
			var adds [][]*splitCell
			if i < len(lines) && isAddLine(lines[i]) {
				adds, i = runStacks(i, isAddLine, view.KindAdd, nextNew)
			}
			for k := 0; k < max(len(dels), len(adds)); k++ {
				row := splitRow{}
				if k < len(dels) {
					row.left = dels[k]
				}
				if k < len(adds) {
					row.right = adds[k]
				}
				rows = append(rows, row)
			}
		case isAddLine(lines[i]):
			var adds [][]*splitCell
			adds, i = runStacks(i, isAddLine, view.KindAdd, nextNew)
			for _, a := range adds {
				rows = append(rows, splitRow{right: a})
			}
		case strings.HasPrefix(lines[i], "\\"):
			rows = append(rows, splitRow{left: []*splitCell{{kind: view.KindNoNewline, raw: lines[i], idx: i}}})
			i++
		default:
			c := &splitCell{kind: view.KindContext, raw: lines[i], idx: i, oldNo: nextOld(), newNo: nextNew()}
			row := splitRow{left: []*splitCell{c}, right: []*splitCell{c}}
			i++
			for i < len(lines) && strings.HasPrefix(lines[i], "\\") {
				row.left = append(row.left, &splitCell{kind: view.KindNoNewline, raw: lines[i], idx: i})
				i++
			}
			rows = append(rows, row)
		}
	}
	return rows
}

func RenderGroupLines(d *gitdiff.Diff, hunkIDs []string, th view.Theme, width int, marked map[string]bool) []view.Line {
	return RenderGroupLinesOpts(d, hunkIDs, th, width, GroupOpts{Marked: marked})
}

type GroupOpts struct {
	Marked  map[string]bool
	Numbers int
	File    func(fileIdx int, hunkIDs []string) view.FileState
	Split   bool
}

func RenderGroupLinesOpts(d *gitdiff.Diff, hunkIDs []string, th view.Theme, width int, o GroupOpts) []view.Line {
	out := []view.Line{}
	if d == nil {
		return out
	}
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
			st.Folded = false
			text := view.Truncate("── (unknown file "+strconv.Itoa(g.fileIdx)+") ", width)
			if w := lipgloss.Width(text); w < width {
				text += strings.Repeat("─", width-w)
			}
			out = append(out, view.Line{Text: th.Style(th.Error, text), Plain: text, Kind: view.KindFile,
				FileIdx: g.fileIdx, BlockHunks: ids})
		}
		if st.Folded {
			continue
		}
		for _, h := range g.hunks {
			out = append(out, RenderHunk(h, th, HunkOpts{
				Width:   width,
				Marked:  o.Marked[h.ID],
				Path:    hunkPath(h),
				Numbers: o.Numbers,
				Split:   o.Split,
			})...)
		}
	}
	return out
}

type Span struct{ Start, End int }

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

type token struct{ start, end int }

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

func isWordByte(b byte) bool {
	switch {
	case b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z', b >= '0' && b <= '9', b == '_':
		return true
	}
	return false
}

type keptTokens struct{ old, new []bool }

func lcsTokens(a string, at []token, b string, bt []token) keptTokens {
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

type segment struct {
	text   string
	styles []lipgloss.Style
}

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

func applyStyles(s string, styles []lipgloss.Style) string {
	for _, st := range styles {
		s = st.Render(s)
	}
	return s
}

func themeStyles(th view.Theme, st lipgloss.Style) []lipgloss.Style {
	if th.Plain {
		return nil
	}
	return []lipgloss.Style{st}
}

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

func padLine(plain string, width int) string {
	n := width - lipgloss.Width(plain)
	if width <= 0 || n <= 0 {
		return ""
	}
	return strings.Repeat(" ", n)
}

func wordStyle(th view.Theme, kind view.LineKind) lipgloss.Style {
	if kind == view.KindAdd {
		return th.AddWord
	}
	return th.DelWord
}

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

type highlighter struct {
	lexer       chroma.Lexer
	style       *chroma.Style
	tokenStyles map[chroma.TokenType]*lipgloss.Style
}

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
			styles = []lipgloss.Style{*tokStyle, base}
		}
		segs = append(segs, segment{text: t.Value, styles: styles})
	}
	return segs
}

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

func isDelLine(l string) bool { return strings.HasPrefix(l, "-") }

func isAddLine(l string) bool { return strings.HasPrefix(l, "+") }

func spansFor(i int, kind view.LineKind, delSpans, addSpans map[int][]Span) []Span {
	if kind == view.KindAdd {
		return addSpans[i]
	}
	return delSpans[i]
}

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

func hunkPath(h *gitdiff.Hunk) string {
	if h.File == nil {
		return ""
	}
	return h.File.Path()
}

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
