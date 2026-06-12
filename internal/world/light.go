package world

import (
	"strings"
	"sync"

	"github.com/Tnze/go-mc/level/block"
)

// LightEngine manages sky and block light propagation across the world.
// Light is computed per-chunk using BFS from light sources (sky access for
// sky light, light-emitting blocks for block light). When blocks change,
// affected chunks are queued for recomputation and processed during the tick.
type LightEngine struct {
	mu      sync.Mutex
	world   *World
	pending map[ChunkPos]struct{} // chunks needing light recomputation
}

// NewLightEngine creates a light engine for the given world.
func NewLightEngine(w *World) *LightEngine {
	return &LightEngine{
		world:   w,
		pending: make(map[ChunkPos]struct{}),
	}
}

// QueueUpdate marks a chunk for light recomputation on the next tick.
func (le *LightEngine) QueueUpdate(cx, cz int) {
	le.mu.Lock()
	le.pending[ChunkPos{cx, cz}] = struct{}{}
	le.mu.Unlock()
}

// ProcessUpdates recomputes light for all queued chunks and clears the queue.
// Called from the tick loop.
func (le *LightEngine) ProcessUpdates() {
	le.mu.Lock()
	pending := le.pending
	le.pending = make(map[ChunkPos]struct{})
	le.mu.Unlock()

	for pos := range pending {
		// Keep the world read lock for the whole recompute. ComputeChunkLight's
		// world-coordinate helpers intentionally use getChunkUnlocked because chunk
		// generation calls this while already holding world.mu; without a lock here,
		// tick-time light recomputation can race a concurrent chunk load/unload and
		// panic on a concurrent map read/write.
		le.world.mu.RLock()
		chunk := le.world.chunks[pos]
		if chunk != nil {
			le.ComputeChunkLight(chunk, pos.X, pos.Z)
		}
		le.world.mu.RUnlock()
	}
}

// lightPos is a world-coordinate position used during BFS propagation.
type lightPos struct {
	x, y, z int
}

// BlockLightProps holds the light-related properties of a block type.
type BlockLightProps struct {
	Emission uint8 // 0-15, light emitted by this block
	Opacity  uint8 // 0-15, how much light this block absorbs
}

// blockLightProps is a per-state-ID lookup table built at init time.
// Index by canonical state ID (Java global state ID).
var blockLightProps []BlockLightProps

func init() {
	buildLightPropsTable()
}

// buildLightPropsTable populates the blockLightProps array by scanning all
// known block states and classifying them by name.
func buildLightPropsTable() {
	count := len(block.StateList)
	blockLightProps = make([]BlockLightProps, count)

	for i, state := range block.StateList {
		name := state.ID()
		blockLightProps[i] = classifyBlock(name)
	}
}

