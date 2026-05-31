package logging

import (
	"io"
	"log"
	"os"
	"strings"
	"time"
)

// ANSI colors for the console. Windows consoles need VT processing enabled
// first (see enableANSI); on terminals without color these are harmless when VT
// is on, and enableANSI is a no-op elsewhere.
const (
	colReset   = "\x1b[0m"
	colGray    = "\x1b[90m"
	colRed     = "\x1b[31m"
	colGreen   = "\x1b[32m"
	colYellow  = "\x1b[33m"
	colCyan    = "\x1b[36m"
	colMagenta = "\x1b[35m"
	colBlue    = "\x1b[34m"
)

// tagColors maps the bracketed level/component tags already present in log
// messages to a color, so the existing log lines become scannable without
// changing a single call site.
var tagColors = map[string]string{
	"[DEBUG]":      colGray,
	"[INFO]":       colGreen,
	"[WARN]":       colYellow,
	"[ERROR]":      colRed,
	"[FATAL]":      colRed,
	"[Server]":     colCyan,
	"[Java]":       colCyan,
	"[Bedrock]":    colMagenta,
	"[SkinBridge]": colBlue,
}

func colorize(s string) string {
	for tag, col := range tagColors {
		s = strings.ReplaceAll(s, tag, col+tag+colReset)
	}
	return s
}

// consoleWriter prepends a dim timestamp and colorizes known tags. It is
// installed via log.SetOutput with log's own flags cleared, so it fully owns the
// line format for both direct log.Printf calls and the Logger abstraction.
type consoleWriter struct{ out io.Writer }

func (w *consoleWriter) Write(p []byte) (int, error) {
	prefix := colGray + time.Now().Format("15:04:05") + colReset + " "
	if _, err := io.WriteString(w.out, prefix+colorize(string(p))); err != nil {
		return 0, err
	}
	return len(p), nil
}

// Setup routes the standard logger through the pretty console writer and enables
// ANSI colors on Windows. Call once at startup before logging.
func Setup() {
	enableANSI()
	SetOutput(os.Stderr)
}

// SetOutput re-routes the standard logger through the pretty console writer into
// w (timestamp prefix + colorized tags preserved). Used by the TUI to capture
// log lines into its panel; pass os.Stderr to restore normal console output.
func SetOutput(w io.Writer) {
	log.SetFlags(0)
	log.SetOutput(&consoleWriter{out: w})
}
