package main

import (
	"errors"
	"flag"
	"io"
	"os"

	"github.com/k22i/clump/internal/config"
	"github.com/k22i/clump/internal/lazygit"
	"github.com/k22i/clump/internal/rex"
	rexlua "github.com/k22i/clump/rex"
)

func installFlags(name string, args []string) (write bool, then *string, err error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	w := fs.Bool("write", false, "")
	then = fs.String("then", "", "")
	if len(args) == 0 || args[0] != "install" {
		return false, nil, usageError{name + ": want install"}
	}
	if err := fs.Parse(args[1:]); err != nil {
		return false, nil, usageError{name + " install: " + err.Error()}
	}
	return *w, then, nil
}

func runLazygit(args []string, stdin *os.File, stdout io.Writer) error {
	write, then, err := installFlags("lazygit", args)
	if err != nil {
		return err
	}
	path, err := lazygit.ConfigPath()
	if err != nil {
		return err
	}
	t := *then
	if t == "" {
		if p, err := config.Path(); err == nil {
			if cfg, err := config.Load(p); err == nil {
				t = cfg.Then
			}
		}
	}
	return lazygit.Install(lazygit.InstallOpts{Path: path, Then: t, Write: write, In: stdin, Out: stdout})
}

func runRex(args []string, stdin *os.File, stdout io.Writer) error {
	write, _, err := installFlags("rex", args)
	if err != nil {
		return err
	}
	cfgPath, err := rex.ConfigPath()
	if err != nil {
		return err
	}
	modPath, err := rex.ModulePath()
	if err != nil {
		return err
	}
	if len(rexlua.Module) == 0 {
		return errors.New("rex: the Lua module is missing from this build")
	}
	return rex.Install(rex.InstallOpts{ConfigPath: cfgPath, ModulePath: modPath, Module: rexlua.Module, Write: write, In: stdin, Out: stdout})
}
