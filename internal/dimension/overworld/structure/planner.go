// Package structure is the Overworld's structure planner. It mirrors
// vanilla's net.minecraft.world.level.levelgen.structure.StructureSet
// / StructureState pair: at world-bootstrap time, we build a list of
// 16 StructureSet records, each with its (placement, spacing,
// separation, salt) values. Per chunk, the planner walks the active
// sets and reports which structure "starts" touch the chunk (the
// realization layer fills in the actual pieces).
//
// Vanilla's table (overworld_dimension) includes the 16 sets the
// blueprint calls out: villages, pillager_outposts, desert_pyramids,
// jungle_temples, swamp_huts, igloos, ancient_cities, trial_chambers,
// trail_ruins, ruined_portals, shipwrecks, ocean_ruins, ocean_monuments,
// buried_treasures, mineshafts, strongholds, woodland_mansions.
//
// Two placement shapes are implemented:
//   - random_spread    : chunk grid with frequency / legacy_type variants
//   - concentric_rings : strongholds only
//
// The planner is deterministic in (worldSeed, chunkX, chunkZ) so
// /locate-style queries produce stable results.
package structure

import (
	"livingworld/internal/dimension/overworld/biome"
	"livingworld/internal/dimension/overworld/noise"
	"math"
)

// Placement is the strategy used to scatter structure starts. Mirrors
// vanilla's StructurePlacement enum.
type Placement int

const (
	PlacementRandomSpread Placement = iota
	PlacementConcentricRings
)

// FrequencyValue is the "is this anchor kept" probability. Vanilla
// uses a float in [0, 1]; some sets (buried_treasures, mineshafts)
// use a frequency < 1 to thin the anchor set.
type FrequencyValue float64

// Set is one entry in the world's structure set table.
type Set struct {
	Name        string
	Structure   string
	Placement   Placement
	Spacing     int
	Separation  int
	Salt        int32
	Frequency   FrequencyValue
	// PreferredBiomes gates which biomes the structure can start in.
	// Used by strongholds (#stronghold_biased_to).
	PreferredBiomes []biome.ID
	// SpreadType is the anchor hashing shape. Triangular for
	// monuments / mansions; default for the rest.
	SpreadType SpreadType
	// ExclusionZones, if non-empty, prevents this set from placing
	// within `Radius` chunks of any chunk centre in the list. Used by
	// pillager_outposts (excludes villages).
	ExclusionZones []Exclusion
}

// Exclusion is a per-placement exclusion zone.
type Exclusion struct {
	Other  string // other structure set name
	Radius int
}

// SpreadType controls the "anchor hashing" shape. Vanilla uses
// triangular for monuments, mansions, and some others; the rest are
// default (uniform). Currently informational only — the planner hashes
// the same way for both.
type SpreadType int

const (
	SpreadDefault SpreadType = iota
	SpreadTriangular
)

// Start is one structure anchor the planner produced for a chunk. The
// (BlockX, BlockZ) is the chunk-aligned block origin; Structure tells
// the realization layer which structure to lay down.
type Start struct {
	Structure string
	BlockX    int
	BlockZ    int
}

