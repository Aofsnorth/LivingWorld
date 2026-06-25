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
// This file covers the behaviors beyond grass (which has its own file) and
// gravity (which has gravity.go). It is called from the unified tick loop
// in Phase 3.

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

// randomTickLeaves implements vanilla leaf decay. Leaves that have no log
// within a 4-block Manhattan distance in any direction start decaying. Each
// random tick, a decaying leaf has a 1/20 chance of turning to air (and
// dropping a sapling with 5% chance). Leaves with a nearby log are "sustained"
// and never decay.
func (m *Manager) randomTickLeaves(rng *rand.Rand, w *World, x, y, z int, leafID int32) {
	// Check for a nearby log (4-block Manhattan distance).
	const checkRange = 4
	for dx := -checkRange; dx <= checkRange; dx++ {
		for dy := -checkRange; dy <= checkRange; dy++ {
			for dz := -checkRange; dz <= checkRange; dz++ {
				if abs(dx)+abs(dy)+abs(dz) > checkRange {
					continue
				}
				nid, ok := loadedBlockID(w, x+dx, y+dy, z+dz)
				if !ok {
					continue
				}
				nn := StateName(nid)
				if strings.HasSuffix(nn, "_log") || strings.HasSuffix(nn, "_wood") {
					return // sustained by a nearby log
				}
			}
		}
	}
	// No log nearby: 1/20 chance per tick to decay.
	if rng.Intn(20) != 0 {
		return
	}
	// Drop sapling (5% chance) and sticks (2% chance).
	if rng.Float64() < 0.05 {
		saplingName := leafToSapling(StateName(leafID))
		if saplingName != "" {
			m.drops.Spawn(saplingName, 1, float64(x)+0.5, float64(y)+0.25, float64(z)+0.5)
		}
	}
	if rng.Float64() < 0.02 {
		m.drops.Spawn("minecraft:stick", 1, float64(x)+0.5, float64(y)+0.25, float64(z)+0.5)
	}
	// Apples: oak and dark oak leaves drop apples (0.5% chance).
	ln := StateName(leafID)
	if strings.Contains(ln, "oak_leaves") || strings.Contains(ln, "dark_oak_leaves") {
		if rng.Float64() < 0.005 {
			m.drops.Spawn("minecraft:apple", 1, float64(x)+0.5, float64(y)+0.25, float64(z)+0.5)
		}
	}
	m.setWorldBlockAndPublish(w, x, y, z, BlockByID(AirID))
}

// leafToSapling maps a leaf block name to its sapling drop.
func leafToSappling(leafName string) string {
	switch {
	case strings.Contains(leafName, "oak_leaves"):
		return "minecraft:oak_sapling"
	case strings.Contains(leafName, "birch_leaves"):
		return "minecraft:birch_sapling"
	case strings.Contains(leafName, "spruce_leaves"):
		return "minecraft:spruce_sapling"
	case strings.Contains(leafName, "jungle_leaves"):
		return "minecraft:jungle_sapling"
	case strings.Contains(leafName, "acacia_leaves"):
		return "minecraft:acacia_sapling"
	case strings.Contains(leafName, "dark_oak_leaves"):
		return "minecraft:dark_oak_sapling"
	case strings.Contains(leafName, "cherry_leaves"):
		return "minecraft:cherry_sapling"
	case strings.Contains(leafName, "mangrove_leaves"):
		return "minecraft:mangrove_propagule"
	}
	return ""
}

// leafToSapling wraps leafToSapling (kept for compat).
func leafToSapling(leafName string) string {
	return leafToSappling(leafName)
}

// randomTickIce melts ice when exposed to high light (>= 11 sky light above).
// Vanilla: ice melts at light level >= 11 above the block.
func (m *Manager) randomTickIce(_ *rand.Rand, w *World, x, y, z int) {
	if localLightAbove(w, x, y, z) >= 11 {
		m.setWorldBlockAndPublish(w, x, y, z, BlockByName("minecraft:water"))
	}
}

// randomTickSnowLayer melts snow when light above >= 11.
func (m *Manager) randomTickSnowLayer(_ *rand.Rand, w *World, x, y, z int) {
	if localLightAbove(w, x, y, z) >= 11 {
		m.setWorldBlockAndPublish(w, x, y, z, BlockByID(AirID))
	}
}

// randomTickFarmland dehydrates farmland when no water is within 4 blocks.
// Vanilla: farmland with no water nearby turns to dirt. Hydrated farmland
// stays (the moisture state is a block state property; we model dry → dirt).
func (m *Manager) randomTickFarmland(_ *rand.Rand, w *World, x, y, z int) {
	// Check for water within 4 blocks horizontal, 1 block below.
	for dx := -4; dx <= 4; dx++ {
		for dz := -4; dz <= 4; dz++ {
			for dy := 0; dy <= 1; dy++ {
				nid, ok := loadedBlockID(w, x+dx, y-dy, z+dz)
				if !ok {
					continue
				}
				nn := StateName(nid)
				if nn == "minecraft:water" || nn == "minecraft:flowing_water" {
					return // hydrated
				}
			}
		}
	}
	// No water nearby: dry out to dirt.
	m.setWorldBlockAndPublish(w, x, y, z, BlockByName("minecraft:dirt"))
}

