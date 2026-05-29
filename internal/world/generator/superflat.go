package generator

import "livingworld/internal/world"

type Superflat struct {
	BedrockID int32
	DirtID    int32
	GrassID   int32
	GroundY   int
}

func NewSuperflat() *Superflat {
	return &Superflat{
		BedrockID: 1,
		DirtID:    2,
		GrassID:   3,
		GroundY:   world.SuperflatGroundY,
	}
}

func (g *Superflat) Generate(cx, cz int) *world.Chunk {
	c := world.NewChunk()
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			c.SetBlock(x, 0, z, world.BlockByID(g.BedrockID))
			c.SetBlock(x, 1, z, world.BlockByID(g.DirtID))
			c.SetBlock(x, 2, z, world.BlockByID(g.DirtID))
			c.SetBlock(x, g.GroundY, z, world.BlockByID(g.GrassID))
		}
	}
	return c
}
