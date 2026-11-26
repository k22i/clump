package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func handoffRepo(t *testing.T) string {
	t.Helper()
	dir := gitRepo(t)
	if _, err := commentRun(t, "add", "--repo", dir, "--rev", "HEAD", "--file", "a.txt", "--line", "2", "name the constant"); err != nil {
		t.Fatal(err)
	}
	if _, err := commentRun(t, "add", "--repo", dir, "--rev", "HEAD", "--file", "a.txt", "needs a test"); err != nil {
		t.Fatal(err)
	}
	return dir
}

func handoffRun(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out strings.Builder
	err := runHandoff(args, os.Stdin, &out, &out)
	return out.String(), err
}

func stubHandoffAgent(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "agent.log")
	sh := "#!/bin/sh\nprintf '%s\\n' \"$*\" > \"" + log + "\"\n"
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte(sh), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

func missingAgentConfig(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	conf := "[agents.ghost]\nargv = [\"clump-handoff-missing-cli\"]\nwhere = \"replace\"\n"
	if err := os.WriteFile(p, []byte(conf), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLUMP_CONFIG", p)
}

func TestParseHandoffFlags(t *testing.T) {
	o, err := parseHandoffFlags([]string{"HEAD", "--agent", "opencode", "--where", "print"})
	if err != nil {
		t.Fatal(err)
	}
	if len(o.args) != 1 || o.agent != "opencode" || o.where != "print" {
		t.Fatalf("opts = %+v", o)
	}
	if _, err := parseHandoffFlags([]string{"--where", "sidecar"}); err == nil ||
		!strings.Contains(err.Error(), "--where must be replace, pane or print") {
		t.Fatalf("bad where: %v", err)
	}
	if _, err := parseHandoffFlags([]string{"a", "b"}); err == nil || !strings.Contains(err.Error(), "at most one revision") {
		t.Fatalf("two revs: %v", err)
	}
	if _, err := parseHandoffFlags([]string{"--branch", "--pr", "1"}); err == nil {
		t.Fatal("--branch with --pr should fail")
	}
	if _, err := parseHandoffFlags([]string{"--pr", "1", "HEAD"}); err == nil {
		t.Fatal("--pr with a rev should fail")
	}
	c := handoffOpts{pr: "42", branch: true, args: []string{"x"}}.diffOpts()
	if c.pr != "42" || !c.branch || len(c.args) != 1 {
		t.Fatalf("diffOpts = %+v", c)
	}
}

func TestHandoffWithoutComments(t *testing.T) {
	dir := gitRepo(t)
	_, err := handoffRun(t, "--repo", dir, "HEAD", "--where", "print")
	if err == nil || !strings.Contains(err.Error(), "nothing to hand off") {
		t.Fatalf("err = %v", err)
	}
}

func TestHandoffPrintCarriesWholeReviewAndStamps(t *testing.T) {
	dir := handoffRepo(t)
	out, err := handoffRun(t, "--repo", dir, "HEAD", "--where", "print", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Address these review comments on", "(second):", "1. a.txt:2 (new side): name the constant", "2. a.txt (file): needs a test", "line number"} {
		if !strings.Contains(out, want) {
			t.Errorf("prompt is missing %q:\n%s", want, out)
		}
	}
	list, _ := commentRun(t, "--repo", dir, "--rev", "HEAD")
	if !strings.Contains(list, "(handed off)") {
		t.Errorf("comments not stamped:\n%s", list)
	}
	openJson, _ := commentRun(t, "--repo", dir, "--rev", "HEAD", "--json")
	if !strings.Contains(openJson, `"handed_off"`) {
		t.Errorf("json list lost the stamp:\n%s", openJson)
	}
	openList, _ := commentRun(t, "--repo", dir, "--rev", "HEAD", "--open")
	if strings.Contains(openList, "name the constant") || strings.Contains(openList, "needs a test") {
		t.Errorf("--open still lists handed-off comments:\n%s", openList)
	}
}

func TestHandoffDryRunPrintsCommandAndStampsNothing(t *testing.T) {
	dir := handoffRepo(t)
	stubHandoffAgent(t)
	out, err := handoffRun(t, "--repo", dir, "HEAD", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "handoff to claude (replace)") || !strings.Contains(out, "(new side): name the constant") {
		t.Errorf("dry run:\n%s", out)
	}
	open, _ := commentRun(t, "--repo", dir, "--rev", "HEAD", "--open")
	if !strings.Contains(open, "name the constant") {
		t.Errorf("dry-run stamped the comments:\n%s", open)
	}
}

func TestHandoffReplaceArgvAndPreExecStamp(t *testing.T) {
	dir := handoffRepo(t)
	log := stubHandoffAgent(t)
	var gotPath string
	var gotArgv []string
	var gotEnv []string
	oldExec := syscallExec
	syscallExec = func(path string, argv []string, env []string) error {
		gotPath, gotArgv, gotEnv = path, argv, env
		return nil
	}
	oldTTY := handoffTerminal
	handoffTerminal = func(*os.File) bool { return true }
	t.Cleanup(func() {
		syscallExec = oldExec
		handoffTerminal = oldTTY
	})
	_, err := handoffRun(t, "--repo", dir, "HEAD", "--where", "replace", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(gotPath, "claude") {
		t.Errorf("exec path = %q", gotPath)
	}
	if len(gotArgv) != 2 || gotArgv[0] != "claude" || !strings.Contains(gotArgv[1], "name the constant") {
		t.Errorf("exec argv = %q", gotArgv)
	}
	if len(gotEnv) == 0 {
		t.Error("exec env should be inherited")
	}
	if _, err := os.Stat(log); err == nil {
		t.Error("the stub agent should not have been run")
	}
	open, _ := commentRun(t, "--repo", dir, "--rev", "HEAD", "--open")
	if strings.Contains(open, "name the constant") {
		t.Errorf("replace did not stamp before exec:\n%s", open)
	}
}

func TestHandoffReplaceRefusesAStubbedStdin(t *testing.T) {
	dir := handoffRepo(t)
	stubHandoffAgent(t)
	oldTTY := handoffTerminal
	handoffTerminal = func(*os.File) bool { return false }
	t.Cleanup(func() { handoffTerminal = oldTTY })
	_, err := handoffRun(t, "--repo", dir, "HEAD", "--where", "replace", "--yes")
	if err == nil || !strings.Contains(err.Error(), "replace needs a terminal on stdin") {
		t.Fatalf("err = %v", err)
	}
}

func TestHandoffPrintFallbackOnMissingAgent(t *testing.T) {
	dir := handoffRepo(t)
	missingAgentConfig(t)
	out, err := handoffRun(t, "--repo", dir, "HEAD", "--agent", "ghost", "--yes")
	if err == nil || !strings.Contains(err.Error(), "not found in $PATH") {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(out, "could not hand off to ghost") || !strings.Contains(out, "name the constant") {
		t.Errorf("fallback output:\n%s", out)
	}
	if !strings.Contains(out, "clump-handoff-missing-cli") {
		t.Errorf("fallback should name the exact command:\n%s", out)
	}
}

func TestHandoffPaneWithoutMultiplexer(t *testing.T) {
	dir := handoffRepo(t)
	stubHandoffAgent(t)
	t.Setenv("REX_SESSION", "")
	t.Setenv("REX_BLOCK", "")
	t.Setenv("TMUX", "")
	_, err := handoffRun(t, "--repo", dir, "HEAD", "--where", "pane", "--yes")
	if err == nil || !strings.Contains(err.Error(), "no Rex or tmux session") {
		t.Fatalf("err = %v", err)
	}
}

func TestHandoffPrintUsesTheNamedAgentWithoutLaunchingIt(t *testing.T) {
	dir := handoffRepo(t)
	missingAgentConfig(t)
	out, err := handoffRun(t, "--repo", dir, "HEAD", "--agent", "ghost", "--where", "print", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "needs a test") {
		t.Errorf("print handoff:\n%s", out)
	}
	list, _ := commentRun(t, "--repo", dir, "--rev", "HEAD")
	if !strings.Contains(list, "(handed off)") {
		t.Errorf("comments not stamped:\n%s", list)
	}
}

func fakeProviderConfig(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	conf := "handoff = \"claude\"\ncheap = \"fake\"\ndefault = \"fake\"\n[providers.fake]\ntype = \"fake\"\n"
	if err := os.WriteFile(p, []byte(conf), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLUMP_CONFIG", p)
}

func TestHandoffPlainAndJsonCarryComments(t *testing.T) {
	dir := handoffRepo(t)
	fakeProviderConfig(t)
	var plain strings.Builder
	stdin, err := os.OpenFile(os.DevNull, os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	if err := runOneShot([]string{"--repo", dir, "HEAD", "--plain"}, stdin, &plain, &plain); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plain.String(), "1. `a.txt:2` (new side) — name the constant") {
		t.Errorf("plain summary lost the comments:\n%s", plain.String())
	}
	var js strings.Builder
	if err := runOneShot([]string{"--repo", dir, "HEAD", "--json"}, stdin, &js, &js); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(js.String(), `"comments"`) || !strings.Contains(js.String(), "name the constant") {
		t.Errorf("json lost the comments:\n%s", js.String())
	}
}

func TestHandoffPrintWithoutAnyResolvableAgent(t *testing.T) {
	dir := handoffRepo(t)
	missingAgentConfig(t)
	out, err := handoffRun(t, "--repo", dir, "HEAD", "--where", "print", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "needs a test") {
		t.Errorf("print must work with no agent installed:\n%s", out)
	}
	list, _ := commentRun(t, "--repo", dir, "--rev", "HEAD")
	if !strings.Contains(list, "(handed off)") {
		t.Errorf("comments not stamped:\n%s", list)
	}
	stubHandoffAgent(t)
	oldExec, oldTTY := syscallExec, handoffTerminal
	execArgv := ""
	syscallExec = func(path string, argv []string, env []string) error {
		execArgv = strings.Join(argv, "\x00")
		return nil
	}
	handoffTerminal = func(*os.File) bool { return true }
	t.Cleanup(func() {
		syscallExec = oldExec
		handoffTerminal = oldTTY
	})
	if _, err := handoffRun(t, "--repo", dir, "HEAD", "--yes"); err != nil {
		t.Fatalf("auto resolution should land on an agent, err = %v", err)
	}
	if !strings.Contains(execArgv, "needs a test") {
		t.Errorf("exec argv = %q", execArgv)
	}
}