// classifyBlock returns the light properties for a block based on its namespaced name.
// Uses string matching on the block name to determine emission and opacity.
func classifyBlock(name string) BlockLightProps {
	// Strip minecraft: prefix for cleaner matching
	base := strings.TrimPrefix(name, "minecraft:")

	// Default: opaque, non-emitting
	props := BlockLightProps{Emission: 0, Opacity: 15}

	// Check for light-emitting blocks
	switch {
	case base == "torch" || base == "wall_torch":
		props.Emission = 14
	case base == "glowstone":
		props.Emission = 15
	case base == "sea_lantern":
		props.Emission = 15
	case base == "lantern":
		props.Emission = 15
	case base == "soul_lantern":
		props.Emission = 10
	case base == "soul_torch" || base == "soul_wall_torch":
		props.Emission = 10
	case base == "jack_o_lantern":
		props.Emission = 15
	case base == "beacon":
		props.Emission = 15
	case base == "end_rod":
		props.Emission = 14
	case base == "redstone_lamp" && strings.Contains(name, "lit=true"):
		props.Emission = 15
	case base == "shroomlight":
		props.Emission = 15
	case base == "amethyst_cluster":
		props.Emission = 5
	case base == "large_amethyst_bud":
		props.Emission = 4
	case base == "medium_amethyst_bud":
		props.Emission = 2
	case base == "small_amethyst_bud":
		props.Emission = 1
	case base == "crying_obsidian":
		props.Emission = 10
	case base == "respawn_anchor" && strings.Contains(name, "charges=4"):
		props.Emission = 15
	case base == "lava" || base == "flowing_lava":
		props.Emission = 15
	case base == "fire":
		props.Emission = 15
	case base == "soul_fire":
		props.Emission = 10
	case base == "magma_block":
		props.Emission = 3
	case base == "brewing_stand":
		props.Emission = 1
	case base == "brown_mushroom":
		props.Emission = 1
	case base == "ender_chest":
		props.Emission = 7
	case base == "end_portal_frame" && strings.Contains(name, "eye=true"):
		props.Emission = 1
	case base == "dragon_egg":
		props.Emission = 1
	case strings.HasPrefix(base, "redstone_ore") && strings.Contains(name, "lit=true"):
		props.Emission = 9
	case strings.HasPrefix(base, "deepslate_redstone_ore") && strings.Contains(name, "lit=true"):
		props.Emission = 9
	case base == "furnace" && strings.Contains(name, "lit=true"):
		props.Emission = 13
	case base == "blast_furnace" && strings.Contains(name, "lit=true"):
		props.Emission = 13
	case base == "smoker" && strings.Contains(name, "lit=true"):
		props.Emission = 13
	case base == "campfire" && strings.Contains(name, "lit=true"):
		props.Emission = 15
	case base == "soul_campfire" && strings.Contains(name, "lit=true"):
		props.Emission = 10
	}

	// Check for transparent/semi-transparent blocks (override opacity)
	switch {
	case base == "air" || base == "cave_air" || base == "void_air":
		props.Opacity = 0
	case base == "glass" || base == "glass_pane":
		props.Opacity = 1
	case strings.HasSuffix(base, "_stained_glass") || strings.HasSuffix(base, "_stained_glass_pane"):
		props.Opacity = 1
	case base == "water" || base == "flowing_water":
		props.Opacity = 2
	case base == "ice":
		props.Opacity = 1
	case strings.HasSuffix(base, "_leaves"):
		props.Opacity = 1
	case base == "cobweb":
		props.Opacity = 1
	case base == "slime_block" || base == "honey_block":
		props.Opacity = 1
	case base == "nether_portal" || base == "end_portal" || base == "end_gateway":
		props.Opacity = 0
	case base == "fire" || base == "soul_fire":
		props.Opacity = 0
	case base == "lava" || base == "flowing_lava":
		props.Opacity = 0
	case strings.HasSuffix(base, "_carpet"):
		props.Opacity = 0
	case strings.HasSuffix(base, "_slab"):
		// Slabs are half-blocks; treat as semi-transparent
		props.Opacity = 2
	case strings.HasSuffix(base, "_stairs"):
		props.Opacity = 2
	case strings.HasPrefix(base, "oak_") || strings.HasPrefix(base, "spruce_") ||
		strings.HasPrefix(base, "birch_") || strings.HasPrefix(base, "jungle_") ||
		strings.HasPrefix(base, "acacia_") || strings.HasPrefix(base, "dark_oak_") ||
		strings.HasPrefix(base, "crimson_") || strings.HasPrefix(base, "warped_"):
		// Wooden blocks are opaque
		props.Opacity = 15
	case base == "grass_block" || base == "dirt" || base == "stone" || base == "cobblestone":
		props.Opacity = 15
	case base == "sand" || base == "gravel" || base == "clay":
		props.Opacity = 15
	case strings.HasSuffix(base, "_ore") || strings.HasSuffix(base, "_deepslate"):
		props.Opacity = 15
	case base == "bedrock":
		props.Opacity = 15
	}

	// Non-solid blocks (flowers, grass, torches, etc.) are transparent
	if isNonSolidBlock(base) {
		props.Opacity = 0
	}

	return props
}

