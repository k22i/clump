// Command clump groups the hunks of a git diff into logical changes with an
// LLM and lets you review and edit them in a TUI. See README.md.
//
//	clump [flags] [<rev> | <base>...<head> | <from>..<to>]   one commit or range (default HEAD)
//	clump --branch                                          this branch vs its default base
//	git diff | clump                                        a patch on stdin
//	clump watch [--repo DIR]                                follow lazygit's selection
//	clump push [--then CMD]                                 lazygit diff filter (forwards to watch)
//	clump config init | path                                print the default config / its path
//	clump rex install [--write]                             Rex integration
//	clump lazygit install [--write]                         lazygit integration
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/k22i/clump/internal/provider"
)

var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// usageError is reported with exit status 2.
type usageError struct{ msg string }

func (e usageError) Error() string { return e.msg }

func run(args []string, stdin *os.File, stdout, stderr io.Writer) int {
	var err error
	sub := ""
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "push":
		return runPush(args[1:], stdin, stdout)
	case "watch":
		err = runWatch(args[1:], stderr)
	case "config":
		err = runConfig(args[1:], stdout)
	case "rex":
		err = runRex(args[1:], stdin, stdout)
	case "lazygit":
		err = runLazygit(args[1:], stdin, stdout)
	case "version", "--version":
		fmt.Fprintln(stdout, "clump", version)
		return 0
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		err = runOneShot(args, stdin, stdout, stderr)
	}
	if err == nil {
		return 0
	}
	var ue usageError
	if errors.As(err, &ue) {
		fmt.Fprintf(stderr, "clump: %s\n\n%s", ue.msg, usage)
		return 2
	}
	fmt.Fprintf(stderr, "clump: %s\n", provider.Redact(err.Error()))
	return 1
}

const usage = `usage:
  clump [flags] [<rev> | <base>...<head> | <from>..<to>]
  clump --branch [flags]
  git diff | clump [flags]
  clump watch [--repo DIR] [--log FILE]
  clump push [--then 'CMD']
  clump config init | path
  clump rex install [--write]
  clump lazygit install [--write]

flags:
  --repo DIR        repository (default: the current directory)
  --branch          compare the current branch with its default base
  --provider NAME   provider or alias (default: config "default")
  --json            print the groups as JSON
  --plain           print a Markdown summary
  --dry-run         print exactly what would be sent, send nothing
  --no-cache        ignore cached results (still writes the cache)
  --yes             do not ask before sending above the ceiling
  --pick            choose a commit or branch comparison first
`

func runConfig(args []string, stdout io.Writer) error {
	if len(args) != 1 {
		return usageError{"config: want init or path"}
	}
	switch args[0] {
	case "init":
		_, err := io.WriteString(stdout, configDefaultTOML())
		return err
	case "path":
		p, err := configPath()
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, p)
		return nil
	}
	return usageError{"config: want init or path, got " + strings.Join(args, " ")}
}