// AllOverworldSets returns the 16 vanilla structure sets the blueprint
// calls out. Spacing, separation, salt, and frequency match the values
// from the 26.1.2 overworld_structure_set table.
func AllOverworldSets() []Set {
	return []Set{
		{Name: "minecraft:villages", Structure: "minecraft:village", Placement: PlacementRandomSpread, Spacing: 34, Separation: 8, Salt: 10387312, Frequency: 1.0},
		{Name: "minecraft:desert_pyramids", Structure: "minecraft:desert_pyramid", Placement: PlacementRandomSpread, Spacing: 32, Separation: 8, Salt: 14357617, Frequency: 1.0},
		{Name: "minecraft:igloos", Structure: "minecraft:igloo", Placement: PlacementRandomSpread, Spacing: 32, Separation: 8, Salt: 14357618, Frequency: 1.0},
		{Name: "minecraft:jungle_temples", Structure: "minecraft:jungle_pyramid", Placement: PlacementRandomSpread, Spacing: 32, Separation: 8, Salt: 14357619, Frequency: 1.0},
		{Name: "minecraft:swamp_huts", Structure: "minecraft:swamp_hut", Placement: PlacementRandomSpread, Spacing: 32, Separation: 8, Salt: 14357620, Frequency: 1.0},
		{Name: "minecraft:pillager_outposts", Structure: "minecraft:pillager_outpost", Placement: PlacementRandomSpread, Spacing: 32, Separation: 8, Salt: 165745296, Frequency: 0.2,
			ExclusionZones: []Exclusion{{Other: "minecraft:villages", Radius: 10}}},
		{Name: "minecraft:ancient_cities", Structure: "minecraft:ancient_city", Placement: PlacementRandomSpread, Spacing: 24, Separation: 8, Salt: 20083232, Frequency: 1.0},
		{Name: "minecraft:trail_ruins", Structure: "minecraft:trail_ruins", Placement: PlacementRandomSpread, Spacing: 34, Separation: 8, Salt: 83469867, Frequency: 1.0},
		{Name: "minecraft:trial_chambers", Structure: "minecraft:trial_chambers", Placement: PlacementRandomSpread, Spacing: 34, Separation: 12, Salt: 94251327, Frequency: 1.0},
		{Name: "minecraft:ocean_monuments", Structure: "minecraft:ocean_monument", Placement: PlacementRandomSpread, Spacing: 32, Separation: 5, Salt: 10387313, Frequency: 1.0, SpreadType: SpreadTriangular},
		{Name: "minecraft:woodland_mansions", Structure: "minecraft:woodland_mansion", Placement: PlacementRandomSpread, Spacing: 80, Separation: 20, Salt: 10387319, Frequency: 1.0, SpreadType: SpreadTriangular},
		{Name: "minecraft:ocean_ruins", Structure: "minecraft:ocean_ruin", Placement: PlacementRandomSpread, Spacing: 20, Separation: 8, Salt: 14357621, Frequency: 1.0},
		{Name: "minecraft:shipwrecks", Structure: "minecraft:shipwreck", Placement: PlacementRandomSpread, Spacing: 24, Separation: 4, Salt: 165745295, Frequency: 1.0},
		{Name: "minecraft:ruined_portals", Structure: "minecraft:ruined_portal", Placement: PlacementRandomSpread, Spacing: 40, Separation: 15, Salt: 34222645, Frequency: 1.0},
		{Name: "minecraft:buried_treasures", Structure: "minecraft:buried_treasure", Placement: PlacementRandomSpread, Spacing: 1, Separation: 0, Salt: 0, Frequency: 0.01},
		{Name: "minecraft:mineshafts", Structure: "minecraft:mineshaft", Placement: PlacementRandomSpread, Spacing: 1, Separation: 0, Salt: 0, Frequency: 0.004},
		{Name: "minecraft:strongholds", Structure: "minecraft:stronghold", Placement: PlacementConcentricRings, Spacing: 0, Separation: 0, Salt: 0, Frequency: 1.0},
	}
}

// AllOverworldSets is the public alias; the table is fixed for the
// Overworld dimension so a single global is fine.
var Sets = AllOverworldSets()

// Planner is the per-world structure planner. Built once at world
// bootstrap; safe for concurrent use (the noise calls are stateless).
type Planner struct {
	Sets  []Set
	Seed  int64
	World *noise.WorldgenRandom
}

// NewPlanner builds a planner for the world seed.
func NewPlanner(seed int64) *Planner {
	return &Planner{Sets: Sets, Seed: seed, World: noise.NewWorldgenRandom(seed)}
}

