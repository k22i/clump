package gitdiff

import "context"

// Loaded is a diff loaded from git or stdin, plus what the UI and the prompt
// need to describe it.
type Loaded struct {
	Diff *Diff
	// Raw is the patch text exactly as git printed it.
	Raw string
	// Context is the commit message(s) for the prompt, already capped
	// (MaxContextBytes). "" for a patch on stdin without a commit header.
	Context string
	// Title is a one-line description for the header: the commit subject, or
	// "<base>...<head> (N commits)".
	Title string
	// Ref is a short identity: a 12-char short hash, "<base>...<head>",
	// "<from>..<to>", or "stdin".
	Ref string
}

// MaxContextBytes caps Loaded.Context. Longer context is cut and ends with
// "\n[... truncated]\n".
const MaxContextBytes = 6000

// MaxRangeCommits caps how many commit messages LoadRange puts in Context.
const MaxRangeCommits = 60

// LogEntry is one commit in RecentCommits.
type LogEntry struct {
	Hash    string // full sha
	Short   string // 7+ char abbreviated sha (git's %h)
	Subject string
	RelDate string // git's %cr, e.g. "2 days ago"
}

// GitArgs are the arguments every diff-producing command gets, before the
// subcommand's own arguments. They pin the output format against the user's
// git config so the parser always sees a/ b/ prefixes and unquoted paths,
// and so the output matches what lazygit asks for.
var GitArgs = []string{
	"-c", "core.quotepath=off",
	"-c", "diff.noprefix=false",
	"-c", "diff.mnemonicPrefix=false",
	"-c", "diff.srcPrefix=a/",
	"-c", "diff.dstPrefix=b/",
	"-c", "color.ui=never",
}

// DiffFlags are added to every git show / git diff the loaders run.
var DiffFlags = []string{"--no-color", "--no-ext-diff", "--unified=3", "--find-renames", "--submodule"}

// RepoRoot returns the top-level directory of the work tree containing dir
// (git rev-parse --show-toplevel).
func RepoRoot(ctx context.Context, dir string) (string, error) {
	panic("gitdiff: RepoRoot not implemented")
}

// LoadRev loads one commit: git show --format= <DiffFlags>
// --diff-merges=first-parent <rev>, so a merge commit is diffed against its
// first parent. Context is "COMMIT MESSAGE:\n" + the full message; Title is
// the subject; Ref is the 12-char short hash.
func LoadRev(ctx context.Context, repo, rev string) (*Loaded, error) {
	panic("gitdiff: LoadRev not implemented")
}

// LoadRange loads a merge-base comparison like a pull request:
// git diff <DiffFlags> <base>...<head>. Context is
// "COMMITS IN THIS RANGE (newest first):\n" + up to MaxRangeCommits messages
// from git log --no-merges <base>..<head>. Title is "<base>...<head> (N commits)".
func LoadRange(ctx context.Context, repo, base, head string) (*Loaded, error) {
	panic("gitdiff: LoadRange not implemented")
}

// LoadBranch is LoadRange(DefaultBase(repo), "HEAD"), with Title
// "<branch> vs <base> (N commits)".
func LoadBranch(ctx context.Context, repo string) (*Loaded, error) {
	panic("gitdiff: LoadBranch not implemented")
}

// LoadTwoDot loads git diff <DiffFlags> <from> <to>: a plain two-tree
// comparison, as lazygit's range selection and diffing mode show it. Context
// lists the commit messages of git log --no-merges <from>..<to> (capped as in
// LoadRange); Ref is "<short from>..<short to>".
func LoadTwoDot(ctx context.Context, repo, from, to string) (*Loaded, error) {
	panic("gitdiff: LoadTwoDot not implemented")
}

// LoadPatch parses a patch read from stdin. If it has a commit header, Title
// and Context come from it; otherwise Title is "patch from stdin".
func LoadPatch(text string) (*Loaded, error) {
	panic("gitdiff: LoadPatch not implemented")
}

// DefaultBase returns the branch to compare the current branch against:
// origin/HEAD's target (e.g. "origin/main"), else "main", else "master".
// It errors if none exists.
func DefaultBase(ctx context.Context, repo string) (string, error) {
	panic("gitdiff: DefaultBase not implemented")
}

// CurrentBranch returns the checked-out branch name, or "HEAD" when detached.
func CurrentBranch(ctx context.Context, repo string) (string, error) {
	panic("gitdiff: CurrentBranch not implemented")
}

// ResolveCommit resolves any revision (a sha, a branch, refs/stash@{2}) to a
// full commit sha (git rev-parse --verify <ref>^{commit}).
func ResolveCommit(ctx context.Context, repo, ref string) (string, error) {
	panic("gitdiff: ResolveCommit not implemented")
}

// RecentCommits returns the last n commits reachable from HEAD, newest first.
func RecentCommits(ctx context.Context, repo string, n int) ([]LogEntry, error) {
	panic("gitdiff: RecentCommits not implemented")
}
