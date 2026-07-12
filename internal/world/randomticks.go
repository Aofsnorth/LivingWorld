package world

import (
	"math/rand"
	"strings"
)

// Random tick system: drives crop growth, leaf decay, ice/snow melting,
// farmland hydration, mushroom spread, fire spread, grass/mycelium spread,
// and other block-age behaviors. Each is sampled at randomTickSpeed per
// section per tick (vanilla default 3).
//
// This file covers the top-level random-tick dispatch and budget; per-block
// handlers live in randomticks_block.go.

// randomTick dispatches the random-tick behaviors for a single sampled
// block. It is called once per sample from randomTickWorld.
func (m *Manager) randomTick(rng *rand.Rand, w *World, x, y, z int) {
	id, ok := loadedBlockID(w, x, y, z)
	if !ok || id == AirID {
		return
	}
	name := StateName(id)
	base := strings.TrimPrefix(name, "minecraft:")

	switch {
	case strings.HasSuffix(base, "_leaves"):
		m.randomTickLeaves(rng, w, x, y, z, id)
	case base == "ice":
		m.randomTickIce(rng, w, x, y, z)
	case base == "snow":
		m.randomTickSnowLayer(rng, w, x, y, z)
	case base == "snow_block":
		// Snow blocks only melt in direct sunlight with high light
		// and warm biomes. Simplified: melt if sky light above >= 12.
		if localLightAbove(w, x, y, z) >= 12 {
			m.setWorldBlockAndPublish(w, x, y, z, BlockByID(AirID))
		}
	case base == "farmland":
		m.randomTickFarmland(rng, w, x, y, z)
	case base == "fire" || base == "soul_fire":
		m.randomTickFire(rng, w, x, y, z)
	case base == "wheat" || base == "carrots" || base == "potatoes" || base == "beetroots":
		m.randomTickCrop(rng, w, x, y, z, id)
	case base == "sugar_cane":
		m.randomTickSugarCane(rng, w, x, y, z)
	case base == "cactus":
		m.randomTickCactus(rng, w, x, y, z)
	case base == "brown_mushroom" || base == "red_mushroom":
		m.randomTickMushroom(rng, w, x, y, z, id)
	}
}

// randomTickWorld is the master random-tick dispatcher. It samples
// randomTickSpeed positions per section and dispatches to randomTick.
// Called from the unified tick loop.
func (m *Manager) randomTickWorld(rng *rand.Rand, w *World) {
	if rng == nil || w == nil {
		return
	}
	chunkPositions := w.loadedChunkPositions()
	if len(chunkPositions) == 0 {
		return
	}
	for _, cp := range chunkPositions {
		for section := 0; section < SectionsPerChunk; section++ {
			for i := 0; i < defaultRandomTickSpeed; i++ {
				bx := cp.WorldX + rng.Intn(ChunkSize)
				by := MinWorldHeight + section*16 + rng.Intn(16)
				bz := cp.WorldZ + rng.Intn(ChunkSize)
				m.randomTick(rng, w, bx, by, bz)
			}
		}
	}
}
