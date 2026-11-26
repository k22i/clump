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

func runUpdate(args []string, stdout, stderr io.Writer) error {
	if len(args) > 0 {
		return usageError{"update: want no arguments, got " + strings.Join(args, " ")}
	}
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

type bearer struct {
	token string
	rt    http.RoundTripper
}

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	req := r.Clone(r.Context())
	req.Header.Set("Authorization", "Bearer "+b.token)
	return b.rt.RoundTrip(req)
}
