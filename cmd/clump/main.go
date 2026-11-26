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
	case "doctor":
		err = runDoctor(stdout)
	case "comments":
		err = runComments(args[1:], stdin, stdout, stderr)
	case "submit":
		err = runSubmit(args[1:], stdin, stdout, stderr)
	case "version", "--version":
		fmt.Fprintln(stdout, "clump", version)
		return 0
	case "update", "upgrade":
		err = runUpdate(args[1:], stdout, stderr)
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
  clump --pr 123 | --pr OWNER/REPO#123 | --pr <pr-url> [flags]
  git diff | clump [flags]
  clump watch [--repo DIR] [--log FILE]
  clump push [--then 'CMD']
  clump comments [REV | RANGE | --branch | --pr SPEC] [--json]
  clump comments add --file PATH [--line N --side old|new] [--rev …] TEXT
  clump comments delete --id ID
  clump submit --pr SPEC --event comment|approve|request-changes \
    [--body TEXT | --body-file PATH] [--yes] [--dry-run]
  clump config init | path
  clump rex install [--write]
  clump lazygit install [--write]
  clump update | upgrade

update:
  clump update downloads the latest release published on GitHub and
  replaces this binary with it — the same prebuilt, checksummed archive
  scripts/install.sh installs, no Go toolchain needed. It says so and
  does nothing when this binary is already the latest release.

comments:
  clump comments lists a diff's stored review comments, numbered, or as
  JSON with --json; add and delete write them for scripts and agents that
  cannot run the TUI. The target is a rev, a range, --branch or --pr for
  the list, --rev/--branch/--pr flags for add, a patch on stdin for both.

  --repo DIR        repository (default: the current directory)
  --json            list as JSON (else print one added comment)
  --file PATH       comment on this file of the diff
  --line N          comment on one line (default: the whole file)
  --side old|new    which side --line N names (default new)
  --group ID        comment on one group of the saved grouping
  --rev REV         the diff to comment on (add, delete)

submit:
  clump submit posts the pull request's stored review comments back to
  GitHub as one review, through the gh CLI: line comments inline on their
  line, file comments on the file, group comments in the review body. In
  the TUI (opened with --pr) P picks the event instead; nothing is posted
  without a confirm.

  --event EVENT     comment, approve or request-changes
  --body TEXT       the review's summary text
  --body-file PATH  the summary from a file ("-" is stdin)
  --yes             post without the confirm question
  --dry-run         print the JSON that would be posted, send nothing

flags:
  --repo DIR        repository (default: the current directory)
  --branch          compare the current branch with its default base
  --pr SPEC         a GitHub pull request: 123, #123, OWNER/REPO#123 or its
                    URL (needs the gh CLI; a number is looked up in
                    origin's repository, the other forms work anywhere)
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