// FindStartsForChunk returns the list of structure starts that touch
// the given chunk. This is the "planning" stage; the realization
// layer (vanilla's StructureStart) lays down the actual pieces.
func (p *Planner) FindStartsForChunk(chunkX, chunkZ int) []Start {
	var out []Start
	for _, s := range p.Sets {
		switch s.Placement {
		case PlacementConcentricRings:
			if st, ok := p.strongholdAtChunk(chunkX, chunkZ, s); ok {
				out = append(out, st)
			}
		default:
			if st, ok := p.randomSpreadAnchor(chunkX, chunkZ, s); ok {
				out = append(out, st)
			}
		}
	}
	return out
}

// randomSpreadAnchor runs the random_spread shape for a chunk. The
// chunk's position in the cell grid is (chunkX // Spacing, chunkZ //
// Spacing). The anchor for that cell is decided by seeding a
// WorldgenRandom with the cell's chunk XZ + the set's salt, then
// taking a random value in [0, Spacing-Separation).
func (p *Planner) randomSpreadAnchor(chunkX, chunkZ int, s Set) (Start, bool) {
	if s.Spacing <= 0 {
		return Start{}, false
	}
	cellX := floorDiv(chunkX, s.Spacing)
	cellZ := floorDiv(chunkZ, s.Spacing)
	for _, cdx := range []int{0, -1, 1} {
		for _, cdz := range []int{0, -1, 1} {
			cx := cellX + cdx
			cz := cellZ + cdz
			r := noise.NewWorldgenRandom(p.Seed)
			r.SetLargeFeatureWithSalt(p.Seed, int32(cx), int32(cz), s.Salt)
			offsetX := int(r.NextIntBounded(int32(s.Spacing - s.Separation)))
			offsetZ := int(r.NextIntBounded(int32(s.Spacing - s.Separation)))
			anchorChunkX := cx*s.Spacing + offsetX
			anchorChunkZ := cz*s.Spacing + offsetZ
			if anchorChunkX == chunkX && anchorChunkZ == chunkZ {
				if s.Frequency < 1.0 {
					rr := r.Fork()
					if rr.NextDouble() > float64(s.Frequency) {
						return Start{}, false
					}
				}
				return Start{Structure: s.Structure, BlockX: anchorChunkX * 16, BlockZ: anchorChunkZ * 16}, true
			}
		}
	}
	return Start{}, false
}

// strongholdAtChunk runs the concentric_rings shape Mojang uses for
// strongholds. The 128 strongholds are placed on a ring of radius
// 32*128 blocks, with spread 3 (chunk-level). The closest anchor to
// the chunk decides placement.
func (p *Planner) strongholdAtChunk(chunkX, chunkZ int, s Set) (Start, bool) {
	const count = 128
	const distance = 32
	const spread = 3
	const salt = 0
	r := noise.NewWorldgenRandom(p.Seed)
	r.SetLargeFeatureWithSalt(p.Seed, 0, 0, int32(salt))
	// Best anchor in the chunk grid.
	best := Start{Structure: s.Structure}
	bestDist := math.MaxFloat64
	for i := 0; i < count; i++ {
		ax := int(r.NextIntBounded(int32(distance * 2)))
		az := int(r.NextIntBounded(int32(distance * 2)))
		// Translate to chunk-aligned.
		chunkAX := ax - distance
		chunkAZ := az - distance
		// Jitter by ±spread.
		chunkAX += int(r.NextIntBounded(int32(spread*2))) - spread
		chunkAZ += int(r.NextIntBounded(int32(spread*2))) - spread
		d := float64((chunkAX-chunkX)*(chunkAX-chunkX) + (chunkAZ-chunkZ)*(chunkAZ-chunkZ))
		if d < bestDist {
			bestDist = d
			best.BlockX = chunkAX * 16
			best.BlockZ = chunkAZ * 16
		}
	}
	if bestDist > 1 {
		return Start{}, false
	}
	return best, true
}

// floorDiv is true floor division (matches the world.ChunkCoord semantics).
func floorDiv(a, b int) int {
	if a >= 0 {
		return a / b
	}
	return -((-a + b - 1) / b)
}
