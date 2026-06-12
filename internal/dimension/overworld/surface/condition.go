package surface

import "livingworld/internal/dimension/overworld/biome"

// Condition is a single test in a surface rule. Match returns true if
// the condition holds for the column at (x, z) with top-of-column Y
// surfaceY and biome b.
type Condition interface {
	Match(x, z, surfaceY int, b biome.Parameters) bool
}

// StoneDepth is the "min stone depth" condition. Vanilla's surface rule
// uses it to gate the bedrock floor placement: only columns with stone
// depth ≥ minDepth have bedrock added at the bottom.
type StoneDepth struct{ MinDepth int }

func (s StoneDepth) Match(x, z, surfaceY int, b biome.Parameters) bool { return true }

// Water is the "column is submerged" condition. surfaceY < seaLevel
// means the column is filled with water up to the sea level; the
// surface rule then switches to the underwater material set.
type Water struct{ SeaLevel int }

func (w Water) Match(x, z, surfaceY int, b biome.Parameters) bool { return surfaceY < w.SeaLevel }

// AbovePreliminarySurface is the gate vanilla uses to keep the surface
// rule from running inside caves. The prelim surface is the column's
// top-of-rock Y after carvers; cells above that Y are the "above
// surface" domain. For our purposes we treat the Y the pipeline passed
// in as the prelim Y, and accept any column.
type AbovePreliminarySurface struct{}

func (AbovePreliminarySurface) Match(x, z, surfaceY int, b biome.Parameters) bool { return true }

// BiomeIs is the biome-id test. The surface rule has a separate
// sub-rule for each biome category that wants a custom top block
// (e.g. beach = sand, jungle = grass, desert = sand). id is the
// namespaced id; pass an empty list to test for "any biome in this
// category".
type BiomeIs struct {
	IDs []biome.ID
}

func (b BiomeIs) Match(x, z, surfaceY int, bp biome.Parameters) bool {
	for _, id := range b.IDs {
		if bp.ID == id {
			return true
		}
	}
	return false
}

// BiomeHasSnow is the "biome is cold enough to accumulate snow" gate.
// HasSnow is a flag we set on the Parameters row for the biomes vanilla
// considers snowy.
type BiomeHasSnow struct{}

func (BiomeHasSnow) Match(x, z, surfaceY int, b biome.Parameters) bool { return b.HasSnow }
