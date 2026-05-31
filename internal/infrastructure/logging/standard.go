package logging

import (
	"fmt"
	"log"
	"os"
)

// StandardLogger adalah implementasi Logger menggunakan standard library log package.
type StandardLogger struct {
	prefix string
	fields map[string]any
	level  Level
}

// NewStandardLogger membuat StandardLogger baru.
func NewStandardLogger(prefix string) *StandardLogger {
	return &StandardLogger{
		prefix: prefix,
		fields: make(map[string]any),
		level:  LevelInfo,
	}
}

// SetLevel sets minimum log level
func (l *StandardLogger) SetLevel(level Level) {
	l.level = level
}

func (l *StandardLogger) shouldLog(level Level) bool {
	return level >= l.level
}

func (l *StandardLogger) formatMessage(level Level, msg string, args ...any) string {
	formatted := fmt.Sprintf(msg, args...)

	// Add prefix if exists
	if l.prefix != "" {
		formatted = fmt.Sprintf("[%s] %s", l.prefix, formatted)
	}

	// Add fields if exists
	if len(l.fields) > 0 {
		fieldStr := ""
		for k, v := range l.fields {
			fieldStr += fmt.Sprintf(" %s=%v", k, v)
		}
		formatted += fieldStr
	}

	return formatted
}

func (l *StandardLogger) Debug(msg string, args ...any) {
	if !l.shouldLog(LevelDebug) {
		return
	}
	log.Printf("[DEBUG] %s", l.formatMessage(LevelDebug, msg, args...))
}

func (l *StandardLogger) Info(msg string, args ...any) {
	if !l.shouldLog(LevelInfo) {
		return
	}
	log.Printf("[INFO] %s", l.formatMessage(LevelInfo, msg, args...))
}

func (l *StandardLogger) Warn(msg string, args ...any) {
	if !l.shouldLog(LevelWarn) {
		return
	}
	log.Printf("[WARN] %s", l.formatMessage(LevelWarn, msg, args...))
}

func (l *StandardLogger) Error(msg string, args ...any) {
	if !l.shouldLog(LevelError) {
		return
	}
	log.Printf("[ERROR] %s", l.formatMessage(LevelError, msg, args...))
}

func (l *StandardLogger) Fatal(msg string, args ...any) {
	log.Printf("[FATAL] %s", l.formatMessage(LevelFatal, msg, args...))
	os.Exit(1)
}

func (l *StandardLogger) WithField(key string, value any) Logger {
	newFields := make(map[string]any, len(l.fields)+1)
	for k, v := range l.fields {
		newFields[k] = v
	}
	newFields[key] = value

	return &StandardLogger{
		prefix: l.prefix,
		fields: newFields,
		level:  l.level,
	}
}

func (l *StandardLogger) WithFields(fields map[string]any) Logger {
	newFields := make(map[string]any, len(l.fields)+len(fields))
	for k, v := range l.fields {
		newFields[k] = v
	}
	for k, v := range fields {
		newFields[k] = v
	}

	return &StandardLogger{
		prefix: l.prefix,
		fields: newFields,
		level:  l.level,
	}
}
