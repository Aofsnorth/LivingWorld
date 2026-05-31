package player

import (
	"encoding/json"
	"os"
	"path/filepath"

	"livingworld/internal/world"

	"github.com/google/uuid"
)

// PersistedPlayer is the on-disk (JSON) snapshot of a player's saved state.
type PersistedPlayer struct {
	X, Y, Z    float64
	Yaw, Pitch float32
	Health     float32
	Food       int
	Saturation float32
	Creative   bool
	Inventory  *Inventory
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
func (m *Manager) LoadPlayerData(id uuid.UUID) (*PersistedPlayer, bool) {
	path, ok := m.playerFile(id)
	if !ok {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var d PersistedPlayer
	if json.Unmarshal(data, &d) != nil {
		return nil, false
	}
	return &d, true
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
		Creative: p.Creative, Inventory: p.Inventory,
	}
	if b, err := json.Marshal(d); err == nil {
		_ = os.WriteFile(path, b, 0o644)
	}
}

// SaveAll persists every connected player. Called on server shutdown.
func (m *Manager) SaveAll() {
	for _, p := range m.GetAllPlayers() {
		m.SavePlayer(p)
	}
}

// ApplyPersisted copies saved fields onto a freshly created player (position,
// rotation, health, food, gamemode, inventory).
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
}
