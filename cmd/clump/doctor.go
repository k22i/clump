package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"time"

	"github.com/k22i/clump/internal/cache"
	"github.com/k22i/clump/internal/config"
	"github.com/k22i/clump/internal/ipc"
	"github.com/k22i/clump/internal/rex"
)

func runDoctor(stdout io.Writer) error {
	p, _ := config.Path()
	cfg, err := config.Load(p)
	fmt.Fprintf(stdout, "config      %s", p)
	if _, statErr := os.Stat(p); statErr != nil {
		fmt.Fprint(stdout, " (not found: using defaults)")
	}
	fmt.Fprintln(stdout)
	if err != nil {
		fmt.Fprintf(stdout, "            error: %v\n", err)
	} else {
		fmt.Fprintf(stdout, "aliases     default=%s cheap=%s strong=%s\n", cfg.Default, cfg.Cheap, cfg.Strong)
		for _, pc := range cfg.Providers {
			note := ""
			switch {
			case pc.Type == "command" && len(pc.Argv) > 0:
				if path, err := exec.LookPath(pc.Argv[0]); err != nil {
					note = " (" + pc.Argv[0] + " not on PATH)"
				} else {
					note = " (runs " + path + ")"
				}
			case pc.APIKeyEnv != "":
				if os.Getenv(pc.APIKeyEnv) == "" {
					note = " ($" + pc.APIKeyEnv + " is not set)"
				}
			}
			fmt.Fprintf(stdout, "provider    %-16s %-9s %s%s\n", pc.Name, pc.Type, pc.ModelID(), note)
		}
		fmt.Fprintf(stdout, "limits      ceiling=%d budget=%d max_groups=%d auto_group=%v\n", cfg.Ceiling, cfg.Budget, cfg.MaxGroups, cfg.AutoGroup)
		g := cfg.Generated
		fmt.Fprintf(stdout, "generated   detect=%v builtin=%v patterns=%d review=%d\n", g.Detect, g.Builtin, len(g.Patterns), len(g.Review))
	}
	if d, err := cache.DefaultDir(); err == nil {
		fmt.Fprintf(stdout, "cache       %s\n", d)
	}
	if path, err := exec.LookPath("gh"); err == nil {
		fmt.Fprintf(stdout, "github      gh at %s (--pr fetches through it)\n", path)
	} else {
		fmt.Fprint(stdout, "github      gh is not on PATH (--pr needs the GitHub CLI)\n")
	}
	if wd, err := os.Getwd(); err == nil {
		if root := ipc.FindRepoRoot(wd); root != "" {
			sock := ipc.SocketPath(root)
			state := "no watch running"
			if c, err := net.DialTimeout("unix", sock, ipc.DialTimeout); err == nil {
				c.Close()
				state = "watch is listening"
			}
			fmt.Fprintf(stdout, "repo        %s\nsocket      %s (%s)\n", root, sock, state)
		}
	}
	fmt.Fprintf(stdout, "rex         %v (REX_SESSION=%q REX_BLOCK=%q)\n", rex.InRex(os.Getenv), os.Getenv("REX_SESSION"), os.Getenv("REX_BLOCK"))
	theme := "no terminal"
	if tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0); err == nil {
		start := time.Now()
		t := rex.DetectTheme(tty, os.Getenv, 200*time.Millisecond)
		tty.Close()
		if t == "" {
			t = "unknown (dark is used)"
		}
		theme = fmt.Sprintf("%s (%s)", t, time.Since(start).Round(time.Millisecond))
	}
	fmt.Fprintf(stdout, "theme       %s\n", theme)
	return nil
}
