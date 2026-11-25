package main

import (
	"errors"
	"io"
	"os"
)

// Entry points built in later phases.

func runRex(args []string, stdin *os.File, stdout io.Writer) error {
	return errors.New("rex: not built yet")
}

func runLazygit(args []string, stdin *os.File, stdout io.Writer) error {
	return errors.New("lazygit: not built yet")
}
