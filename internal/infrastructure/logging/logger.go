// Package logging menyediakan abstraksi untuk logging di LivingWorld.
// Ini memungkinkan kita untuk mengganti implementasi logging tanpa mengubah kode yang menggunakannya.
package logging

import "fmt"

// Logger adalah interface untuk logging operations.
type Logger interface {
	// Debug logs a debug message
	Debug(msg string, args ...any)

	// Info logs an informational message
	Info(msg string, args ...any)

	// Warn logs a warning message
	Warn(msg string, args ...any)

	// Error logs an error message
	Error(msg string, args ...any)

	// Fatal logs a fatal error and exits
	Fatal(msg string, args ...any)

	// WithField returns a new logger with an additional context field
	WithField(key string, value any) Logger

	// WithFields returns a new logger with multiple context fields
	WithFields(fields map[string]any) Logger
}

// Level represents log level
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

// String returns string representation of log level
func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	case LevelFatal:
		return "FATAL"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", l)
	}
}
