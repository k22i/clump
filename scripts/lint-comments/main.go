package main

import (
	"bufio"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	baselinePath = "scripts/lint-comments/baseline.txt"
	buildTagPfx  = "//go:build"
	embedPfx     = "//go:embed"
)

var vendored = []string{
	"internal/chromalexers",
}

func main() {
	root := flag.String("root", ".", "module root to scan")
	baseline := flag.String("baseline", baselinePath, "ratchet baseline, relative to the module root")
	update := flag.Bool("update", false, "rewrite the baseline with the current counts (may only lower them)")
	adopt := flag.Bool("adopt", false, "with -update, allow files to enter the baseline for the first time")
	flag.Parse()

	dir := moduleRoot(*root)
	counts, err := scan(dir)
	if err != nil {
		fail("%v", err)
	}

	if *update {
		if err := rewrite(dir, filepath.Join(dir, *baseline), counts, *adopt); err != nil {
			fail("%v", err)
		}
		fmt.Printf("baseline updated: %d files, %d comments grandfathered\n", len(counts), total(counts))
		return
	}

	allowed, err := read(dir, filepath.Join(dir, *baseline))
	if err != nil {
		fail("%v", err)
	}

	var bad []string
	for file, n := range counts {
		if n > allowed[file] {
			bad = append(bad, fmt.Sprintf("%s: %d comment(s), baseline allows %d", file, n, allowed[file]))
		}
	}
	sort.Strings(bad)
	if len(bad) > 0 {
		fmt.Fprintln(os.Stderr, "comments are banned in clump's Go source (see docs/adr/0039-no-comments-in-source.md):")
		for _, b := range bad {
			fmt.Fprintln(os.Stderr, "  "+b)
		}
		fmt.Fprintf(os.Stderr, "\n%d file(s) over baseline; %d comment(s) across the tree, %d allowed\n", len(bad), total(counts), total(allowed))
		os.Exit(1)
	}
	fmt.Printf("ok: %d comment(s) across the tree, all grandfathered\n", total(counts))
}

func moduleRoot(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		fail("%v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(abs, "go.mod")); err == nil {
			return abs
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return abs
		}
		abs = parent
	}
}

func scan(root string) (map[string]int, error) {
	counts := map[string]int{}
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, p)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if skipDir(rel) {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || skipFile(rel) {
			return nil
		}
		n, err := count(fset, p)
		if err != nil {
			return err
		}
		if n > 0 {
			counts[rel] = n
		}
		return nil
	})
	return counts, err
}

func skipDir(rel string) bool {
	for _, v := range vendored {
		if rel == v || strings.HasPrefix(rel, v+"/") {
			return true
		}
	}
	return false
}

func skipFile(rel string) bool {
	for _, v := range vendored {
		if strings.HasPrefix(rel, v+"/") {
			return true
		}
	}
	return strings.HasPrefix(rel, "spike/")
}

func count(fset *token.FileSet, path string) (int, error) {
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return 0, err
	}
	if ast.IsGenerated(f) {
		return 0, nil
	}
	n := 0
	for _, cg := range f.Comments {
		for _, c := range cg.List {
			if !exempt(c.Text) {
				n++
			}
		}
	}
	return n, nil
}

func exempt(text string) bool {
	if strings.Contains(text, "nolint") {
		return true
	}
	switch {
	case strings.HasPrefix(text, buildTagPfx), strings.HasPrefix(text, embedPfx),
		strings.HasPrefix(text, "//go:"), strings.HasPrefix(text, "//line "),
		strings.HasPrefix(text, "//export "), strings.HasPrefix(text, "//+build"):
		return true
	}
	return false
}

func read(root, path string) (map[string]int, error) {
	out := map[string]int{}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		i := strings.LastIndex(line, " ")
		if i < 0 {
			return nil, fmt.Errorf("%s: bad line %q", path, line)
		}
		n, err := strconv.Atoi(strings.TrimSpace(line[i+1:]))
		if err != nil {
			return nil, fmt.Errorf("%s: bad line %q", path, line)
		}
		out[strings.TrimSpace(line[:i])] = n
	}
	return out, sc.Err()
}

func rewrite(root, path string, counts map[string]int, adopt bool) error {
	old, err := read(root, path)
	if err != nil {
		return err
	}
	for file, n := range old {
		if now, ok := counts[file]; ok && now > n {
			return fmt.Errorf("%s: would raise the baseline from %d to %d; the ratchet only turns one way — delete the new comments", file, n, now)
		}
	}
	if !adopt {
		for file := range counts {
			if _, listed := old[file]; !listed {
				return fmt.Errorf("%s: %d comment(s) in a file the baseline does not list; the ratchet only turns one way — delete them, or pass -adopt to grandfather them deliberately", file, counts[file])
			}
		}
	}
	var b strings.Builder
	b.WriteString("# Comment ratchet: the most comments each file may still carry.\n")
	b.WriteString("# Lower a count (or drop the line) as a file is cleaned; `-update` refuses to raise one.\n")
	files := make([]string, 0, len(counts))
	for file := range counts {
		files = append(files, file)
	}
	sort.Strings(files)
	for _, file := range files {
		fmt.Fprintf(&b, "%s %d\n", file, counts[file])
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func total(m map[string]int) int {
	n := 0
	for _, v := range m {
		n += v
	}
	return n
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "lint-comments: "+format+"\n", args...)
	os.Exit(2)
}
