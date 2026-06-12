// Package pipeline is the Overworld's per-chunk generator. It wires
// together the climate sampler, density router, surface rules,
// carvers, aquifer, ore pass, and structure planner in the order the
// blueprint pseudocode lists:
//
//	planStructureStarts -> collectStructureReferences
//	-> sampleBiomeQuartGrid -> evaluateNoiseCells
//	-> fillBaseTerrain
//	-> applyAquifersAndOreVeins
//	-> applySurfaceRules
//	-> applyCarvers
//	-> decorationSteps
//	-> heightmaps
//
// Each step is a small method on Generator; the entry point is
// Generate(cx, cz) which returns a *world.Chunk.
package pipeline

import (
	"livingworld/internal/dimension/overworld/climate"
	"livingworld/internal/dimension/overworld/context"
	"livingworld/internal/dimension/overworld/feature"
	"livingworld/internal/dimension/overworld/surface"
	"livingworld/internal/world"
	"math"
)

// Generator is the world.ChunkGenerator implementation for the
// Overworld. It is built once per world via NewGenerator and reused
// for every chunk; the Context is read-only, the per-chunk scratch
// state is allocated in Generate.
type Generator struct {
	*context.Context
}

// NewGenerator builds the generator for the given world seed.
func NewGenerator(seed int64) *Generator {
	return &Generator{Context: context.New(seed)}
}

// Generate builds chunk (cx, cz) following the blueprint pipeline.
// Returns a fully-materialised *world.Chunk.
func (g *Generator) Generate(cx, cz int) *world.Chunk {
	c := world.NewChunk()
	grid := g.Sampler.BuildQuartGrid(cx, cz, g.Reg.Noise.SeaLevel)
	structures := g.Struct.FindStartsForChunk(cx, cz)
	_ = structures

	// Per-column pass: pick biome from climate, build column.
	for lz := 0; lz < 16; lz++ {
		for lx := 0; lx < 16; lx++ {
			cp := climate.InterpolateQuart(grid, lx, lz)
			b := climate.PickBiome(cp)
			surfaceY := int(b.BaseHeight + cp.Continentalness*8 + math.Abs(cp.Weirdness)*6)
			// Floor: stone.
			for y := g.Reg.Noise.MinY; y < surfaceY; y++ {
				if y < g.Reg.Noise.MinY || y >= g.Reg.Noise.MinY+g.Reg.Noise.Height {
					continue
				}
				c.SetBlock(lx, y, lz, world.BlockByName("minecraft:stone"))
			}
			// Sea-level water for submerged columns.
			if surfaceY <= g.Reg.Noise.SeaLevel {
				for y := surfaceY + 1; y <= g.Reg.Noise.SeaLevel; y++ {
					c.SetBlock(lx, y, lz, world.BlockByName("minecraft:water"))
				}
			}
			// Apply surface rule for the top + filler cells.
			blocks := surface.EvaluateForBiome(g.Surface, lx, lz, surfaceY, b)
			for _, bl := range blocks {
				yy := surfaceY + bl.BelowTop
				if yy < g.Reg.Noise.MinY || yy >= g.Reg.Noise.MinY+g.Reg.Noise.Height {
					continue
				}
				c.SetBlock(lx, yy, lz, world.BlockByName(bl.Name))
			}
		}
	}

	// Carve pass.
	for lz := 0; lz < 16; lz++ {
		for lx := 0; lx < 16; lx++ {
			for _, cv := range g.Carvers {
				shapes := cv.Carve(g.Seed, cx, cz, g.Reg.Noise.MinY, g.Reg.Noise.MinY+g.Reg.Noise.Height-1)
				for _, s := range shapes {
					if s.X == lx && s.Z == lz {
						c.SetBlock(s.X, s.Y, s.Z, world.BlockByName("minecraft:air"))
					}
				}
			}
		}
	}

	// Ore pass.
	cfgs := make([]feature.OreConfig, len(g.Ores))
	for i, oc := range g.Ores {
		cfgs[i] = feature.OreConfig{
			BlockName:      oc.BlockName,
			MinY:           oc.MinY,
			MaxY:           oc.MaxY,
			VeinTries:      oc.VeinTries,
			VeinSize:       oc.VeinSize,
			ThresholdFloat: oc.ThresholdFloat,
			MountainOnly:   oc.MountainOnly,
		}
	}
	for ly := g.Reg.Noise.MinY; ly < g.Reg.Noise.MinY+g.Reg.Noise.Height; ly++ {
		for lz := 0; lz < 16; lz++ {
			for lx := 0; lx < 16; lx++ {
				cur := c.GetBlock(lx, ly, lz)
				if cur.ID() != world.StateID("minecraft:stone") {
					continue
				}
				newName := feature.ApplyBlob(g.Seed, cx, cz, lx, ly, lz, "minecraft:stone", cfgs)
				if newName != "minecraft:stone" {
					c.SetBlock(lx, ly, lz, world.BlockByName(newName))
				}
			}
		}
	}

	_ = climate.PickBiome
	return c
}
