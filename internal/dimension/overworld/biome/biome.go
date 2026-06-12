// Package biome is the canonical registry of Overworld biomes. It mirrors
// Minecraft Java Edition 26.1.2's 54-biome Overworld set with the data
// the worldgen pipeline reads:
//
//   - ID and category for /locate lookups.
//   - climate parameters (temperature, downfall) for ambient spawns / fog.
//   - 6D climate parameter target for OverworldBiomeBuilder. The
//     (temperature, humidity, continentalness, erosion, depth, weirdness)
//     point closest to the sampled climate field wins — this is exactly
//     the shape vanilla uses.
//   - terrain shaping (base height, height variation) for the
//     final_density router.
//   - surface material set (top block, filler block, underwater filler) for
//     the surface rules stage.
//
// The package is data-only. Selection logic lives in the climate package
// (the multi-noise sampler + OverworldBiomeBuilder) so this file can be
// edited without touching the router.
package biome

// Category groups biomes into the high-level families the blueprint calls
// out. Used by feature placement, mob spawning, and the per-biome vanilla
// classification.
type Category int

const (
	CategoryUnknown Category = iota
	CategoryPlains
	CategoryForest
	CategoryTaiga
	CategorySwamp
	CategoryDesert
	CategorySavanna
	CategoryJungle
	CategoryBadlands
	CategoryMushroom
	CategoryBeach
	CategoryOcean
	CategoryRiver
	CategoryMountain
	CategoryHills
	CategoryCave
)

// ID is the namespaced biome identifier (e.g. "minecraft:plains"). Vanilla
// uses the namespace as a separate field; we keep the full id for one-key
// lookups.
type ID = string

// All canonical Overworld biome ids. Kept as a slice so callers can range
// in stable order. The slice is the single source of truth for "is this a
// valid Overworld biome?".
var OverworldBiomes = []ID{
	"minecraft:plains",
	"minecraft:sunflower_plains",
	"minecraft:forest",
	"minecraft:flower_forest",
	"minecraft:birch_forest",
	"minecraft:old_growth_birch_forest",
	"minecraft:dark_forest",
	"minecraft:pale_garden",
	"minecraft:taiga",
	"minecraft:old_growth_pine_taiga",
	"minecraft:old_growth_spruce_taiga",
	"minecraft:snowy_taiga",
	"minecraft:swamp",
	"minecraft:mangrove_swamp",
	"minecraft:mushroom_fields",
	"minecraft:desert",
	"minecraft:savanna",
	"minecraft:savanna_plateau",
	"minecraft:windswept_savanna",
	"minecraft:jungle",
	"minecraft:sparse_jungle",
	"minecraft:bamboo_jungle",
	"minecraft:badlands",
	"minecraft:eroded_badlands",
	"minecraft:wooded_badlands",
	"minecraft:snowy_plains",
	"minecraft:ice_spikes",
	"minecraft:grove",
	"minecraft:snowy_slopes",
	"minecraft:jagged_peaks",
	"minecraft:frozen_peaks",
	"minecraft:stony_peaks",
	"minecraft:meadow",
	"minecraft:cherry_grove",
	"minecraft:windswept_hills",
	"minecraft:windswept_forest",
	"minecraft:windswept_gravelly_hills",
	"minecraft:ocean",
	"minecraft:deep_ocean",
	"minecraft:cold_ocean",
	"minecraft:deep_cold_ocean",
	"minecraft:lukewarm_ocean",
	"minecraft:deep_lukewarm_ocean",
	"minecraft:warm_ocean",
	"minecraft:frozen_ocean",
	"minecraft:deep_frozen_ocean",
	"minecraft:river",
	"minecraft:frozen_river",
	"minecraft:beach",
	"minecraft:snowy_beach",
	"minecraft:stony_shore",
	"minecraft:dripstone_caves",
	"minecraft:lush_caves",
	"minecraft:deep_dark",
}

