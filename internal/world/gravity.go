package world

import (
	"math/rand"
	"strings"
)

// Gravity blocks: sand, gravel, red sand, anvils, scaffolding, dragon egg,
// pointed dripstone, powder snow. Vanilla: a gravity block that has air below
// it turns into a falling entity; when the entity lands on a solid block it
// places itself. LivingWorld models this as an instant-drop: scan downward
// until a solid block is found, move the block there in one tick, and spawn
// a brief "falling block" drop-entity for cross-edition rendering (future).
//
// The gravity tick runs once per unified 20 Hz tick over every loaded chunk,
// sampling randomTickSpeed positions per section. Vanilla uses block updates
// (a gravity block checks on every neighbor-change), but random-ticking the
// gravity blocks is a cheap approximation that catches unsupported sand/gravel
// within a few ticks.

// isGravityBlock reports whether the block name is subject to gravity.
func isGravityBlock(name string) bool {
	base := strings.TrimPrefix(name, "minecraft:")
	switch base {
	case "sand", "red_sand", "gravel",
		"anvil", "chipped_anvil", "damaged_anvil",
		"scaffolding", "pointed_dripstone", "dragon_egg":
		return true
	}
	// Concrete powder variants (white_concrete_powder, etc.)
	if strings.HasSuffix(base, "_concrete_powder") {
		return true
	}
	return false
}

// gravityTick scans loaded chunks for gravity blocks with air below and
// drops them to the nearest solid surface. Runs from the unified tick loop
// (Phase 3, alongside grass random ticks).
func (m *Manager) gravityTick(rng *rand.Rand) {
	if rng == nil {
		return
	}
	m.mu.RLock()
	worlds := make([]*World, 0, len(m.worlds))
	for _, w := range m.worlds {
		worlds = append(worlds, w)
	}
	m.mu.RUnlock()

	for _, w := range worlds {
		m.gravityTickWorld(rng, w)
	}
}

// gravityTickWorld samples positions in every loaded chunk section and
// drops gravity blocks that have no solid support below.
func (m *Manager) gravityTickWorld(rng *rand.Rand, w *World) {
	if w == nil {
		return
	}
	chunkPositions := w.loadedChunkPositions()
	if len(chunkPositions) == 0 {
		return
	}
	// Sample 2 positions per section per tick — gravity is uncommon so a
	// high sample rate isn't needed. Each unsupported block drops
	// immediately, so one scan catches cascading falls.
	for _, cp := range chunkPositions {
		for section := 0; section < SectionsPerChunk; section++ {
			for i := 0; i < 2; i++ {
				bx := cp.WorldX + rng.Intn(ChunkSize)
				by := MinWorldHeight + section*16 + rng.Intn(16)
				bz := cp.WorldZ + rng.Intn(ChunkSize)

				id, ok := loadedBlockID(w, bx, by, bz)
				if !ok || id == AirID {
					continue
				}
				name := StateName(id)
				if !isGravityBlock(name) {
					continue
				}
				// Check if the block below is air (unsupported).
				belowID, belowOk := loadedBlockID(w, bx, by-1, bz)
				if !belowOk {
					continue
				}
				if belowID != AirID {
					continue
				}
				// Find the landing Y: scan downward until a solid block.
				landY := by - 1
				for landY > MinWorldHeight {
					checkID, checkOk := loadedBlockID(w, bx, landY-1, bz)
					if !checkOk || checkID != AirID {
						break
					}
					landY--
				}
				if landY <= MinWorldHeight {
					// Fell into the void: remove the block.
					m.setWorldBlockAndPublish(w, bx, by, bz, BlockByID(AirID))
					continue
				}
				if landY == by-1 {
					// Only one block down: just drop it directly.
					block := BlockByID(id)
					m.setWorldBlockAndPublish(w, bx, by, bz, BlockByID(AirID))
					m.setWorldBlockAndPublish(w, bx, landY, bz, block)
				} else {
					// Multi-block fall: clear source, place at landing.
					block := BlockByID(id)
					m.setWorldBlockAndPublish(w, bx, by, bz, BlockByID(AirID))
					m.setWorldBlockAndPublish(w, bx, landY, bz, block)
				}
			}
		}
	}
}
