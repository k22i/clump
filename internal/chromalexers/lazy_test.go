package chromalexers

import "testing"

// The registry must still find every kind of lexer: XML ones, Go-coded
// ones (Go, Markdown), and the analysers that used to be init functions.
func TestLazyRegistry(t *testing.T) {
	for file, want := range map[string]string{"a.go": "Go", "README.md": "markdown", "x.py": "Python", "y.ts": "TypeScript", "z.rs": "Rust", "c.yaml": "YAML"} {
		l := Match(file)
		if l == nil || l.Config().Name != want {
			t.Errorf("Match(%q) = %v, want %s", file, l, want)
		}
	}
	if Get("dns") == nil || Get("mysql") == nil || Get("Zed") == nil {
		t.Fatal("analyser lexers missing")
	}
	if len(Names(false)) < 250 {
		t.Fatalf("only %d lexers", len(Names(false)))
	}
	it, err := Match("a.go").Tokenise(nil, "package main\n")
	if err != nil || it().Value == "" {
		t.Fatalf("tokenise: %v", err)
	}
}
