package pipeline

import (
	"livingworld/internal/world"
)

// seaLevel is the first air Y above a full ocean: water occupies
// (..., 61, 62], matching vanilla's sea_level = 63.
const seaLevel = 63

// minY / maxY are the canonical Overworld build limits.
const (
	minY = -64
	maxY = 319
)

// lavaLevel: cave cells carved below this Y flood with lava (vanilla -54).
const lavaLevel = -54

// buildColumn writes the full vertical column (lx, lz) of the chunk:
// bedrock floor, deepslate body, stone body, biome surface materials,
// and sea/river water. Caves, ores, and features run afterwards.
func (g *Generator) buildColumn(c *world.Chunk, lx, lz, wx, wz int, col column) {
	top := col.height

	// Dirt (filler) depth varies 2..4 with a small noise field, like
	// vanilla's surface_depth noise.
	dirtDepth := 2 + int(clampF(g.surfNoise.at2(float64(wx), float64(wz))*2.5+1.5, 0, 2.99))

	underwater := top < seaLevel-1

	for y := minY; y <= top; y++ {
		var id int32
		switch {
		case y == minY:
			id = g.ids.bedrock
		case y < minY+5 && hash01(g.seed, wx, y, wz) < float64(minY+5-y)/5.0:
			// Bedrock gradient over -63..-60, denser toward the floor.
			id = g.ids.bedrock
		case y > top-1 && !underwater:
			id = g.surfaceTop(wx, y, wz, col)
		case y > top-1-dirtDepth:
			id = g.surfaceFiller(wx, y, wz, col, top, underwater)
		case y < 0:
			id = g.ids.deepslate
		case y < 8 && hash01(g.seed, wx, y^0x55, wz) < float64(8-y)/8.0:
			// Dithered stone→deepslate transition over 0..8.
			id = g.ids.deepslate
		default:
			id = g.stoneBody(wx, y, wz, col, top)
		}
		c.SetBlock(lx, y, lz, world.BlockByID(id))
	}

	// Sea / river water above the terrain.
	if top < seaLevel-1 {
		for y := top + 1; y <= seaLevel-1; y++ {
			c.SetBlock(lx, y, lz, world.BlockByID(g.ids.water))
		}
		// Frozen biomes freeze the water surface.
		if col.biome.HasSnow && col.temp < -0.4 {
			c.SetBlock(lx, seaLevel-1, lz, world.BlockByID(g.ids.ice))
		}
	}
}

// surfaceTop picks the top-of-column block for the biome.
func (g *Generator) surfaceTop(wx, y, wz int, col column) int32 {
	switch col.biome.ID {
	case "minecraft:badlands", "minecraft:eroded_badlands":
		return g.ids.redSand
	case "minecraft:desert":
		return g.ids.sand
	case "minecraft:beach", "minecraft:warm_ocean", "minecraft:lukewarm_ocean":
		return g.ids.sand
	case "minecraft:snowy_beach":
		return g.ids.sand
	case "minecraft:stony_shore", "minecraft:stony_peaks":
		return g.ids.stone
	case "minecraft:jagged_peaks", "minecraft:frozen_peaks", "minecraft:snowy_slopes":
		return g.ids.snowBlock
	case "minecraft:windswept_gravelly_hills":
		return g.ids.gravel
	case "minecraft:mushroom_fields":
		return g.ids.mycelium
	case "minecraft:mangrove_swamp":
		return g.ids.mud
	case "minecraft:snowy_plains", "minecraft:snowy_taiga", "minecraft:ice_spikes", "minecraft:grove":
		// Vanilla keeps grass under the snow layers; the snow layer is
		// added by the top-layer feature pass.
		return g.ids.grassBlock
	case "minecraft:old_growth_pine_taiga", "minecraft:old_growth_spruce_taiga":
		if hash01(g.seed, wx, 7, wz) < 0.35 {
			return g.ids.podzol
		}
		return g.ids.grassBlock
	default:
		return g.ids.grassBlock
	}
}

// surfaceFiller picks the under-the-top filler band.
func (g *Generator) surfaceFiller(wx, y, wz int, col column, top int, underwater bool) int32 {
	if underwater {
		// Ocean / river floor material.
		switch col.biome.ID {
		case "minecraft:warm_ocean", "minecraft:lukewarm_ocean", "minecraft:deep_lukewarm_ocean",
			"minecraft:river", "minecraft:beach", "minecraft:snowy_beach", "minecraft:desert":
			return g.ids.sand
		case "minecraft:frozen_river":
			return g.ids.gravel
		case "minecraft:mangrove_swamp", "minecraft:swamp":
			return g.ids.mud
		default:
			return g.ids.gravel
		}
	}
	switch col.biome.ID {
	case "minecraft:badlands", "minecraft:eroded_badlands":
		return g.badlandsBand(y)
	case "minecraft:desert", "minecraft:beach", "minecraft:snowy_beach":
		if y > top-4 {
			return g.ids.sand
		}
		return g.ids.sandstone
	case "minecraft:stony_shore", "minecraft:stony_peaks", "minecraft:jagged_peaks", "minecraft:frozen_peaks":
		return g.ids.stone
	case "minecraft:windswept_gravelly_hills":
		return g.ids.gravel
	case "minecraft:mangrove_swamp":
		return g.ids.mud
	default:
		return g.ids.dirt
	}
}

// stoneBody handles the deep body of badlands hills (terracotta strata
// continue through the hill, like vanilla) and plain stone elsewhere.
func (g *Generator) stoneBody(wx, y, wz int, col column, top int) int32 {
	switch col.biome.ID {
	case "minecraft:badlands", "minecraft:eroded_badlands":
		if y > 40 {
			return g.badlandsBand(y)
		}
	}
	return g.ids.stone
}

// badlandsBand returns the terracotta stratum for absolute Y — bands are
// horizontal world-wide layers, which is what makes mesas read as mesas.
func (g *Generator) badlandsBand(y int) int32 {
	switch (y + int(g.bandOffset)) % 15 {
	case 2, 9:
		return g.ids.orangeTerracotta
	case 4:
		return g.ids.yellowTerracotta
	case 6, 7:
		return g.ids.whiteTerracotta
	case 12:
		return g.ids.brownTerracotta
	default:
		return g.ids.terracotta
	}
}
