// Package worldgen materializes the foundation-free terrain pipeline into the
// engine's *world.Chunk. terrain produces a buffer of namespaced block names
// (with no world/registry import); this glue resolves those names to canonical
// Java state ids via world.StateID and writes them into a chunk.
package worldgen

import (
	"livingworld/internal/world"
	"livingworld/internal/worldgen/terrain"
)

// Generator turns the deterministic terrain buffer into world chunks. It
// structurally satisfies world.ChunkGenerator.
type Generator struct{ seed int64 }

// NewGenerator returns a deterministic terrain generator for the given seed.
func NewGenerator(seed int64) *Generator { return &Generator{seed: seed} }

var _ world.ChunkGenerator = (*Generator)(nil)

// Generate builds chunk (cx,cz): it runs the terrain pipeline, then translates
// the name buffer into blocks. terrain's world-Y range [MinY,MaxY] maps onto the
// chunk's 0-based column via y-MinY (bedrock at MinY -> chunk Y 0). Empty Air
// and carved CaveAir cells, and any name that resolves to air, are left unset.
func (g *Generator) Generate(cx, cz int) *world.Chunk {
	buf := terrain.Build(g.seed, cx, cz)
	c := world.NewChunk()
	idCache := make(map[string]int32, 8) // resolve each distinct name once, not per block
	for y := terrain.MinY; y <= terrain.MaxY; y++ {
		for z := 0; z < terrain.Size; z++ {
			for x := 0; x < terrain.Size; x++ {
				name := buf.Get(x, y, z)
				if name == terrain.Air || name == terrain.CaveAir {
					continue
				}
				id, ok := idCache[name]
				if !ok {
					id = world.StateID(name)
					idCache[name] = id
				}
				if id != world.AirID {
					c.SetBlock(x, y-terrain.MinY, z, world.BlockByID(id))
				}
			}
		}
	}
	return c
}
