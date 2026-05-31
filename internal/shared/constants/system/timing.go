// Package system menyediakan konstanta terkait system configuration.
package system

import "time"

// Timing constants untuk background tasks.
const (
	// DefaultAutosaveInterval adalah interval default untuk world autosave
	DefaultAutosaveInterval = 300 * time.Second // 5 menit
)
