// Package player tests: Phase 3 player data hardening (quarantine + round-trip).
package player

import (
	"os"
	"path/filepath"
	"testing"

	"livingworld/internal/world"

	"github.com/google/uuid"
)

// TestPlayerDataRoundTrip: a freshly saved player JSON file must load back
// into a PersistedPlayer with the same values. This is the happy path; the
// malformed-JSON recovery test below covers the unhappy path.
func TestPlayerDataRoundTrip(t *testing.T) {
	dir := t.TempDir()
	m := NewManager()
	if err := m.EnablePersistence(dir); err != nil {
		t.Fatalf("EnablePersistence: %v", err)
	}

	uid := uuid.New()
	p := &Player{
		UUID:       uid,
		Username:   "tester",
		Position:   world.Position{X: 1.5, Y: 64, Z: -3.25},
		Rotation:   world.Rotation{Yaw: 90, Pitch: 0},
		Health:     18.5,
		Food:       12,
		Saturation: 4.0,
		Creative:   true,
		XPLevel:    7,
		XPProgress: 0.42,
		Gamemode:   1,
	}
	m.SavePlayer(p)

	got, ok := m.LoadPlayerData(uid)
	if !ok {
		t.Fatalf("LoadPlayerData: ok=false, want true")
	}
	if got.X != 1.5 || got.Y != 64 || got.Z != -3.25 {
		t.Errorf("position: got (%v,%v,%v), want (1.5,64,-3.25)", got.X, got.Y, got.Z)
	}
	if got.Health != 18.5 {
		t.Errorf("health: got %v, want 18.5", got.Health)
	}
	if got.XPLevel != 7 || got.XPProgress != 0.42 {
		t.Errorf("xp: got (level=%v, prog=%v), want (7, 0.42)", got.XPLevel, got.XPProgress)
	}
	if got.Gamemode != 1 {
		t.Errorf("gamemode: got %v, want 1", got.Gamemode)
	}
}

// TestPlayerDataQuarantine: a malformed JSON file in the player data dir
// must be moved to `quarantine/` on the next LoadPlayerData call. The
// load itself returns ok=false (treated as first-time join), not an
// error or a panic.
func TestPlayerDataQuarantine(t *testing.T) {
	dir := t.TempDir()
	m := NewManager()
	if err := m.EnablePersistence(dir); err != nil {
		t.Fatalf("EnablePersistence: %v", err)
	}

	// Write a malformed JSON file directly.
	uid := uuid.New()
	badPath := filepath.Join(dir, uid.String()+".json")
	if err := os.WriteFile(badPath, []byte("{not valid json at all"), 0o644); err != nil {
		t.Fatalf("write bad json: %v", err)
	}

	// LoadPlayerData must NOT crash and must NOT return an error. It
	// returns ok=false (no data) and quarantines the file.
	got, ok := m.LoadPlayerData(uid)
	if ok {
		t.Errorf("LoadPlayerData on malformed JSON: ok=true, want false (no usable data)")
	}
	if got != nil {
		t.Errorf("LoadPlayerData on malformed JSON: got non-nil, want nil")
	}

	// Original file must be moved to quarantine/.
	if _, err := os.Stat(badPath); !os.IsNotExist(err) {
		t.Errorf("original bad file still present: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(dir, "quarantine"))
	if err != nil {
		t.Fatalf("quarantine dir missing: %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("quarantine dir is empty")
	}
}

// TestPlayerDataMissingIsDefault: a first-time join (no file at all) is
// a clean no-data case, not an error.
func TestPlayerDataMissingIsDefault(t *testing.T) {
	dir := t.TempDir()
	m := NewManager()
	if err := m.EnablePersistence(dir); err != nil {
		t.Fatalf("EnablePersistence: %v", err)
	}
	_, ok := m.LoadPlayerData(uuid.New())
	if ok {
		t.Errorf("LoadPlayerData on missing file: ok=true, want false")
	}
}
