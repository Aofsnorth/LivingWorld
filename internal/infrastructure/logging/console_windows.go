//go:build windows

package logging

import (
	"os"
	"syscall"
	"unsafe"
)

// enableANSI turns on ENABLE_VIRTUAL_TERMINAL_PROCESSING so the Windows console
// renders ANSI color escapes instead of printing them literally. Failures (e.g.
// output redirected to a file) are ignored.
func enableANSI() {
	const enableVTProcessing = 0x0004
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getConsoleMode := kernel32.NewProc("GetConsoleMode")
	setConsoleMode := kernel32.NewProc("SetConsoleMode")
	for _, h := range []uintptr{os.Stdout.Fd(), os.Stderr.Fd()} {
		var mode uint32
		if r, _, _ := getConsoleMode.Call(h, uintptr(unsafe.Pointer(&mode))); r == 0 {
			continue
		}
		_, _, _ = setConsoleMode.Call(h, uintptr(mode|enableVTProcessing))
	}
}
