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
// Called from the tick loop. It returns the chunks whose light actually changed,
// so the caller can re-send light to players (a no-op relight — common when a
// neighbour load doesn't affect already-correct open terrain — reports nothing
// and triggers no network traffic).
func (le *LightEngine) ProcessUpdates() []ChunkPos {
	le.mu.Lock()
	pending := le.pending
	le.pending = make(map[ChunkPos]struct{})
	le.mu.Unlock()

	if len(pending) == 0 {
		return nil
	}

	// Keep the world read lock for the whole batch. ComputeChunkLight's
	// world-coordinate helpers intentionally use getChunkUnlocked because chunk
	// generation calls this while already holding world.mu; without a lock here,
	// tick-time light recomputation can race a concurrent chunk load/unload and
	// panic on a concurrent map read/write. Fingerprints are snapshotted for the
	// whole batch BEFORE any recompute so a cross-chunk write from one relight is
	// still detected as a change on the chunk it touched.
	type relit struct {
		pos    ChunkPos
		chunk  *Chunk
		before uint64
	}
	le.world.mu.RLock()
	defer le.world.mu.RUnlock()

	items := make([]relit, 0, len(pending))
	for pos := range pending {
		if c := le.world.chunks[pos]; c != nil {
			items = append(items, relit{pos: pos, chunk: c, before: lightFingerprint(c)})
		}
	}
	for _, it := range items {
		le.ComputeChunkLight(it.chunk, it.pos.X, it.pos.Z)
	}
	var changed []ChunkPos
	for _, it := range items {
		if lightFingerprint(it.chunk) != it.before {
			changed = append(changed, it.pos)
		}
	}
	return changed
}

// lightFingerprint is an FNV-1a hash over a chunk's sky and block light, used to
// detect whether a recompute actually changed anything (cheap relative to the
// BFS, ~98 KB per chunk).
func lightFingerprint(c *Chunk) uint64 {
	const (
		offset = uint64(14695981039346656037)
		prime  = uint64(1099511628211)
	)
	h := offset
	for i := range c.sections {
		for _, arr := range [2][]byte{c.sections[i].skyLight, c.sections[i].blockLight} {
			for _, b := range arr {
				h = (h ^ uint64(b)) * prime
			}
		}
	}
	return h
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

// stepLoss is the sky/block light lost crossing into a block of the given
// opacity: at least 1 per block, more for translucent blocks. Matches vanilla's
// max(1, opacity).
func stepLoss(opacity uint8) uint8 {
	if opacity < 1 {
		return 1
	}
	return opacity
}

// lightOffsets are the 6 cardinal neighbour directions used by light BFS. The
// last entry {0,-1,0} (straight down) is special-cased for sky light.
var lightOffsets = [6][3]int{{1, 0, 0}, {-1, 0, 0}, {0, 0, 1}, {0, 0, -1}, {0, 1, 0}, {0, -1, 0}}

const lightTopY = MinWorldHeight + SectionsPerChunk*16 - 1 // highest canonical Y (319)

// computeSkyLight recomputes sky light for a chunk from scratch using a single
// seeded BFS, matching vanilla semantics:
//   - every block above its column heightmap has direct sky access (15);
//   - light propagates losing max(1, opacity) per step, EXCEPT straight down into
//     a fully transparent block, which loses nothing — a sunlit vertical shaft
//     stays at 15 until something attenuating blocks it.
//
// Every one of the chunk's own sky-light cells is reinitialised here (15 above
// the heightmap, 0 below) before the BFS fills the rest, so a recompute after a
// block change correctly lowers stale-bright cells. Cross-chunk reads pull from
// already-loaded neighbours via the world-coord helpers, so a shared edge that
// was lit before its neighbour existed settles correctly once the neighbour is
// queued for relight (see World.queueNeighborRelight).
func (le *LightEngine) computeSkyLight(chunk *Chunk, cx, cz int) {
	var queue []lightPos

	// Seed: reinitialise the column and collect every sky-exposed source. Seeding
	// all exposed cells (not just the lowest) is required so light still reaches
	// under overhangs/floating terrain from the side.
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			heightY := int(chunk.GetHeightmap(x, z))
			worldX, worldZ := cx*16+x, cz*16+z
			for y := lightTopY; y > heightY; y-- {
				chunk.SetSkyLight(x, y, z, 15)
				queue = append(queue, lightPos{worldX, y, worldZ})
			}
			for y := heightY; y >= MinWorldHeight; y-- {
				chunk.SetSkyLight(x, y, z, 0)
			}
		}
	}

	// BFS: monotonically raise neighbour light until nothing can be brightened.
	for len(queue) > 0 {
		pos := queue[0]
		queue = queue[1:]
		cur := le.getSkyLightWorld(pos.x, pos.y, pos.z)
		if cur == 0 {
			continue
		}
		for _, off := range lightOffsets {
			nx, ny, nz := pos.x+off[0], pos.y+off[1], pos.z+off[2]
			if ny < MinWorldHeight || ny > lightTopY {
				continue
			}
			opacity := GetLightProps(le.getBlockIDWorld(nx, ny, nz)).Opacity

			var newLight uint8
			if off == [3]int{0, -1, 0} && cur == 15 && opacity == 0 {
				newLight = 15 // straight down through a transparent block: no loss
			} else if loss := stepLoss(opacity); cur > loss {
				newLight = cur - loss
			}

			if newLight > le.getSkyLightWorld(nx, ny, nz) {
				le.setSkyLightWorld(nx, ny, nz, newLight)
				queue = append(queue, lightPos{nx, ny, nz})
			}
		}
	}
}

// computeBlockLight recomputes block light for a chunk from scratch using BFS
// from emitters, losing max(1, opacity) per step. Every cell is reinitialised to
// its own emission first, so a recompute after a light source is removed
// correctly clears stale light.
func (le *LightEngine) computeBlockLight(chunk *Chunk, cx, cz int) {
	var queue []lightPos
	// Seed: reinitialise every cell to its own emission and collect emitters.
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			worldX, worldZ := cx*16+x, cz*16+z
			for y := MinWorldHeight; y <= lightTopY; y++ {
				emission := GetLightProps(chunk.GetBlock(x, y, z).ID()).Emission
				chunk.SetBlockLight(x, y, z, emission)
				if emission > 0 {
					queue = append(queue, lightPos{worldX, y, worldZ})
				}
			}
		}
	}

	// BFS: monotonically raise neighbour light until nothing can be brightened.
	for len(queue) > 0 {
		pos := queue[0]
		queue = queue[1:]
		cur := le.getBlockLightWorld(pos.x, pos.y, pos.z)
		if cur == 0 {
			continue
		}
		for _, off := range lightOffsets {
			nx, ny, nz := pos.x+off[0], pos.y+off[1], pos.z+off[2]
			if ny < MinWorldHeight || ny > lightTopY {
				continue
			}
			opacity := GetLightProps(le.getBlockIDWorld(nx, ny, nz)).Opacity

			var newLight uint8
			if loss := stepLoss(opacity); cur > loss {
				newLight = cur - loss
			}

			if newLight > le.getBlockLightWorld(nx, ny, nz) {
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
