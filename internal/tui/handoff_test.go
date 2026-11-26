package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/k22i/clump/internal/comment"
	"github.com/k22i/clump/internal/config"
	"github.com/k22i/clump/internal/handoff"
)

func stubHandoffBinaries(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir
}

func stubRexLogging(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "rex.log")
	sh := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"" + log + "\"\nexit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "rex"), []byte(sh), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

func loadHandoffComment(t *testing.T, h *harness, text string) {
	t.Helper()
	anchors := comment.Anchors(h.m.doc.L.Diff, h.m.doc.Hash, h.m.groupIDs())
	if len(anchors) == 0 {
		t.Fatal("no anchors")
	}
	for _, a := range anchors {
		if a.Kind == comment.KindLine {
			if _, err := h.store.AddComment(a.Key, text, h.m.o.Now()); err != nil {
				t.Fatal(err)
			}
			h.m.loadComments()
			return
		}
	}
}

func TestHandoffKeyWithoutCommentsFlashes(t *testing.T) {
	h := newHarness(t, nil, nil)
	h.load(testPatch)
	h.keys("H")
	if h.m.ov != ovNone {
		t.Fatalf("overlay = %v, want none", h.m.ov)
	}
	if !strings.Contains(h.m.flash, "no comments to hand off") {
		t.Errorf("flash = %q", h.m.flash)
	}
}

func TestHandoffMenuPicksAnAgentForReplace(t *testing.T) {
	stubHandoffBinaries(t, "claude")
	h := newHarness(t, nil, nil)
	h.load(testPatch)
	loadHandoffComment(t, h, "rewrite this line")
	h.keys("H")
	if h.m.ov != ovHandoff {
		t.Fatalf("overlay = %v, want ovHandoff", h.m.ov)
	}
	if !strings.Contains(h.m.handoff.title, "Hand off 1 comment") {
		t.Errorf("title = %q", h.m.handoff.title)
	}
	wantItems := len(h.m.o.Cfg.Agents) + 1
	if len(h.m.handoff.items) != wantItems {
		t.Fatalf("items = %d, want %d", len(h.m.handoff.items), wantItems)
	}
	if !strings.Contains(h.m.handoff.items[0].label, "claude — replace") {
		t.Errorf("first item = %q", h.m.handoff.items[0].label)
	}
	h.keys("enter")
	if h.m.ov != ovNone {
		t.Fatalf("overlay = %v after enter", h.m.ov)
	}
	req := h.m.request
	if req == nil || req.Where != handoff.WhereReplace || req.Agent != "claude" {
		t.Fatalf("request = %+v", req)
	}
	if len(req.Argv) != 2 || req.Argv[0] != "claude" || !strings.Contains(req.Argv[1], "rewrite this line") {
		t.Errorf("agent argv = %q", req.Argv)
	}
	if !strings.Contains(req.Prompt, "src/a.go") {
		t.Errorf("prompt lost the comment target:\n%s", req.Prompt)
	}
	if len(req.Anchors) == 0 {
		t.Error("request carries no anchors to stamp")
	}
}

func TestHandoffMenuPrintItemQuitsWithThePrompt(t *testing.T) {
	h := newHarness(t, nil, nil)
	h.load(testPatch)
	loadHandoffComment(t, h, "explain this")
	h.keys("H")
	h.keys("j", "j", "j")
	if got := h.m.handoff.items[h.m.handoff.cursor].name; got != handoffPrintItem {
		t.Fatalf("cursor on %q, want the print item", got)
	}
	h.keys("enter")
	req := h.m.request
	if req == nil || req.Where != handoff.WherePrint {
		t.Fatalf("request = %+v", req)
	}
	if !strings.Contains(req.Prompt, "explain this") {
		t.Errorf("prompt:\n%s", req.Prompt)
	}
}

func TestHandoffMenuEscCancels(t *testing.T) {
	h := newHarness(t, nil, nil)
	h.load(testPatch)
	loadHandoffComment(t, h, "x")
	h.keys("H")
	h.keys("esc")
	if h.m.ov != ovNone || h.m.request != nil {
		t.Fatalf("overlay = %v, request = %v", h.m.ov, h.m.request)
	}
}

