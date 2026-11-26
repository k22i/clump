package tui

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/k22i/clump/internal/cache"
	"github.com/k22i/clump/internal/config"
	"github.com/k22i/clump/internal/engine"
	"github.com/k22i/clump/internal/gitdiff"
	"github.com/k22i/clump/internal/group"
	"github.com/k22i/clump/internal/provider"
	"github.com/k22i/clump/internal/tui/view"
)

type harness struct {
	t     *testing.T
	m     *Model
	fake  *provider.Fake
	store *cache.Store
	eng   *engine.Engine
}

const testPatch = `diff --git a/src/a.go b/src/a.go
--- a/src/a.go
+++ b/src/a.go
@@ -1,3 +1,4 @@ package a
 package a
+import "fmt"
 func A() {}
 // end
@@ -10,2 +11,3 @@ func B() {
 b := 1
+fmt.Println(b)
 }
diff --git a/src/b.go b/src/b.go
--- a/src/b.go
+++ b/src/b.go
@@ -1 +1 @@
-var x = 1
+var x = 2
diff --git a/README.md b/README.md
--- a/README.md
+++ b/README.md
@@ -1 +1,2 @@
 # Title
+Searchable needle here.
`

func newHarness(t *testing.T, store *cache.Store, mut func(*config.Config)) *harness {
	t.Helper()
	cfg := config.Default()
	cfg.Providers = append(cfg.Providers,
		provider.Config{Name: "fake", Type: provider.TypeFake, BytesPerToken: 2},
		provider.Config{Name: "fake2", Type: provider.TypeFake, BytesPerToken: 2})
	cfg.Cheap = "fake"
	if mut != nil {
		mut(cfg)
	}
	reg, err := cfg.Registry()
	if err != nil {
		t.Fatal(err)
	}
	fake := &provider.Fake{ProviderName: "fake"}
	if err := reg.Set("fake", fake); err != nil {
		t.Fatal(err)
	}
	if err := reg.Set("fake2", &provider.Fake{ProviderName: "fake2", Responses: []string{`{"groups":[{"title":"All of it","hunks":["h1","h2","h3","h4"]}]}`}}); err != nil {
		t.Fatal(err)
	}
	if store == nil {
		store, err = cache.Open(cache.Options{Dir: t.TempDir()})
		if err != nil {
			t.Fatal(err)
		}
	}
	eng := engine.New(cfg, reg, store)
	m := New(context.Background(), Options{Engine: eng, Store: store, Cfg: cfg, Provider: "cheap", Theme: view.PlainTheme(),
		Repo: t.TempDir()})
	h := &harness{t: t, m: m, fake: fake, store: store, eng: eng}
	h.send(tea.WindowSizeMsg{Width: 120, Height: 30})
	return h
}

func (h *harness) send(msg tea.Msg) {
	h.t.Helper()
	_, cmd := h.m.Update(msg)
	h.run(cmd)
}

