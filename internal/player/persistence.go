package player

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"time"

	"livingworld/internal/world"

	"github.com/google/uuid"
)

// PersistedPlayer is the on-disk (JSON) snapshot of a player's saved state.
//
// Phase 3 hardening: the schema is forward-compatible. New fields may be
// added to this struct without breaking older save files (json.Unmarshal
// silently ignores unknown keys). If a future field rename or removal is
// required, write a migration in LoadPlayerData, do NOT break the on-disk
// format silently.
type PersistedPlayer struct {
	X, Y, Z    float64
	Yaw, Pitch float32
	Health     float32
	Food       int
	Saturation float32
	Creative   bool
	Inventory  *Inventory
	// Phase 3: explicit XP and gamemode fields (previously derived from
	// Creative bool alone). Older saves without these default to zero
	// (survival, no XP) which is the correct vanilla fallback.
	XPLevel    int     `json:"xp_level,omitempty"`
	XPProgress float32 `json:"xp_progress,omitempty"`
	Gamemode   int     `json:"gamemode,omitempty"` // 0 survival, 1 creative, ...
}

// EnablePersistence makes the manager save/load player data as JSON files under
// dir (one file per UUID). A no-op data dir disables persistence.
func (m *Manager) EnablePersistence(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	m.dataMu.Lock()
	m.dataDir = dir
	m.dataMu.Unlock()
	return nil
}

func (m *Manager) playerFile(id uuid.UUID) (string, bool) {
	m.dataMu.RLock()
	dir := m.dataDir
	m.dataMu.RUnlock()
	if dir == "" {
		return "", false
	}
	return filepath.Join(dir, id.String()+".json"), true
}

// LoadPlayerData reads a player's persisted state. Returns false if persistence
// is disabled or there is no save for this UUID (a first-time join).
//
// Phase 3 hardening: a malformed JSON file is moved to a sibling
// `quarantine/` subdir and the load returns false. The caller treats this
// the same as a first-time join (default state) instead of crashing.
func (m *Manager) LoadPlayerData(id uuid.UUID) (*PersistedPlayer, bool) {
	path, ok := m.playerFile(id)
	if !ok {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		// ENOENT = no save yet; any other error is also a clean "no data".
		return nil, false
	}
	var d PersistedPlayer
	if err := json.Unmarshal(data, &d); err != nil {
		quarantinePlayerData(path, err)
		return nil, false
	}
	return &d, true
}

// quarantinePlayerData moves a malformed player JSON file to a sibling
// `quarantine/` subdir. Best-effort: a failure to rename is logged and
// swallowed, since the on-disk file is corrupt anyway and the next save
// will overwrite it.
func quarantinePlayerData(path string, parseErr error) {
	dir := filepath.Dir(path)
	qdir := filepath.Join(dir, "quarantine")
	if err := os.MkdirAll(qdir, 0o755); err != nil {
		log.Printf("[Player] quarantine: mkdir %s: %v", qdir, err)
		return
	}
	stamp := time.Now().UTC().Format("20060102T150405")
	base := filepath.Base(path)
	dst := filepath.Join(qdir, stamp+"."+base+".bad")
	if err := os.Rename(path, dst); err != nil {
		log.Printf("[Player] quarantine: rename %s -> %s: %v (parse err was %v)", path, dst, err, parseErr)
		return
	}
	log.Printf("[Player] quarantined malformed player save %s: %v (moved to %s)", path, parseErr, dst)
}

// SavePlayer writes a player's current state to disk (no-op if persistence is
// disabled). Called on disconnect and shutdown.
func (m *Manager) SavePlayer(p *Player) {
	path, ok := m.playerFile(p.UUID)
	if !ok || p == nil {
		return
	}
	d := PersistedPlayer{
		X: p.Position.X, Y: p.Position.Y, Z: p.Position.Z,
		Yaw: p.Rotation.Yaw, Pitch: p.Rotation.Pitch,
		Health: p.Health, Food: p.Food, Saturation: p.Saturation,
		Creative:   p.Creative,
		Inventory:  p.Inventory,
		XPLevel:    p.XPLevel,
		XPProgress: p.XPProgress,
		Gamemode:   p.Gamemode,
	}
	if b, err := json.Marshal(d); err == nil {
		// Atomic write: temp + rename. If the process dies mid-write the
		// previous good save is preserved (Phase 3 crash-safety).
		tmp := path + ".tmp"
		if err := os.WriteFile(tmp, b, 0o644); err == nil {
			_ = os.Rename(tmp, path)
		}
	}
}

// SaveAll persists every connected player. Called on server shutdown.
func (m *Manager) SaveAll() {
	for _, p := range m.GetAllPlayers() {
		m.SavePlayer(p)
	}
}

// ApplyPersisted copies saved fields onto a freshly created player (position,
// rotation, health, food, gamemode, inventory, XP).
func (p *Player) ApplyPersisted(d *PersistedPlayer) {
	p.Position = world.Position{X: d.X, Y: d.Y, Z: d.Z}
	p.Rotation = world.Rotation{Yaw: d.Yaw, Pitch: d.Pitch}
	if d.Health > 0 {
		p.Health = d.Health
	}
	p.Food = d.Food
	p.Saturation = d.Saturation
	p.Creative = d.Creative
	if d.Inventory != nil {
		p.Inventory = d.Inventory
	}
	if d.XPLevel > 0 {
		p.XPLevel = d.XPLevel
	}
	if d.XPProgress > 0 {
		p.XPProgress = d.XPProgress
	}
	if d.Gamemode >= 0 && d.Gamemode <= 3 {
		p.Gamemode = d.Gamemode
	}
}