func TestHandoffAgentMissingFromPathFlashes(t *testing.T) {
	h := newHarness(t, nil, func(c *config.Config) {
		c.Agents = []handoff.Agent{{Name: "ghost", Argv: []string{"clump-handoff-missing-cli"}, Where: handoff.WhereReplace}}
	})
	h.load(testPatch)
	loadHandoffComment(t, h, "x")
	h.keys("H")
	h.keys("enter")
	if h.m.request != nil {
		t.Fatal("quit was requested for an agent that is not on PATH")
	}
	if !strings.Contains(h.m.flash, "not on PATH") {
		t.Errorf("flash = %q", h.m.flash)
	}
}

func TestHandoffPaneLaunchesRexAndStamps(t *testing.T) {
	log := stubRexLogging(t)
	h := newHarness(t, nil, func(c *config.Config) {
		c.Agents = []handoff.Agent{{Name: "codex", Argv: []string{"codex"}, Where: handoff.WherePane}}
	})
	t.Setenv("REX_SESSION", "sess")
	t.Setenv("REX_BLOCK", "blk")
	h.load(testPatch)
	loadHandoffComment(t, h, "move this guard")
	h.m.o.Repo = t.TempDir()
	h.keys("H")
	h.keys("enter")
	if h.m.request != nil {
		t.Fatal("a pane handoff must not quit the TUI")
	}
	if !strings.Contains(h.m.flash, "handed off to codex") {
		t.Errorf("flash = %q", h.m.flash)
	}
	got, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	cmd := string(got)
	for _, want := range []string{"split", "-b", "blk", "--cwd", "codex", "move this guard"} {
		if !strings.Contains(cmd, want) {
			t.Errorf("rex argv missing %q: %s", want, cmd)
		}
	}
	placed := h.m.reportComments()
	if len(placed) != 1 {
		t.Fatalf("placed = %+v", placed)
	}
	stored, err := h.store.Comments([]string{placed[0].Anchor})
	if err != nil {
		t.Fatal(err)
	}
	if stored[placed[0].Anchor][0].HandedOff.IsZero() {
		t.Errorf("comments not stamped after the pane launch: %+v", stored)
	}
}

func TestHandoffPaneWithoutMultiplexerFlashes(t *testing.T) {
	h := newHarness(t, nil, func(c *config.Config) {
		c.Agents = []handoff.Agent{{Name: "codex", Argv: []string{"codex"}, Where: handoff.WherePane}}
	})
	t.Setenv("REX_SESSION", "")
	t.Setenv("REX_BLOCK", "")
	t.Setenv("TMUX", "")
	h.load(testPatch)
	loadHandoffComment(t, h, "x")
	h.keys("H")
	h.keys("enter")
	if h.m.request != nil || h.m.ov == ovHandoff {
		t.Fatalf("request = %v, overlay = %v", h.m.request, h.m.ov)
	}
	if !strings.Contains(h.m.flash, "no Rex or tmux session") {
		t.Errorf("flash = %q", h.m.flash)
	}
}

func TestHandoffWatchTurnsReplaceIntoPaneAndHidesPrint(t *testing.T) {
	stubHandoffBinaries(t, "claude")
	h := newHarness(t, nil, nil)
	h.load(testPatch)
	loadHandoffComment(t, h, "x")
	h.m.watch = newWatchState(&WatchOptions{Repo: "/repo/root"})
	h.keys("H")
	for _, it := range h.m.handoff.items {
		if strings.Contains(it.label, handoffPrintItem) {
			t.Errorf("watch must not offer print: %q", it.label)
		}
		if it.name == "claude" && it.label != "claude — pane" {
			t.Errorf("watch relabels replace: %q", it.label)
		}
	}
	h.keys("enter")
	if h.m.request != nil {
		t.Fatal("watch must not quit with a replace request")
	}
	if !strings.Contains(h.m.flash, "no Rex or tmux session") {
		t.Errorf("flash = %q", h.m.flash)
	}
}
