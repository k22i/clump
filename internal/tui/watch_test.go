package tui

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/k22i/clump/internal/config"
	"github.com/k22i/clump/internal/gitdiff"
	"github.com/k22i/clump/internal/ipc"
	"github.com/k22i/clump/internal/lazygit"
)

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.b.Write(p) }
func (s *syncBuf) String() string              { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

func newWatchHarness(t *testing.T, mut func(*config.Config), auto bool) (*harness, *syncBuf) {
	t.Helper()
	h := newHarness(t, nil, mut)
	logb := &syncBuf{}
	wo := &WatchOptions{Repo: "/repo", Debounce: 5 * time.Millisecond, AutoGroup: auto, Log: logb,
		Resolve: func(ctx context.Context, repo string, sel lazygit.Selection, diff string) (*gitdiff.Loaded, error) {
			d, err := gitdiff.Parse(diff)
			if err != nil {
				return nil, err
			}
			return &gitdiff.Loaded{Diff: d, Raw: diff, Title: "sel " + sel.Ref, Ref: sel.Ref}, nil
		}}
	h.m = New(context.Background(), Options{Engine: h.eng, Store: h.store, Cfg: h.eng.Cfg, Provider: "cheap",
		Theme: h.m.th, Repo: "/repo", Watch: wo})
	h.send(tea.WindowSizeMsg{Width: 120, Height: 30})
	return h, logb
}

var sentNS int64 = 1

func push(argv, diff string) pushMsg {
	sentNS++
	return pushMsg{ipc.Msg{V: 1, SentNS: sentNS, RepoRoot: "/repo", GitArgv: strings.Fields(argv), Diff: diff}}
}

func patchN(n int) string {
	return "diff --git a/src/f.go b/src/f.go\n--- a/src/f.go\n+++ b/src/f.go\n@@ -1 +1 @@\n-old\n+new " + strings.Repeat("x", n) + "\n" +
		"diff --git a/README.md b/README.md\n--- a/README.md\n+++ b/README.md\n@@ -1 +1 @@\n-a\n+b" + strings.Repeat("y", n) + "\n"
}

func showArgv(sha string) string { return "git -C /repo show --stat -p " + sha + " --" }

func TestWatchFollowsAndCaches(t *testing.T) {
	h, logb := newWatchHarness(t, nil, true)
	if !strings.Contains(h.screen(), "watching /repo") {
		t.Fatalf("idle screen:\n%s", h.screen())
	}
	h.send(push(showArgv("aaaa111"), patchN(1)))
	if h.m.doc == nil || h.m.doc.state != stGrouped || h.fake.Calls() != 1 {
		t.Fatalf("first push: doc=%v calls=%d\n%s", h.m.doc, h.fake.Calls(), logb)
	}
	if !strings.Contains(h.screen(), "→ fake") || !strings.Contains(h.screen(), "auto on") {
		t.Fatalf("header:\n%s", h.screen())
	}
	h.send(push(showArgv("aaaa111"), patchN(1)))
	if h.fake.Calls() != 1 || !strings.Contains(logb.String(), "dedupe") {
		t.Fatalf("re-send made a call\n%s", logb)
	}
	h.send(push(showArgv("bbbb222"), patchN(2)))
	if h.fake.Calls() != 2 || h.m.doc.L.Ref != "commit:bbbb222" {
		t.Fatal("second commit not followed")
	}
	h.send(push(showArgv("aaaa111"), patchN(1)))
	if h.fake.Calls() != 2 || !h.m.doc.cached || !strings.Contains(h.screen(), "cached") {
		t.Fatalf("revisit not cached\n%s", logb)
	}
	if n := strings.Count(logb.String(), "call start"); n != 2 {
		t.Fatalf("call starts = %d", n)
	}
}

func TestWatchDropsOutOfOrder(t *testing.T) {
	h, logb := newWatchHarness(t, nil, true)
	newer := push(showArgv("bbbb222"), patchN(2))
	older := pushMsg{newer.m}
	older.m.SentNS = newer.m.SentNS - 1
	older.m.GitArgv, older.m.Diff = strings.Fields(showArgv("aaaa111")), patchN(1)
	h.send(newer)
	h.send(older)
	if h.m.doc.L.Ref != "commit:bbbb222" || !strings.Contains(logb.String(), "out of order") {
		t.Fatalf("ref %s\n%s", h.m.doc.L.Ref, logb)
	}
}

func TestWatchDebounceCollapsesBursts(t *testing.T) {
	h, logb := newWatchHarness(t, nil, true)
	var ticks []tea.Cmd
	for i := 0; i < 10; i++ {
		_, cmd := h.m.Update(push(showArgv("c"+strings.Repeat("0", i)), patchN(10+i)))
		ticks = append(ticks, cmd)
	}
	for _, c := range ticks {
		h.run(c)
	}
	if h.fake.Calls() != 1 || h.m.doc.L.Ref != "commit:c000000000" {
		t.Fatalf("calls %d ref %v\n%s", h.fake.Calls(), h.m.doc.L.Ref, logb)
	}
}

func (h *harness) step(msg tea.Msg) tea.Cmd {
	_, cmd := h.m.Update(msg)
	return cmd
}

func (h *harness) collect(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	switch msg := msg.(type) {
	case nil:
		return nil
	case tea.BatchMsg:
		var out []tea.Msg
		for _, c := range msg {
			out = append(out, h.collect(c)...)
		}
		return out
	}
	if v := reflect.ValueOf(msg); v.Kind() == reflect.Slice && v.Type().Elem() == reflect.TypeOf(tea.Cmd(nil)) {
		var out []tea.Msg
		for i := 0; i < v.Len(); i++ {
			out = append(out, h.collect(v.Index(i).Interface().(tea.Cmd))...)
		}
		return out
	}
	return []tea.Msg{msg}
}

func untilGrouped(t *testing.T, h *harness, cmd tea.Cmd) (grouped *groupedMsg, rest []tea.Msg) {
	t.Helper()
	queue := h.collect(cmd)
	for len(queue) > 0 {
		msg := queue[0]
		queue = queue[1:]
		if g, ok := msg.(groupedMsg); ok {
			return &g, queue
		}
		queue = append(queue, h.collect(h.step(msg))...)
	}
	return nil, nil
}

func TestWatchQueueLatestWinsAndStaleIsCached(t *testing.T) {
	h, logb := newWatchHarness(t, nil, true)
	gA, _ := untilGrouped(t, h, h.step(push(showArgv("aaaa111"), patchN(1))))
	if gA == nil {
		t.Fatal("A never grouped")
	}
	if g, _ := untilGrouped(t, h, h.step(push(showArgv("bbbb222"), patchN(2)))); g != nil {
		t.Fatal("B started while A was in flight")
	}
	if g, _ := untilGrouped(t, h, h.step(push(showArgv("cccc333"), patchN(3)))); g != nil {
		t.Fatal("C started while A was in flight")
	}
	if h.m.watch.queued == nil || h.m.watch.queued.DiffHash != h.m.doc.Hash || h.m.doc.L.Ref != "commit:cccc333" {
		t.Fatal("C should be the one queued call (latest wins)")
	}
	gC, _ := untilGrouped(t, h, h.step(*gA))
	if gC == nil {
		t.Fatal("queued C did not start after A")
	}
	h.send(*gC)
	if h.fake.Calls() != 2 || h.m.doc.state != stGrouped || h.m.doc.L.Ref != "commit:cccc333" {
		t.Fatalf("calls %d state %v\n%s", h.fake.Calls(), h.m.doc.state, logb)
	}
	if !strings.Contains(logb.String(), "call stale") || strings.Contains(logb.String(), "call start hash="+h.m.doc.Hash[:12]+" provider=fake est") && strings.Count(logb.String(), "call start") != 2 {
		t.Fatalf("log:\n%s", logb)
	}
	dA, _ := gitdiff.Parse(patchN(1))
	if _, ok, _ := h.eng.Lookup(dA, "cheap"); !ok {
		t.Fatal("stale A result not cached")
	}
	dB, _ := gitdiff.Parse(patchN(2))
	if _, ok, _ := h.eng.Lookup(dB, "cheap"); ok {
		t.Fatal("B was grouped although superseded")
	}
}

func TestWatchDimsAndCursorOnly(t *testing.T) {
	h, logb := newWatchHarness(t, nil, true)
	h.send(push(showArgv("aaaa111"), patchN(1)))
	h.send(push("git log --graph --color=always refs/heads/main --", "* abc main\n"))
	if !h.m.watch.dimmed || !strings.Contains(h.screen(), "not followed") {
		t.Fatalf("branch log not dimmed\n%s", logb)
	}
	h.send(push("git -C /repo diff --no-ext-diff -- src", patchN(5)))
	if !h.m.watch.dimmed || h.fake.Calls() != 1 {
		t.Fatal("worktree followed by default")
	}
	h.send(push(showArgv("aaaa111"), patchN(1)))
	if h.m.watch.dimmed || h.fake.Calls() != 1 {
		t.Fatal("not undimmed")
	}
	h.m.cursor = 0
	h.send(push("git diff aaaa111^ aaaa111 -- README.md", "diff --git a/README.md b/README.md\n"))
	if h.m.cursor != 1 || h.fake.Calls() != 1 {
		t.Fatalf("cursor %d\n%s", h.m.cursor, logb)
	}
}

func TestWatchFollowWorktreeOption(t *testing.T) {
	h, _ := newWatchHarness(t, nil, true)
	h.m.watch.o.FollowWorktree = true
	h.send(push("git -C /repo diff --no-ext-diff -- src", patchN(5)))
	if h.m.doc == nil || h.m.doc.L.Ref != "worktree:src" || h.fake.Calls() != 1 {
		t.Fatal("worktree not followed with follow_worktree")
	}
}

func TestWatchAutoOffAndGroupNow(t *testing.T) {
	h, logb := newWatchHarness(t, nil, false)
	h.send(push(showArgv("aaaa111"), patchN(1)))
	if h.fake.Calls() != 0 || h.m.doc.state != stNeedsConfirm || !strings.Contains(h.screen(), "press g to group (~") || !strings.Contains(h.screen(), "auto off") {
		t.Fatalf("auto off sent or no hint:\n%s\n%s", h.screen(), logb)
	}
	h.keys("g")
	if h.fake.Calls() != 1 || h.m.doc.state != stGrouped {
		t.Fatal("g did not group")
	}
	h.send(push(showArgv("bbbb222"), patchN(2)))
	if h.fake.Calls() != 1 {
		t.Fatal("auto off sent the second diff")
	}
	h.keys("A")
	if h.fake.Calls() != 2 || !h.m.watch.auto {
		t.Fatal("A did not turn auto on and group")
	}
}

func TestWatchNeverAutoSendsAboveCeiling(t *testing.T) {
	h, _ := newWatchHarness(t, func(c *config.Config) { c.Ceiling = 10 }, true)
	h.send(push(showArgv("aaaa111"), patchN(1)))
	if h.fake.Calls() != 0 || h.m.ov == ovConfirm || !strings.Contains(h.screen(), "press g to group (~") {
		t.Fatalf("above the ceiling: calls %d ov %v\n%s", h.fake.Calls(), h.m.ov, h.screen())
	}
	h.keys("g")
	if h.fake.Calls() != 1 {
		t.Fatal("g above the ceiling did not send")
	}
}

func TestWatchEditsSurviveAndAreKeyedByDiff(t *testing.T) {
	h, _ := newWatchHarness(t, nil, true)
	h.send(push(showArgv("aaaa111"), patchN(1)))
	h.keys("n")
	h.typeText("Mine")
	h.keys("enter")
	h.send(push(showArgv("bbbb222"), patchN(2)))
	h.send(push(showArgv("aaaa111"), patchN(1)))
	if !h.m.doc.edited() || h.titles()[len(h.titles())-1] != "Mine" {
		t.Fatalf("edit lost on revisit: %v", h.titles())
	}
}

func TestWatchQuitClosesListener(t *testing.T) {
	dir := t.TempDir()
	_ = dir
	h, _ := newWatchHarness(t, nil, true)
	h.keys("q")
	if h.m.watch.ctx.Err() == nil {
		t.Fatal("q did not cancel the watch context")
	}
}
