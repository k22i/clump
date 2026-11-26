// Command clump update downloads the latest release published on GitHub
// and replaces this binary with it: the same prebuilt, checksummed archive
// scripts/install.sh installs, without a Go toolchain. clump stores no
// tokens: the GitHub API call is anonymous, or authenticated with
// $GITHUB_TOKEN, else the gh CLI's token when gh is logged in — like the
// other GitHub integrations.
package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/k22i/clump/internal/update"
)

// updateAPI and updateRepo are where releases come from; tests point them
// at a fake server. selfExe is the binary update.Install replaces.
var (
	updateAPI  = update.API
	updateRepo = update.Repo
	selfExe    = executablePath
)

func executablePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return exe, nil
}

// runUpdate implements clump update (and its upgrade alias): fetch the
// latest release, and when it is newer than this binary, download this
// platform's archive, verify its checksum and install it in place.
func runUpdate(args []string, stdout, stderr io.Writer) error {
	if len(args) > 0 {
		return usageError{"update: want no arguments, got " + strings.Join(args, " ")}
	}
	// Ctrl-C interrupts a download; the install itself is a rename.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	exe, err := selfExe()
	if err != nil {
		return fmt.Errorf("update: %w", err)
	}
	client := authClient(ctx)
	rel, err := update.Latest(ctx, client, updateAPI, updateRepo)
	if err != nil {
		return fmt.Errorf("update: %w", err)
	}
	if version != "dev" && !update.Newer(version, rel.Tag) {
		fmt.Fprintf(stdout, "clump %s is up to date\n", version)
		return nil
	}
	fmt.Fprintf(stderr, "update: downloading clump %s…\n", rel.Tag)
	bin, err := update.Download(ctx, client, rel, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return fmt.Errorf("update: %w", err)
	}
	if err := update.Install(exe, bin); err != nil {
		return fmt.Errorf("update: %w: need write access to %s", err, exe)
	}
	if version == "dev" {
		fmt.Fprintf(stdout, "clump dev → %s (%s)\n", rel.Tag, exe)
	} else {
		fmt.Fprintf(stdout, "clump %s → %s (%s)\n", version, rel.Tag, exe)
	}
	return nil
}

// authClient returns a client for the GitHub API, authenticated with
// $GITHUB_TOKEN, else the token of a logged-in gh CLI, else anonymous.
func authClient(ctx context.Context) *http.Client {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		if out, err := exec.CommandContext(ctx, "gh", "auth", "token").Output(); err == nil {
			token = strings.TrimSpace(string(out))
		}
	}
	if token == "" {
		return http.DefaultClient
	}
	return &http.Client{Transport: bearer{token, http.DefaultTransport}}
}

// bearer adds the token to every request; the token shows up nowhere else
// (provider.Redact keeps error messages clean too).
type bearer struct {
	token string
	rt    http.RoundTripper
}

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	req := r.Clone(r.Context())
	req.Header.Set("Authorization", "Bearer "+b.token)
	return b.rt.RoundTrip(req)
}