func (h *harness) run(cmd tea.Cmd) {
	h.t.Helper()
	if cmd == nil {
		return
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	var msg tea.Msg
	select {
	case msg = <-done:
	case <-time.After(2 * time.Second):
		return
	}
	switch msg := msg.(type) {
	case nil:
		return
	case tea.BatchMsg:
		for _, c := range msg {
			h.run(c)
		}
		return
	case tea.QuitMsg:
		return
	}
	if v := reflect.ValueOf(msg); v.Kind() == reflect.Slice && v.Type().Elem() == reflect.TypeOf(tea.Cmd(nil)) {
		for i := 0; i < v.Len(); i++ {
			h.run(v.Index(i).Interface().(tea.Cmd))
		}
		return
	}
	if reflect.TypeOf(msg).PkgPath() != reflect.TypeOf(groupedMsg{}).PkgPath() && !isSysMsg(msg) {
		return
	}
	h.send(msg)
}

func isSysMsg(msg tea.Msg) bool { return strings.Contains(reflect.TypeOf(msg).PkgPath(), "/tui/") }

func (h *harness) keys(ks ...string) {
	h.t.Helper()
	for _, k := range ks {
		h.send(keyMsg(k))
	}
}

func (h *harness) typeText(s string) {
	for _, r := range s {
		h.send(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

func keyMsg(k string) tea.KeyPressMsg {
	switch k {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "ctrl+r":
		return tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl}
	case "ctrl+d":
		return tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl}
	}
	r := []rune(k)[0]
	return tea.KeyPressMsg{Code: r, Text: k}
}

func (h *harness) load(patch string) {
	h.t.Helper()
	l, err := gitdiff.LoadPatch(patch)
	if err != nil {
		h.t.Fatal(err)
	}
	l.Title, l.Ref = "Test change", "abc123def456"
	h.send(loadedMsg{l: l})
}

func (h *harness) grouping() group.Grouping { return h.m.doc.hist.Current() }

func (h *harness) check() {
	h.t.Helper()
	if err := group.Check(h.m.doc.L.Diff, h.grouping()); err != nil {
		h.t.Fatalf("Check: %v\n%+v", err, h.grouping())
	}
}

func (h *harness) titles() []string {
	var out []string
	for _, g := range h.grouping().Groups {
		out = append(out, g.Title)
	}
	return out
}

func (h *harness) screen() string { return h.m.render() }

func TestKeyNames(t *testing.T) {
	for _, k := range []string{"enter", "esc", "tab", "down", "ctrl+r", "ctrl+d", "J", "x", "?", "]"} {
		if got := keyMsg(k).String(); got != k {
			t.Errorf("keyMsg(%q).String() = %q", k, got)
		}
	}
}

func TestOpenGroupsThenCaches(t *testing.T) {
	h := newHarness(t, nil, nil)
	h.load(testPatch)
	if h.m.doc.state != stGrouped || h.fake.Calls() != 1 {
		t.Fatalf("state %v calls %d", h.m.doc.state, h.fake.Calls())
	}
	if got := h.titles(); !reflect.DeepEqual(got, []string{"Changes in src", "Changes in (root)"}) {
		t.Fatalf("titles %v", got)
	}
	h.check()
	if s := h.screen(); !strings.Contains(s, "Changes in src") || !strings.Contains(s, "→ fake") || strings.Contains(s, "cached") {
		t.Fatalf("screen:\n%s", s)
	}
	h2 := newHarness(t, h.store, nil)
	h2.fake = h.fake
	h2.load(testPatch)
	if h2.m.doc.state != stGrouped || !h2.m.doc.cached || h.fake.Calls() != 1 {
		t.Fatalf("second open: state %v cached %v", h2.m.doc.state, h2.m.doc.cached)
	}
	if !strings.Contains(h2.screen(), "cached") {
		t.Fatal("no cached marker")
	}
}

func TestRawFileListWhileGrouping(t *testing.T) {
	h := newHarness(t, nil, nil)
	h.fake.Delay = time.Hour
	l, _ := gitdiff.LoadPatch(testPatch)
	_, cmd := h.m.Update(loadedMsg{l: l})
	_ = cmd
	if h.m.doc.state != stGrouping {
		t.Fatalf("state %v", h.m.doc.state)
	}
	if got := h.titles(); !reflect.DeepEqual(got, []string{"src/a.go", "src/b.go", "README.md"}) {
		t.Fatalf("raw file list %v", got)
	}
	h.keys("j", "enter")
	if h.m.focus != focusDiff || h.m.cursor != 1 {
		t.Fatal("raw list not browsable")
	}
	if s := h.screen(); !strings.Contains(s, "grouping…") || !strings.Contains(s, "var x = 2") {
		t.Fatalf("screen:\n%s", s)
	}
	h.keys("x")
	if len(h.m.marks) != 0 || !strings.Contains(h.m.flash, "wait for the groups") {
		t.Fatal("editing allowed before grouping")
	}
}

func TestEditMoveToNewGroupAndPersist(t *testing.T) {
	h := newHarness(t, nil, nil)
	h.load(testPatch)
	h.keys("enter", "}", "}")
	if id := h.m.cursorHunk(); id != "h2" {
		t.Fatalf("cursor hunk %q", id)
	}
	h.keys("x")
	if !h.m.marks["h2"] {
		t.Fatal("h2 not marked")
	}
	h.keys("M")
	if h.m.ov != ovTarget {
		t.Fatal("no target picker")
	}
	h.send(keyMsg("k"))
	h.keys("enter")
	if h.m.ov != ovInput {
		t.Fatal("no title input")
	}
	h.typeText("Logging")
	h.keys("enter")
	h.check()
	g := h.grouping()
	if len(g.Groups) != 3 || g.Groups[2].Title != "Logging" || !reflect.DeepEqual(g.Groups[2].Hunks, []string{"h2"}) {
		t.Fatalf("after move: %+v", g)
	}
	if len(h.m.marks) != 0 || !h.m.doc.edited() {
		t.Fatal("marks not cleared or not edited")
	}
	if !strings.Contains(h.screen(), "edited") {
		t.Fatal("no edited marker")
	}
	calls := h.fake.Calls()
	h2 := newHarness(t, h.store, nil)
	h2.load(testPatch)
	if !h2.m.doc.edited() || !group.Equal(h2.grouping(), g) || h.fake.Calls() != calls || h2.fake.Calls() != 0 {
		t.Fatalf("override not restored: edited=%v %+v", h2.m.doc.edited(), h2.grouping())
	}
	h2.keys("u")
	if !h2.m.doc.edited() || !strings.Contains(h2.m.flash, "nothing to undo") {
		t.Fatal("undo crossed sessions")
	}
	h.keys("u")
	if h.m.doc.edited() {
		t.Fatal("still edited after undo")
	}
	if _, ok, _ := h.store.LoadOverride(h.m.doc.Hash); ok {
		t.Fatal("override kept after undoing every edit")
	}
	h.keys("ctrl+r")
	if !h.m.doc.edited() {
		t.Fatal("redo lost the edit")
	}
	if _, ok, _ := h.store.LoadOverride(h.m.doc.Hash); !ok {
		t.Fatal("redo did not save the override")
	}
}

func TestEditKeys(t *testing.T) {
	h := newHarness(t, nil, nil)
	h.load(testPatch)
	h.keys("n")
	h.typeText("Empty one")
	h.keys("enter")
	h.check()
	if got := h.titles(); len(got) != 3 || got[2] != "Empty one" || h.m.cursor != 2 {
		t.Fatalf("new group: %v cursor %d", got, h.m.cursor)
	}
	h.keys("R")
	for range "Empty one" {
		h.keys("backspace")
	}
	h.typeText("Docs")
	h.keys("enter")
	h.typeText("Why it matters.")
	h.keys("enter")
	if g := h.grouping().Groups[2]; g.Title != "Docs" || g.Description != "Why it matters." {
		t.Fatalf("rename: %+v", g)
	}
	h.keys("K")
	if got := h.titles(); got[1] != "Docs" || h.m.cursor != 1 {
		t.Fatalf("reorder up: %v cursor %d", got, h.m.cursor)
	}
	h.keys("J")
	if got := h.titles(); got[2] != "Docs" {
		t.Fatalf("reorder down: %v", got)
	}
	h.keys("home", "g")
	if h.m.ov != ovTarget {
		t.Fatal("no merge picker")
	}
	h.keys("down", "enter")
	h.check()
	if got := h.titles(); !reflect.DeepEqual(got, []string{"Changes in (root)", "Docs"}) {
		t.Fatalf("merge: %v", got)
	}
	h.keys("end", "d")
	h.check()
	g := h.grouping()
	if last := g.Groups[len(g.Groups)-1]; last.ID != group.UngroupedID || len(last.Hunks) != 3 {
		t.Fatalf("delete: %+v", g)
	}
	for i := 0; i < 6; i++ {
		h.keys("u")
		h.check()
	}
	if h.m.doc.edited() {
		t.Fatalf("not back at the base: %v", h.titles())
	}
	h.keys("ctrl+r")
	h.check()
	if got := h.titles(); len(got) != 3 {
		t.Fatalf("redo: %v", got)
	}
}

func TestVisualMarkAndMove(t *testing.T) {
	h := newHarness(t, nil, nil)
	h.load(testPatch)
	h.keys("a")
	if h.m.mode != modeAll || h.m.focus != focusDiff {
		t.Fatal("all mode")
	}
	h.keys("home", "}")
	if h.m.cursorHunk() != "h1" {
		t.Fatalf("cursor %q", h.m.cursorHunk())
	}
	h.keys("v", "}", "}", "x")
	if !reflect.DeepEqual(h.m.selection(), []string{"h1", "h2", "h3"}) {
		t.Fatalf("visual selection %v", h.m.selection())
	}
	h.keys("M")
	h.keys("2")
	h.check()
	if g := h.grouping(); !reflect.DeepEqual(g.Groups[1].Hunks, []string{"h1", "h2", "h3", "h4"}) {
		t.Fatalf("move: %+v", g)
	}
	var bounds int
	for _, l := range h.m.ensureLines() {
		if l.Kind == view.KindBoundary {
			bounds++
		}
	}
	if bounds != 1 {
		t.Fatalf("boundaries %d", bounds)
	}
}

func TestSearch(t *testing.T) {
	h := newHarness(t, nil, nil)
	h.load(testPatch)
	h.keys("a", "/")
	h.typeText("needle")
	h.keys("enter")
	if len(h.m.matches) != 1 || !strings.Contains(h.m.ensureLines()[h.m.dcur].Plain, "needle") {
		t.Fatalf("matches %v dcur %d", h.m.matches, h.m.dcur)
	}
	h.keys("n")
	if !strings.Contains(h.m.ensureLines()[h.m.dcur].Plain, "needle") {
		t.Fatal("n lost the match")
	}
	h.keys("esc")
	if h.m.query != "" {
		t.Fatal("esc did not clear the query")
	}
}

func TestRegroupKeepsSnapshot(t *testing.T) {
	h := newHarness(t, nil, nil)
	h.load(testPatch)
	h.keys("n")
	h.typeText("Mine")
	h.keys("enter")
	h.keys("r")
	if h.m.ov != ovConfirm || h.m.confirm.kind != confirmRegroup {
		t.Fatal("regroup did not ask")
	}
	h.keys("s")
	if h.fake.Calls() != 2 || h.m.doc.edited() || len(h.m.doc.snapshots) != 1 {
		t.Fatalf("calls %d edited %v snapshots %d", h.fake.Calls(), h.m.doc.edited(), len(h.m.doc.snapshots))
	}
	ov, ok, err := h.store.LoadOverride(h.m.doc.Hash)
	if err != nil || !ok || len(ov.Grouping.Groups) != 0 || len(ov.Snapshots) != 1 || len(ov.Snapshots[0].Grouping.Groups) != 3 {
		t.Fatalf("override after regroup: %+v %v %v", ov, ok, err)
	}
	h.keys("r")
	if h.fake.Calls() != 3 {
		t.Fatalf("calls %d", h.fake.Calls())
	}
}

func TestOverCeilingAsks(t *testing.T) {
	h := newHarness(t, nil, func(c *config.Config) { c.Ceiling = 10 })
	h.load(testPatch)
	if h.m.doc.state != stNeedsConfirm || h.m.ov != ovConfirm || h.fake.Calls() != 0 {
		t.Fatalf("state %v ov %v calls %d", h.m.doc.state, h.m.ov, h.fake.Calls())
	}
	h.keys("n")
	if h.fake.Calls() != 0 || !strings.Contains(h.screen(), "press g to group (~") {
		t.Fatalf("declined but sent, or no hint:\n%s", h.screen())
	}
	h.keys("r")
	if h.m.ov != ovConfirm {
		t.Fatal("r above the ceiling should ask again")
	}
	h.keys("y")
	if h.fake.Calls() != 1 || h.m.doc.state != stGrouped {
		t.Fatalf("calls %d state %v", h.fake.Calls(), h.m.doc.state)
	}
}

func TestOverCeilingDeclinedThenG(t *testing.T) {
	h := newHarness(t, nil, func(c *config.Config) { c.Ceiling = 10 })
	h.load(testPatch)
	h.keys("n", "g")
	if h.fake.Calls() != 1 || h.m.doc.state != stGrouped {
		t.Fatalf("calls %d state %v flash %q", h.fake.Calls(), h.m.doc.state, h.m.flash)
	}
}

func TestProviderSwitch(t *testing.T) {
	h := newHarness(t, nil, nil)
	h.load(testPatch)
	h.keys("m")
	if h.m.ov != ovProvider {
		t.Fatal("no provider menu")
	}
	for i, it := range h.m.menu.items {
		if it.name == "fake2" {
			h.m.menu.cursor = i
		}
	}
	h.keys("enter")
	if got := h.titles(); !reflect.DeepEqual(got, []string{"All of it"}) || h.m.doc.provider != "fake2" {
		t.Fatalf("after switch: %v %s", got, h.m.doc.provider)
	}
}

func TestRenderSizes(t *testing.T) {
	for _, sz := range [][2]int{{140, 40}, {100, 20}, {80, 24}, {40, 12}} {
		t.Run(fmt.Sprint(sz), func(t *testing.T) {
			h := newHarness(t, nil, nil)
			h.send(tea.WindowSizeMsg{Width: sz[0], Height: sz[1]})
			h.load(testPatch)
			for _, ks := range [][]string{nil, {"enter"}, {"a"}, {"?"}, {"esc", "m"}, {"esc", "R"}} {
				h.keys(ks...)
				s := h.screen()
				lines := strings.Split(s, "\n")
				if len(lines) != sz[1] {
					t.Fatalf("%v: %d lines, want %d", ks, len(lines), sz[1])
				}
				for i, l := range lines {
					if w := lipgloss.Width(l); w > sz[0] {
						t.Fatalf("%v: line %d is %d wide: %q", ks, i, w, l)
					}
				}
			}
		})
	}
}

func TestCopyAndQuit(t *testing.T) {
	h := newHarness(t, nil, nil)
	h.load(testPatch)
	_, cmd := h.m.Update(keyMsg("y"))
	if cmd == nil || !strings.Contains(h.m.flash, "copying") {
		t.Fatal("y did nothing")
	}
	_, cmd = h.m.Update(keyMsg("q"))
	if cmd == nil {
		t.Fatal("q did not quit")
	}
}

func TestStatusReports(t *testing.T) {
	h := newHarness(t, nil, nil)
	var got []string
	h.m.o.Status = func(kind, msg string) tea.Cmd { got = append(got, kind+":"+msg); return nil }
	h.load(testPatch)
	if len(got) < 2 || !strings.HasPrefix(got[0], "working:") || got[len(got)-1] != "done:2 groups" {
		t.Fatalf("status %v", got)
	}
}

func TestRegroupCallsTheProviderEvenWithACachedResult(t *testing.T) {
	h := newHarness(t, nil, nil)
	h.load(testPatch)
	if h.fake.Calls() != 1 || h.m.doc.cached {
		t.Fatalf("first grouping: calls %d cached %v", h.fake.Calls(), h.m.doc.cached)
	}
	if _, ok, err := h.eng.Lookup(h.m.doc.L.Diff, "cheap"); !ok || err != nil {
		t.Fatalf("the first grouping should be cached already: ok=%v err=%v", ok, err)
	}
	h.keys("r")
	if h.fake.Calls() != 2 {
		t.Fatalf("r took the total provider calls to %d, want 2 (regroup never reads the result cache)", h.fake.Calls())
	}
	if h.m.doc.cached {
		t.Fatal("the regrouped document is marked cached")
	}
}
