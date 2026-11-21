//go:build !windows

package main

import "os"

func enableTerminalOutput(_ *os.File) bool { return true }
