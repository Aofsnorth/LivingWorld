// Package dimension is the public façade of LivingWorld's dimension
// system. Each dimension is its own subpackage:
//
//	overworld   — the 26.1.2 vanilla Overworld with the full pipeline.
//	nether      — stub: lava ocean + netherrack base layer.
//	end         — stub: void + end stone surface.
//
// The dimension.Dimension interface is what world.ChunkGenerator
// implementations and runtime code (boss rooms, portals, etc) call.
// Each subpackage returns a Dimension via a New<Dim> constructor.
package dimension

import (
	"livingworld/internal/dimension/end"
	"livingworld/internal/dimension/nether"
	"livingworld/internal/dimension/overworld/pipeline"
	"livingworld/internal/world"
)

// ID is the namespaced dimension id (e.g. "minecraft:overworld"). The
// same string the dimension_type JSON uses.
type ID = string

// Canonical dimension ids.
const (
	IDOverworld ID = "minecraft:overworld"
	IDNether    ID = "minecraft:the_nether"
	IDEnd       ID = "minecraft:the_end"
)

// Dimension is the public surface every dimension package returns. The
// Generator method returns a world.ChunkGenerator that the world
// runtime can use to build chunks for this dimension.
type Dimension interface {
	// ID returns the namespaced dimension id.
	ID() ID
	// MinY is the dimension's vertical floor (inclusive).
	MinY() int
	// Height is the dimension's vertical height (MinY..MinY+Height-1).
	Height() int
	// SeaLevel is the dimension's sea level (used by the surface
	// rules and the mob spawn director).
	SeaLevel() int
	// Generator returns a chunk generator ready to be attached to a
	// *world.World. The world's seed is passed in at construction.
	Generator(seed int64) world.ChunkGenerator
}

// NewOverworld returns the canonical Overworld dimension.
func NewOverworld() Dimension { return overworldDimension{} }

// NewNether returns the Nether stub dimension.
func NewNether() Dimension { return netherWrapper{} }

// NewEnd returns the End stub dimension.
func NewEnd() Dimension { return endWrapper{} }

// overworldDimension is the canonical 26.1.2 baseline. The actual
// pipeline lives in overworld/pipeline.
type overworldDimension struct{}

func (overworldDimension) ID() ID        { return IDOverworld }
func (overworldDimension) MinY() int     { return -64 }
func (overworldDimension) Height() int   { return 384 }
func (overworldDimension) SeaLevel() int { return 63 }
func (overworldDimension) Generator(seed int64) world.ChunkGenerator {
	return pipeline.NewGenerator(seed)
}

// netherWrapper adapts the nether.Dimension to the dimension.Dimension
// surface. The Nether's vertical range is 0..127.
type netherWrapper struct{}

func (netherWrapper) ID() ID        { return IDNether }
func (netherWrapper) MinY() int     { return 0 }
func (netherWrapper) Height() int   { return 128 }
func (netherWrapper) SeaLevel() int { return 31 }
func (netherWrapper) Generator(seed int64) world.ChunkGenerator {
	return nether.New(seed)
}

// endWrapper adapts the end.Dimension to the dimension.Dimension
// surface. The End's vertical range is 0..255.
type endWrapper struct{}

func (endWrapper) ID() ID        { return IDEnd }
func (endWrapper) MinY() int     { return 0 }
func (endWrapper) Height() int   { return 256 }
func (endWrapper) SeaLevel() int { return 0 }
func (endWrapper) Generator(seed int64) world.ChunkGenerator {
	return end.New(seed)
}
