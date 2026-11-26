package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/k22i/clump/internal/comment"
)

// stubSubmitGh puts a gh on PATH that records "api" invocations and answers
// like a posted review. It fails when $CLUMP_GH_FAIL is set.
func stubSubmitGh(t *testing.T) (logPath string) {
	t.Helper()
	dir := t.TempDir()
	logPath = filepath.Join(dir, "gh.log")
	outPath := filepath.Join(dir, "api.json")
	if err := os.WriteFile(outPath, []byte(`{"html_url":"https://github.com/k22i/clump/pull/42#pullrequestreview-7","id":7}`), 0o644); err != nil {
		t.Fatal(err)
	}
	sh := `#!/bin/sh
[ -n "$CLUMP_GH_FAIL" ] && { echo "$CLUMP_GH_FAIL" >&2; exit 1; }
if [ "$1" = api ]; then
  cat > /dev/null
  printf '%s\n' "$*" >> "$CLUMP_TUI_GH_LOG"
  cat "$CLUMP_TUI_GH_OUT"
fi
`
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(sh), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CLUMP_TUI_GH_LOG", logPath)
	t.Setenv("CLUMP_TUI_GH_OUT", outPath)
	return logPath
}

func submitLog(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, l := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

func TestSubmitNeedsPR(t *testing.T) {
	h := newHarness(t, nil, nil)
	h.load(testPatch)
	h.keys("P")
	if h.m.ov != ovNone {
		t.Fatalf("overlay = %v, want none", h.m.ov)
	}
	if !strings.Contains(h.m.flash, "clump --pr") {
		t.Errorf("flash = %q", h.m.flash)
	}
}

func TestSubmitWithoutComments(t *testing.T) {
	h := newHarness(t, nil, nil)
	h.m.o.PR = "42"
	h.load(testPatch)
	h.keys("P")
	if h.m.ov != ovNone {
		t.Fatalf("overlay = %v, want none", h.m.ov)
	}
	if !strings.Contains(h.m.flash, "no comments") {
		t.Errorf("flash = %q", h.m.flash)
	}
}

func TestSubmitPicksAndPosts(t *testing.T) {
	logPath := stubSubmitGh(t)
	h := newHarness(t, nil, nil)
	h.m.o.PR = "k22i/clump#42"
	h.load(testPatch)
	// Comment on the diff's first added line directly through the store.
	anchors := comment.Anchors(h.m.doc.L.Diff, h.m.doc.Hash, h.m.groupIDs())
	if len(anchors) == 0 {
		t.Fatal("no anchors")
	}
	var line *comment.Anchor
	for i := range anchors {
		if anchors[i].Kind == comment.KindLine && anchors[i].Side == comment.SideNew {
			line = &anchors[i]
			break
		}
	}
	if line == nil {
		t.Fatal("no new-side line anchor")
	}
	if _, err := h.store.AddComment(line.Key, "needs a comment", h.m.o.Now()); err != nil {
		t.Fatal(err)
	}
	h.m.loadComments()

	h.keys("P")
	if h.m.ov != ovSubmit {
		t.Fatalf("overlay = %v, want ovSubmit", h.m.ov)
	}
	if len(h.m.submit.items) != 3 {
		t.Fatalf("items = %d, want 3", len(h.m.submit.items))
	}
	if !strings.Contains(h.m.submit.title, "1 line comment") {
		t.Errorf("title = %q", h.m.submit.title)
	}
	h.keys("j", "j") // request changes
	h.keys("enter")
	if !strings.Contains(h.m.flash, "request changes review submitted: https://github.com/k22i/clump/pull/42") {
		t.Errorf("flash = %q", h.m.flash)
	}
	if h.m.flashErr {
		t.Errorf("flashErr = true")
	}
	got := submitLog(t, logPath)
	if len(got) != 1 || got[0] != "api --method POST --input - repos/k22i/clump/pulls/42/reviews" {
		t.Fatalf("gh called with %q", got)
	}
}

func TestSubmitEscCancels(t *testing.T) {
	stubSubmitGh(t)
	h := newHarness(t, nil, nil)
	h.m.o.PR = "42"
	h.load(testPatch)
	anchors := comment.Anchors(h.m.doc.L.Diff, h.m.doc.Hash, h.m.groupIDs())
	if _, err := h.store.AddComment(anchors[0].Key, "x", h.m.o.Now()); err != nil {
		t.Fatal(err)
	}
	h.m.loadComments()
	h.keys("P")
	h.keys("esc")
	if h.m.ov != ovNone {
		t.Fatalf("overlay = %v, want none", h.m.ov)
	}
}

func TestSubmitFailureFlashesError(t *testing.T) {
	stubSubmitGh(t)
	t.Setenv("CLUMP_GH_FAIL", "no permission")
	h := newHarness(t, nil, nil)
	h.m.o.PR = "42"
	h.load(testPatch)
	anchors := comment.Anchors(h.m.doc.L.Diff, h.m.doc.Hash, h.m.groupIDs())
	if _, err := h.store.AddComment(anchors[0].Key, "x", h.m.o.Now()); err != nil {
		t.Fatal(err)
	}
	h.m.loadComments()
	h.keys("P")
	h.keys("enter")
	if !h.m.flashErr || !strings.Contains(h.m.flash, "no permission") {
		t.Errorf("flash = %q (err %v)", h.m.flash, h.m.flashErr)
	}
}
