package pipeline

import (
	"livingworld/internal/dimension/overworld/biome"
)

// column is the per-column scratch the pipeline computes once and every
// later stage (surface, ores, features) reads.
type column struct {
	height int // Y of the highest solid terrain block
	biome  biome.Parameters
	temp   float64 // climate temperature  [-1, 1]
	humid  float64 // climate humidity     [-1, 1]
	cont   float64 // continentalness      [-1, 1]
	eros   float64 // erosion              [-1, 1]
}

// shapeColumn computes the terrain column at world (wx, wz). It is a
// pure function of (seed, wx, wz) — features anchored in neighbouring
// chunks re-evaluate it to find their ground height, so it must stay
// chunk-independent. The height comes from the vanilla final_density
// tree (see terrainTop); the biome comes from the vanilla multi-noise
// parameter list (see quartDecision).
func (g *Generator) shapeColumn(wx, wz int) column {
	return g.shapeColumnWith(g.terrainTop(wx, wz), wx, wz)
}

// BiomeAt returns the biome label of a column without evaluating the vanilla
// terrain density — the cheap path for decoration recipes and tooling. The
// decision samples the same vanilla climate the chunk pass does.
func (g *Generator) BiomeAt(wx, wz int) biome.ID {
	return g.quartDecision(wx, g.terrainTop(wx, wz), wz).biome.ID
}

// shapeColumnWith assembles a column around a precomputed terrain top so
// chunk generation can batch the expensive density sampling into the chunk
// grid instead of per-column calls. Climate and biome both come from the
// vanilla router (quartDecision).
func (g *Generator) shapeColumnWith(top, wx, wz int) column {
	d := g.quartDecision(wx, top, wz)
	return column{
		height: top,
		biome:  d.biome,
		temp:   d.temp,
		humid:  d.humid,
		cont:   d.cont,
		eros:   d.eros,
	}
}