// isNonSolidBlock returns true for blocks that don't fill their cube (flowers, grass, etc.).
func isNonSolidBlock(base string) bool {
	nonSolid := []string{
		"grass", "fern", "dead_bush", "seagrass", "tall_grass", "large_fern",
		"dandelion", "poppy", "blue_orchid", "allium", "azure_bluet",
		"red_tulip", "orange_tulip", "white_tulip", "pink_tulip", "oxeye_daisy",
		"cornflower", "lily_of_the_valley", "wither_rose", "sunflower", "lilac",
		"rose_bush", "peony", "lily_pad", "sweet_berry_bush",
		"wheat", "carrots", "potatoes", "beetroots", "melon_stem", "pumpkin_stem",
		"nether_wart", "chorus_flower", "chorus_plant",
		"red_mushroom", "brown_mushroom",
		"sugar_cane", "kelp", "bamboo", "vine", "weeping_vines", "twisting_vines",
		"crimson_roots", "warped_roots", "nether_sprouts",
		"torch", "wall_torch", "soul_torch", "soul_wall_torch", "redstone_torch", "redstone_wall_torch",
		"lever", "rail", "powered_rail", "detector_rail", "activator_rail",
		"stone_button", "oak_button", "spruce_button", "birch_button", "jungle_button",
		"acacia_button", "dark_oak_button", "crimson_button", "warped_button", "polished_blackstone_button",
		"stone_pressure_plate", "oak_pressure_plate", "spruce_pressure_plate", "birch_pressure_plate",
		"jungle_pressure_plate", "acacia_pressure_plate", "dark_oak_pressure_plate",
		"crimson_pressure_plate", "warped_pressure_plate", "polished_blackstone_pressure_plate",
		"light_weighted_pressure_plate", "heavy_weighted_pressure_plate",
		"tripwire_hook", "tripwire", "snow", "moss_carpet",
		"redstone_wire", "repeater", "comparator",
		"sign", "wall_sign", "oak_sign", "oak_wall_sign",
		"spruce_sign", "spruce_wall_sign", "birch_sign", "birch_wall_sign",
		"jungle_sign", "jungle_wall_sign", "acacia_sign", "acacia_wall_sign",
		"dark_oak_sign", "dark_oak_wall_sign", "crimson_sign", "crimson_wall_sign",
		"warped_sign", "warped_wall_sign",
		"item_frame", "glow_item_frame", "painting",
		"flower_pot", "potted_", "candle", "candles",
		"end_rod", "lightning_rod", "chain",
		"bell", "lantern", "soul_lantern", "campfire", "soul_campfire",
		"cake", "candle_cake",
		"brewing_stand", "cauldron", "water_cauldron", "lava_cauldron", "powder_snow_cauldron",
		"composter", "lectern", "grindstone", "stonecutter", "loom", "cartography_table",
		"fletching_table", "smithing_table", "smoker", "blast_furnace", "furnace",
		"enchanting_table", "anvil", "chipped_anvil", "damaged_anvil",
		"beacon", "conduit", "lodestone", "respawn_anchor",
		"banner", "wall_banner",
	}

	for _, ns := range nonSolid {
		if base == ns || strings.HasPrefix(base, ns) {
			return true
		}
	}

	// Check for potted plants
	if strings.HasPrefix(base, "potted_") {
		return true
	}

	return false
}

// GetLightProps returns the light properties for the given canonical state ID.
func GetLightProps(stateID int32) BlockLightProps {
	if stateID < 0 || int(stateID) >= len(blockLightProps) {
		return BlockLightProps{Emission: 0, Opacity: 15}
	}
	return blockLightProps[stateID]
}

// ComputeChunkLight computes sky and block light for a single chunk.
// This is called when a chunk is first loaded or when blocks change.
func (le *LightEngine) ComputeChunkLight(chunk *Chunk, cx, cz int) {
	// Build heightmap
	le.computeHeightmap(chunk, cx, cz)

	// Compute sky light
	le.computeSkyLight(chunk, cx, cz)

	// Compute block light
	le.computeBlockLight(chunk, cx, cz)
}

// computeHeightmap scans the chunk and records the highest non-air block per column.
func (le *LightEngine) computeHeightmap(chunk *Chunk, cx, cz int) {
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			highest := MinWorldHeight - 1
			for y := MinWorldHeight + SectionsPerChunk*16 - 1; y >= MinWorldHeight; y-- {
				blockID := chunk.GetBlock(x, y, z).ID()
				if blockID != AirID {
					highest = y
					break
				}
			}
			chunk.SetHeightmap(x, z, int32(highest))
		}
	}
}

