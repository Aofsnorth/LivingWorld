// Package persistence provides flat-file world and player storage for
// LivingWorld. It is edition-agnostic and stdlib-only (no external deps):
// chunk block data is serialized to compact gzipped blobs (chunk.go) and
// level/player data to JSON. DESIGN decision (a): flat-file is the default,
// pluggable backend; a LevelDB backend can be added later behind the same
// Store surface. All writes are atomic (temp file + rename) so a crash mid-save
// never corrupts existing data.
package persistence

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Store is a flat-file persistence backend rooted at a single world directory:
//
//	<dir>/level.json               world metadata (seed, spawn, time)
//	<dir>/region/c.<cx>.<cz>.gz    per-chunk gzipped block data
//	<dir>/players/<uuid>.json      player state (position, health, inventory)
type Store struct {
	dir string
}

// Open creates the world directory layout if needed and returns a Store.
func Open(dir string) (*Store, error) {
	for _, sub := range []string{"region", "players"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			return nil, fmt.Errorf("persistence: create %s: %w", sub, err)
		}
	}
	return &Store{dir: dir}, nil
}

// Close releases the store. Writes are flushed per call, so this is a no-op for
// the flat-file backend; it exists so callers can defer Close uniformly.
func (s *Store) Close() error { return nil }

// LevelData is world-scoped metadata.
type LevelData struct {
	Name      string `json:"name"`
	Seed      int64  `json:"seed"`
	SpawnX    int    `json:"spawn_x"`
	SpawnY    int    `json:"spawn_y"`
	SpawnZ    int    `json:"spawn_z"`
	Time      int64  `json:"time"`
	Generator string `json:"generator,omitempty"`
}

func (s *Store) levelPath() string { return filepath.Join(s.dir, "level.json") }

// SaveLevel writes world metadata.
func (s *Store) SaveLevel(l *LevelData) error { return writeJSON(s.levelPath(), l) }

// LoadLevel reads world metadata; ok=false if it has never been saved.
func (s *Store) LoadLevel() (*LevelData, bool, error) {
	var l LevelData
	ok, err := readJSON(s.levelPath(), &l)
	if !ok || err != nil {
		return nil, ok, err
	}
	return &l, true, nil
}

// ItemStack is one persisted inventory slot (edition-agnostic, namespaced id).
type ItemStack struct {
	ID    string `json:"id"`
	Count int    `json:"count"`
	Meta  int    `json:"meta,omitempty"`
	NBT   []byte `json:"nbt,omitempty"`
}

// PlayerData is the persisted per-player state (R: inventory, position, health).
type PlayerData struct {
	UUID      string      `json:"uuid"`
	Name      string      `json:"name"`
	X         float64     `json:"x"`
	Y         float64     `json:"y"`
	Z         float64     `json:"z"`
	Yaw       float32     `json:"yaw"`
	Pitch     float32     `json:"pitch"`
	Health    float32     `json:"health"`
	Food      int         `json:"food"`
	GameMode  int         `json:"gamemode"`
	HeldSlot  int         `json:"held_slot"`
	Inventory []ItemStack `json:"inventory,omitempty"`
}

func (s *Store) playerPath(uuid string) string {
	return filepath.Join(s.dir, "players", uuid+".json")
}

// SavePlayer writes a player's state, keyed by UUID.
func (s *Store) SavePlayer(p *PlayerData) error {
	if p.UUID == "" {
		return fmt.Errorf("persistence: player UUID is empty")
	}
	return writeJSON(s.playerPath(p.UUID), p)
}

// LoadPlayer reads a player's state; ok=false if never saved (first join).
func (s *Store) LoadPlayer(uuid string) (*PlayerData, bool, error) {
	var p PlayerData
	ok, err := readJSON(s.playerPath(uuid), &p)
	if !ok || err != nil {
		return nil, ok, err
	}
	return &p, true, nil
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(path, data)
}

// readJSON returns ok=false (nil error) when the file does not exist.
func readJSON(path string, v any) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, json.Unmarshal(data, v)
}

// writeAtomic writes data via a temp file + rename so readers never observe a
// partially written file. os.Rename replaces an existing target on Windows too.
func writeAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
