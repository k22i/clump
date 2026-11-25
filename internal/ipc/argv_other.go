//go:build !darwin && !linux

package ipc

func parentArgv() []string { return nil }