// computeSkyLight initializes sky light for a chunk using column scan + BFS.
func (le *LightEngine) computeSkyLight(chunk *Chunk, cx, cz int) {
	// Phase 1: Scan each column from top down, set sky light for blocks with direct sky access
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			heightY := int(chunk.GetHeightmap(x, z))

			// Set sky light = 15 for all air blocks above the highest solid block
			for y := MinWorldHeight + SectionsPerChunk*16 - 1; y > heightY; y-- {
				chunk.SetSkyLight(x, y, z, 15)
			}

			// Below the highest block, scan down and set light based on opacity
			currentLight := uint8(15)
			for y := heightY; y >= MinWorldHeight; y-- {
				blockID := chunk.GetBlock(x, y, z).ID()
				opacity := GetLightProps(blockID).Opacity

				// Reduce light by opacity
				if currentLight > opacity {
					currentLight -= opacity
				} else {
					currentLight = 0
				}

				chunk.SetSkyLight(x, y, z, currentLight)

				// Apply distance falloff (1 light per block)
				if currentLight > 1 {
					currentLight -= 1
				} else {
					currentLight = 0
				}

				// If light is 0, everything below is also 0
				if currentLight == 0 {
					for y2 := y - 1; y2 >= MinWorldHeight; y2-- {
						chunk.SetSkyLight(x, y2, z, 0)
					}
					break
				}
			}
		}
	}

	// Phase 2: BFS from light/dark boundaries to spread light horizontally and into caves
	var queue []lightPos
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			for y := MinWorldHeight; y < MinWorldHeight+SectionsPerChunk*16; y++ {
				currentLight := chunk.GetSkyLight(x, y, z)
				if currentLight == 0 {
					continue
				}

				// Check if any neighbor has lower light (potential spread boundary)
				worldX := cx*16 + x
				worldZ := cz*16 + z
				for _, offset := range [][3]int{{1, 0, 0}, {-1, 0, 0}, {0, 0, 1}, {0, 0, -1}, {0, 1, 0}, {0, -1, 0}} {
					nx, ny, nz := worldX+offset[0], y+offset[1], worldZ+offset[2]
					if ny < MinWorldHeight || ny >= MinWorldHeight+SectionsPerChunk*16 {
						continue
					}

					neighborLight := le.getSkyLightWorld(nx, ny, nz)
					// If neighbor has lower light and is transparent, queue for propagation
					if neighborLight < currentLight {
						neighborBlockID := le.getBlockIDWorld(nx, ny, nz)
						opacity := GetLightProps(neighborBlockID).Opacity
						if opacity < 15 { // transparent
							queue = append(queue, lightPos{worldX, y, worldZ})
							break // Only queue once per position
						}
					}
				}
			}
		}
	}

	// BFS: spread sky light to neighbors
	visited := make(map[lightPos]bool)
	for len(queue) > 0 {
		pos := queue[0]
		queue = queue[1:]

		if visited[pos] {
			continue
		}
		visited[pos] = true

		currentLight := le.getSkyLightWorld(pos.x, pos.y, pos.z)
		if currentLight == 0 {
			continue
		}

		// Try to spread to each neighbor
		for _, offset := range [][3]int{{1, 0, 0}, {-1, 0, 0}, {0, 0, 1}, {0, 0, -1}, {0, 1, 0}, {0, -1, 0}} {
			nx, ny, nz := pos.x+offset[0], pos.y+offset[1], pos.z+offset[2]

			if ny < MinWorldHeight || ny >= MinWorldHeight+SectionsPerChunk*16 {
				continue
			}

			neighborBlockID := le.getBlockIDWorld(nx, ny, nz)
			opacity := GetLightProps(neighborBlockID).Opacity

			// Calculate new light level at neighbor (falloff by 1 + opacity, same as block light)
			newLight := uint8(0)
			if currentLight > opacity+1 {
				newLight = currentLight - opacity - 1
			}

			// Get current neighbor light
			neighborLight := le.getSkyLightWorld(nx, ny, nz)

			// If new light is brighter, update and queue
			if newLight > neighborLight {
				le.setSkyLightWorld(nx, ny, nz, newLight)
				queue = append(queue, lightPos{nx, ny, nz})
			}
		}
	}
}

