//go:build !windows
// +build !windows

package main

func ConsoleCP(*bool)                  {}
func IsCygwinTerminal(fd uintptr) bool { return false }
