package generator

import "livingworld/internal/world"

// Superflat generates the classic flat world: a bedrock floor, two dirt layers,
// and a grass surface. Block IDs are resolved from the global palette by name,
// so they map cleanly to both Java state IDs and Bedrock runtime IDs.
type Superflat struct {
	bedrock int32
	dirt    int32
	grass   int32
	GroundY int
}

func NewSuperflat() *Superflat {
	return &Superflat{
		bedrock: world.StateID("minecraft:bedrock"),
		dirt:    world.StateID("minecraft:dirt"),
		grass:   world.StateID("minecraft:grass_block"),
		GroundY: world.SuperflatGroundY,
	}
}

func (g *Superflat) Generate(cx, cz int) *world.Chunk {
	c := world.NewChunk()
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			c.SetBlock(x, 0, z, world.BlockByID(g.bedrock))
			c.SetBlock(x, 1, z, world.BlockByID(g.dirt))
			c.SetBlock(x, 2, z, world.BlockByID(g.dirt))
			c.SetBlock(x, g.GroundY, z, world.BlockByID(g.grass))
		}
	}
	return c
}