// computeBlockLight initializes block light for a chunk using BFS from light sources.
func (le *LightEngine) computeBlockLight(chunk *Chunk, cx, cz int) {
	// Phase 1: Find all light-emitting blocks and set their emission
	var queue []lightPos
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			for y := MinWorldHeight; y < MinWorldHeight+SectionsPerChunk*16; y++ {
				blockID := chunk.GetBlock(x, y, z).ID()
				emission := GetLightProps(blockID).Emission

				if emission > 0 {
					chunk.SetBlockLight(x, y, z, emission)
					worldX := cx*16 + x
					worldZ := cz*16 + z
					queue = append(queue, lightPos{worldX, y, worldZ})
				}
			}
		}
	}

	// Phase 2: BFS to spread block light from sources
	visited := make(map[lightPos]bool)
	for len(queue) > 0 {
		pos := queue[0]
		queue = queue[1:]

		if visited[pos] {
			continue
		}
		visited[pos] = true

		currentLight := le.getBlockLightWorld(pos.x, pos.y, pos.z)
		if currentLight == 0 {
			continue
		}

		// Try to spread to each neighbor
		for _, offset := range [][3]int{{1, 0, 0}, {-1, 0, 0}, {0, 0, 1}, {0, 0, -1}, {0, 1, 0}, {0, -1, 0}} {
			nx, ny, nz := pos.x+offset[0], pos.y+offset[1], pos.z+offset[2]

			if ny < MinWorldHeight || ny >= MinWorldHeight+SectionsPerChunk*16 {
				continue
			}

			neighborBlockID := le.getBlockIDWorld(nx, ny, nz)
			opacity := GetLightProps(neighborBlockID).Opacity

			// Calculate new light level at neighbor (falloff by 1 + opacity)
			newLight := uint8(0)
			if currentLight > opacity+1 {
				newLight = currentLight - opacity - 1
			}

			// Get current neighbor light
			neighborLight := le.getBlockLightWorld(nx, ny, nz)

			// If new light is brighter, update and queue
			if newLight > neighborLight {
				le.setBlockLightWorld(nx, ny, nz, newLight)
				queue = append(queue, lightPos{nx, ny, nz})
			}
		}
	}
}

// getChunkUnlocked retrieves a chunk without acquiring locks.
// Used internally when the caller already holds the appropriate lock.
func (le *LightEngine) getChunkUnlocked(cx, cz int) *Chunk {
	return le.world.chunks[ChunkPos{cx, cz}]
}

// Helper methods for world-coordinate light access.
// These use getChunkUnlocked to avoid deadlocks when called from
// ComputeChunkLight (which is called with locks already held).

func (le *LightEngine) getSkyLightWorld(x, y, z int) uint8 {
	cx, cz := x>>4, z>>4
	chunk := le.getChunkUnlocked(cx, cz)
	if chunk == nil {
		return 0 // unloaded chunk
	}
	return chunk.GetSkyLight(x&15, y, z&15)
}

func (le *LightEngine) setSkyLightWorld(x, y, z int, val uint8) {
	cx, cz := x>>4, z>>4
	chunk := le.getChunkUnlocked(cx, cz)
	if chunk == nil {
		return // unloaded chunk
	}
	chunk.SetSkyLight(x&15, y, z&15, val)
}

func (le *LightEngine) getBlockLightWorld(x, y, z int) uint8 {
	cx, cz := x>>4, z>>4
	chunk := le.getChunkUnlocked(cx, cz)
	if chunk == nil {
		return 0 // unloaded chunk
	}
	return chunk.GetBlockLight(x&15, y, z&15)
}

func (le *LightEngine) setBlockLightWorld(x, y, z int, val uint8) {
	cx, cz := x>>4, z>>4
	chunk := le.getChunkUnlocked(cx, cz)
	if chunk == nil {
		return // unloaded chunk
	}
	chunk.SetBlockLight(x&15, y, z&15, val)
}

func (le *LightEngine) getBlockIDWorld(x, y, z int) int32 {
	cx, cz := x>>4, z>>4
	chunk := le.getChunkUnlocked(cx, cz)
	if chunk == nil {
		return AirID // unloaded chunk treated as air
	}
	return chunk.GetBlock(x&15, y, z&15).ID()
}
