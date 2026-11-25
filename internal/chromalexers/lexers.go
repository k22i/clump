package chromalexers

import (
	"embed"
	"io/fs"
	"sync"

	"github.com/alecthomas/chroma/v2"
)

//go:embed embedded
var embedded embed.FS

// The registry is built on first use instead of at package init: parsing
// the 279 embedded lexer configs takes ~3.5 ms, which every invocation of
// the clump binary would otherwise pay (including `clump push`, which must
// stay within a few milliseconds of cat). This is the only change from
// upstream chroma/v2/lexers (MIT, see LICENSE), apart from the package
// name, the three analyser init functions (dns.go, mysql.go, zed.go), which
// now run when the registry is built, and keyed Rule literals (go vet).
var (
	registryOnce sync.Once
	globalReg    *chroma.LexerRegistry
	pendingMu    sync.Mutex
	pending      []chroma.Lexer
)

// GlobalLexerRegistry returns the global LexerRegistry of Lexers, building
// it on first use.
func GlobalLexerRegistry() *chroma.LexerRegistry {
	registryOnce.Do(func() {
		reg := chroma.NewLexerRegistry()
		paths, err := fs.Glob(embedded, "embedded/*.xml")
		if err != nil {
			panic(err)
		}
		for _, path := range paths {
			reg.Register(chroma.MustNewXMLLexer(embedded, path))
		}
		pendingMu.Lock()
		for _, l := range pending {
			reg.Register(l)
		}
		pending = nil
		// Upstream ran these as package init functions (which forced the
		// whole registry to load at startup).
		analyserDNS(reg)
		analyserMySQL(reg)
		analyserZed(reg)
		globalReg = reg
		pendingMu.Unlock()
	})
	return globalReg
}

// Names of all lexers, optionally including aliases.
func Names(withAliases bool) []string {
	return GlobalLexerRegistry().Names(withAliases)
}

// Aliases of all the lexers, and skip those lexers who do not have any aliases,
// or show their name instead
func Aliases(skipWithoutAliases bool) []string {
	return GlobalLexerRegistry().Aliases(skipWithoutAliases)
}

// Get a Lexer by name, alias or file extension.
//
// Note that this if there isn't an exact match on name or alias, this will
// call Match(), so it is not efficient.
func Get(name string) chroma.Lexer {
	return GlobalLexerRegistry().Get(name)
}

// MatchMimeType attempts to find a lexer for the given MIME type.
func MatchMimeType(mimeType string) chroma.Lexer {
	return GlobalLexerRegistry().MatchMimeType(mimeType)
}

// Match returns the first lexer matching filename.
//
// Note that this iterates over all file patterns in all lexers, so it's not
// particularly efficient.
func Match(filename string) chroma.Lexer {
	return GlobalLexerRegistry().Match(filename)
}

// Register a Lexer with the global registry.
func Register(lexer chroma.Lexer) chroma.Lexer {
	pendingMu.Lock()
	if globalReg == nil {
		pending = append(pending, lexer)
		pendingMu.Unlock()
		return lexer
	}
	pendingMu.Unlock()
	return GlobalLexerRegistry().Register(lexer)
}

// Analyse text content and return the "best" lexer..
func Analyse(text string) chroma.Lexer {
	return GlobalLexerRegistry().Analyse(text)
}

// PlaintextRules is used for the fallback lexer as well as the explicit
// plaintext lexer.
func PlaintextRules() chroma.Rules {
	return chroma.Rules{
		"root": []chroma.Rule{
			{Pattern: `.+`, Type: chroma.Text, Mutator: nil},
			{Pattern: `\n`, Type: chroma.Text, Mutator: nil},
		},
	}
}

// Fallback lexer if no other is found.
var Fallback chroma.Lexer = chroma.MustNewLexer(&chroma.Config{
	Name:      "fallback",
	Filenames: []string{"*"},
	Priority:  -1,
}, PlaintextRules)
