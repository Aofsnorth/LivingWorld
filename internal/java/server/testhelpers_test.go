package server

import (
	"testing"

	"livingworld/config"
	"livingworld/internal/player"
	"livingworld/internal/world"

	"github.com/google/uuid"
)

// newTestJavaBridge constructs a minimal javaBridge with the
// pieces M5's tests need: a world manager with the default
// world, a player manager, and an empty session manager. It
// does NOT start any goroutines, register gmserver handlers,
// or open a network listener — it's a test fixture, not a
// runnable bridge.
func newTestJavaBridge(t *testing.T) *javaBridge {
	t.Helper()
	cfg := &config.Config{
		World: config.WorldConfig{Seed: 42},
	}
	wm := world.NewManager()
	// Force the default world to materialise so Mobs().Spawn
	// has a world context. The spawn director keys off the
	// default world's dimension / day time, so tests that
	// call Spawn directly don't need the day-time setup.
	_ = wm.GetDefaultWorld()
	pm := player.NewManager()
	return &javaBridge{
		cfg:      cfg,
		pm:       pm,
		wm:       wm,
		sessions: NewSessionManager(),
	}
}

// attackerUUID returns a deterministic UUID for tests. The
// value is the "M5 attacker" namespace (any 16 bytes is fine —
// the bridge just copies them into the mob's hurtBy field).
func attackerUUID() uuid.UUID {
	return uuid.MustParse("55555555-5555-5555-5555-555555555555")
}
