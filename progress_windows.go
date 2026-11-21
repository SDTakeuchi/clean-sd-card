//go:build windows

package main

import (
	"os"
	"syscall"
	"unsafe"
)

const enableVirtualTerminalProcessing = 0x0004

func enableTerminalOutput(out *os.File) bool {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getMode := kernel32.NewProc("GetConsoleMode")
	setMode := kernel32.NewProc("SetConsoleMode")
	var mode uint32
	handle := uintptr(out.Fd())
	if ok, _, _ := getMode.Call(handle, uintptr(unsafe.Pointer(&mode))); ok == 0 {
		return false
	}
	if mode&enableVirtualTerminalProcessing != 0 {
		return true
	}
	ok, _, _ := setMode.Call(handle, uintptr(mode|enableVirtualTerminalProcessing))
	return ok != 0
}