// SurfaceBlock is a single block-name entry (vanilla namespaced id) for
// the surface rule stage. We keep the names here so the dimension layer
// can resolve them once into state ids and write them into the chunk.
type SurfaceBlock struct {
	// Top is the block one cell above the filler. For non-snowy biomes
	// this is grass_block / sand / coarse_dirt / etc.
	Top string
	// Underwater is the block to use when the cell is below sea level.
	// For non-beach biomes this is dirt / sand / gravel. For snowy biomes
	// it is snow_block.
	Underwater string
	// Filler is the block directly below the top block (e.g. dirt under
	// grass_block).
	Filler string
}

// Parameters is the full per-biome descriptor fed into OverworldBiomeBuilder
// and the surface rule stage.
type Parameters struct {
	ID          ID
	Category    Category
	Temperature float64 // vanilla 0.0..1.0 (0=ice, 0.2=snow, 0.5=plains, 0.95=desert)
	Downfall    float64 // vanilla 0.0..1.0
	// ClimateTarget is the 6D point the climate sampler scores. The fields
	// are normalise, vanilla style: T/H/C/E/W are roughly [-1, 1], D is
	// either 0 (surface) or 1 (deep / cave), and the magnitude doesn't
	// matter — OverworldBiomeBuilder uses a weighted distance.
	ClimateTarget ClimatePoint
	// BaseHeight / HeightVariation feed final_density. They are the
	// "terrain shape" vanilla encodes as MysteriousMountainExtraNoise
	// shifts. Numbers are in blocks above the noise floor.
	BaseHeight      float64
	HeightVariation float64
	Surface         SurfaceBlock
	// HasSnow is a shortcut for the surface rule. True means the
	// climate target is cold enough that snow layers accumulate
	// in cold biomes.
	HasSnow bool
	// CaveRules flags biomes that only spawn in caves (dripstone,
	// lush, deep_dark). The multi-noise sampler treats depth=1 as
	// the cave domain and only consults cave biomes there.
	CaveRules bool
}

// ClimatePoint is the 6D point the multi-noise sampler compares to.
type ClimatePoint struct {
	// Temperature: -1 cold → 1 hot. Vanilla splits this into T<0 / T<0.1
	// for snow / freezing, and T>0.5 for hot biomes.
	Temperature float64
	// Humidity / Vegetation: -1 dry → 1 wet. Vanilla uses the "downfall"
	// axis for humidity at selection time.
	Humidity float64
	// Continentalness: -1 deep ocean → -0.11 beach → 0 coast → 0.3
	// inland → 1 far inland. Aquifer depth depends on this.
	Continentalness float64
	// Erosion: 0 valleys → 0.6 ridges → 1 mountain tops. Vanilla
	// uses erosion == 0 to gate erosion-only biomes (ice spikes).
	Erosion float64
	// Depth: 0 surface, 1 underground (caves). Vanilla uses this to
	// route between surface biomes and cave biomes.
	Depth float64
	// Weirdness / Ridges: -1 valleys → 0 mid → 1 mountain ridges.
	// Peaky biomes (jagged_peaks) are gated on this.
	Weirdness float64
}

// All returns the full Overworld parameter table. Index 0 is the default
// fallback; the rest are sorted by category then by name for stability.
func All() []Parameters { return registry }

// ByID returns the Parameters for a biome id, or the fallback (plains) if
// the id is unknown. The fallback keeps chunk generation alive even when
// the climate sampler asks for a biome that isn't in the registry — a
// future datapack that adds a custom biome shouldn't crash the chunk.
func ByID(id ID) Parameters {
	for _, p := range registry {
		if p.ID == id {
			return p
		}
	}
	return registry[0] // plains
}

// HasOverworldBiome reports whether id is a known Overworld biome. Used
// to gate the overworld generator (Nether / End skip the multi-noise
// sampler).
func HasOverworldBiome(id ID) bool {
	for _, p := range registry {
		if p.ID == id {
			return true
		}
	}
	return false
}
