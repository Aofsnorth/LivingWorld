//go:build !windows

package logging

// enableANSI is a no-op on non-Windows platforms, where terminals already
// interpret ANSI escapes.
func enableANSI() {}