// randomTickFire spreads fire to adjacent flammable blocks and eventually
// extinguishes. Vanilla: fire has an age (0-15), each tick it ages +1,
// at age >= 15 it extinguishes. Fire also tries to spread to 4 random
// neighbors (±1 in each axis). Flammable blocks have a "catch chance"
// based on their flammability (planks=5, leaves=60, etc.).
func (m *Manager) randomTickFire(rng *rand.Rand, w *World, x, y, z int) {
	// Try to spread to 4 random neighbors.
	for i := 0; i < 4; i++ {
		nx := x + rng.Intn(3) - 1
		ny := y + rng.Intn(3) - 1
		nz := z + rng.Intn(3) - 1
		nid, ok := loadedBlockID(w, nx, ny, nz)
		if !ok || nid != AirID {
			continue
		}
		// Check if the block below the target is flammable.
		belowID, belowOk := loadedBlockID(w, nx, ny-1, nz)
		if !belowOk {
			continue
		}
		if isFlammable(StateName(belowID)) && rng.Float64() < 0.15 {
			m.setWorldBlockAndPublish(w, nx, ny, nz, BlockByName("minecraft:fire"))
		}
	}
	// 20% chance to extinguish (simplified aging).
	if rng.Float64() < 0.20 {
		m.setWorldBlockAndPublish(w, x, y, z, BlockByID(AirID))
	}
}

// isFlammable reports whether a block can catch fire.
func isFlammable(name string) bool {
	base := strings.TrimPrefix(name, "minecraft:")
	switch {
	case strings.HasSuffix(base, "_planks"), strings.HasSuffix(base, "_slab"):
		return true
	case strings.HasSuffix(base, "_log"), strings.HasSuffix(base, "_wood"):
		return true
	case strings.HasSuffix(base, "_leaves"):
		return true
	case base == "bookshelf" || base == "hay_block" || base == "bamboo":
		return true
	case base == "target" || base == "cave_vines":
		return false
	case strings.HasSuffix(base, "_wool"):
		return true
	case base == "dried_kelp_block" || base == "bamboo_block":
		return true
	}
	return false
}

// randomTickCrop advances crop growth by one stage. Vanilla crops have
// ages 0-7 (wheat/carrots/potatoes) or 0-3 (beetroots). Each random tick,
// a crop has a chance to grow based on light level and farmland below.
func (m *Manager) randomTickCrop(rng *rand.Rand, w *World, x, y, z int, cropID int32) {
	// Crops need light >= 9 above and farmland below.
	if localLightAbove(w, x, y, z) < 9 {
		return
	}
	belowID, ok := loadedBlockID(w, x, y-1, z)
	if !ok {
		return
	}
	belowName := StateName(belowID)
	if belowName != "minecraft:farmland" {
		return
	}
	// 1/3 chance to advance per random tick (vanilla approximation).
	if rng.Intn(3) != 0 {
		return
	}
	// We can't modify block state properties directly in the current
	// block model (block IDs are opaque). For now, the crop stays at
	// its current age. A full block-state system would increment the
	// age property. Log the intent for future implementation.
	// TODO: block state age properties.
}

// randomTickSugarCane grows sugar cane upward if conditions are met.
// Vanilla: max 3 blocks tall, needs water adjacent to the base.
func (m *Manager) randomTickSugarCane(rng *rand.Rand, w *World, x, y, z int) {
	// Count height of the sugar cane column.
	height := 0
	for dy := 0; dy < 3; dy++ {
		nid, ok := loadedBlockID(w, x, y-dy, z)
		if !ok || StateName(nid) != "minecraft:sugar_cane" {
			break
		}
		height++
	}
	if height >= 3 {
		return // max height
	}
	// Check if the block above is air.
	aboveID, ok := loadedBlockID(w, x, y+1, z)
	if !ok || aboveID != AirID {
		return
	}
	// 1/16 chance to grow per random tick.
	if rng.Intn(16) != 0 {
		return
	}
	m.setWorldBlockAndPublish(w, x, y+1, z, BlockByName("minecraft:sugar_cane"))
}

// randomTickCactus grows cactus upward (max 3 blocks).
func (m *Manager) randomTickCactus(rng *rand.Rand, w *World, x, y, z int) {
	height := 0
	for dy := 0; dy < 3; dy++ {
		nid, ok := loadedBlockID(w, x, y-dy, z)
		if !ok || StateName(nid) != "minecraft:cactus" {
			break
		}
		height++
	}
	if height >= 3 {
		return
	}
	aboveID, ok := loadedBlockID(w, x, y+1, z)
	if !ok || aboveID != AirID {
		return
	}
	if rng.Intn(16) != 0 {
		return
	}
	m.setWorldBlockAndPublish(w, x, y+1, z, BlockByName("minecraft:cactus"))
}

// randomTickMushroom spreads mushrooms to nearby dark spots (light < 13).
func (m *Manager) randomTickMushroom(rng *rand.Rand, w *World, x, y, z int, mushID int32) {
	if localLightAbove(w, x, y, z) >= 13 {
		return // too bright for mushrooms to spread
	}
	// 1/25 chance to spread per random tick.
	if rng.Intn(25) != 0 {
		return
	}
	// Try to place a mushroom nearby.
	nx := x + rng.Intn(5) - 2
	ny := y + rng.Intn(3) - 1
	nz := z + rng.Intn(5) - 2
	targetID, ok := loadedBlockID(w, nx, ny, nz)
	if !ok || targetID != AirID {
		return
	}
	belowID, belowOk := loadedBlockID(w, nx, ny-1, nz)
	if !belowOk {
		return
	}
	belowName := StateName(belowID)
	// Mushrooms grow on dirt, podzol, mycelium, or nylium.
	switch belowName {
	case "minecraft:dirt", "minecraft:podzol", "minecraft:mycelium",
		"minecraft:crimson_nylium", "minecraft:warped_nylium",
		"minecraft:coarse_dirt", "minecraft:grass_block":
		m.setWorldBlockAndPublish(w, nx, ny, nz, BlockByID(mushID))
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

// abs returns the absolute value of x.
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
