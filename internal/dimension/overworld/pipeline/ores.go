package pipeline

import (
	"livingworld/internal/dimension/overworld/noise"
	"livingworld/internal/world"
)

// oreConfig is one ore (or stone-blob) placement rule, modelled on the
// 1.18+ vanilla distributions: a per-chunk attempt count, a Y
// distribution (uniform or triangle), and a vein size.
type oreConfig struct {
	name     string // for debugging
	block    int32  // block placed in the stone layer
	deep     int32  // deepslate variant (0 = use block everywhere)
	count    int    // veins per chunk
	size     int    // blocks per vein (random walk length)
	yMin     int
	yMax     int
	triangle bool // triangle distribution peaking at the midpoint
	mountain bool // only in mountain biomes (emerald)
}

// buildOreConfigs resolves the 1.18-style ore table once at generator
// construction. Y ranges follow the modern vanilla distributions
// (triangle peaks: iron 16, copper 48, gold -16, diamond deep, lapis 0).
func (g *Generator) buildOreConfigs() []oreConfig {
	id := world.StateID
	return []oreConfig{
		// Stone variety blobs first so ores can overwrite them.
		{name: "dirt", block: id("minecraft:dirt"), count: 7, size: 22, yMin: 0, yMax: 160},
		{name: "gravel", block: id("minecraft:gravel"), count: 6, size: 22, yMin: -64, yMax: 160},
		{name: "granite", block: id("minecraft:granite"), count: 4, size: 32, yMin: 0, yMax: 124},
		{name: "diorite", block: id("minecraft:diorite"), count: 4, size: 32, yMin: 0, yMax: 124},
		{name: "andesite", block: id("minecraft:andesite"), count: 4, size: 32, yMin: 0, yMax: 124},
		{name: "tuff", block: id("minecraft:tuff"), deep: id("minecraft:tuff"), count: 3, size: 30, yMin: -64, yMax: 0},

		{name: "coal", block: id("minecraft:coal_ore"), deep: id("minecraft:deepslate_coal_ore"),
			count: 18, size: 12, yMin: 0, yMax: 190},
		{name: "iron", block: id("minecraft:iron_ore"), deep: id("minecraft:deepslate_iron_ore"),
			count: 10, size: 8, yMin: -24, yMax: 56, triangle: true},
		{name: "iron_small", block: id("minecraft:iron_ore"), deep: id("minecraft:deepslate_iron_ore"),
			count: 4, size: 4, yMin: -63, yMax: 72},
		{name: "copper", block: id("minecraft:copper_ore"), deep: id("minecraft:deepslate_copper_ore"),
			count: 14, size: 9, yMin: -16, yMax: 112, triangle: true},
		{name: "gold", block: id("minecraft:gold_ore"), deep: id("minecraft:deepslate_gold_ore"),
			count: 4, size: 8, yMin: -64, yMax: 32, triangle: true},
		{name: "redstone", block: id("minecraft:redstone_ore"), deep: id("minecraft:deepslate_redstone_ore"),
			count: 4, size: 7, yMin: -63, yMax: 15},
		{name: "redstone_deep", block: id("minecraft:redstone_ore"), deep: id("minecraft:deepslate_redstone_ore"),
			count: 8, size: 7, yMin: -63, yMax: -32, triangle: true},
		{name: "lapis", block: id("minecraft:lapis_ore"), deep: id("minecraft:deepslate_lapis_ore"),
			count: 2, size: 6, yMin: -32, yMax: 32, triangle: true},
		{name: "lapis_uniform", block: id("minecraft:lapis_ore"), deep: id("minecraft:deepslate_lapis_ore"),
			count: 2, size: 6, yMin: -64, yMax: 60},
		{name: "diamond", block: id("minecraft:diamond_ore"), deep: id("minecraft:deepslate_diamond_ore"),
			count: 7, size: 6, yMin: -63, yMax: 14, triangle: true},
		{name: "emerald", block: id("minecraft:emerald_ore"), deep: id("minecraft:deepslate_emerald_ore"),
			count: 5, size: 2, yMin: 60, yMax: 240, triangle: true, mountain: true},
	}
}

// placeOres runs every ore config against the chunk with a
// population-seeded WorldgenRandom, so the same chunk always rolls the
// same veins regardless of generation order.
func (g *Generator) placeOres(c *world.Chunk, cols *[16][16]column, cx, cz int) {
	bx, bz := int32(cx*16), int32(cz*16)
	r := noise.NewWorldgenRandom(g.seed)
	r.SetPopulationSeed(uint64(g.seed), bx, bz)
	popSeed := r.NextLong()

	for i, cfg := range g.ores {
		r.SetFeatureSeed(popSeed, int32(i), 6 /* underground_ores step */)
		for try := 0; try < cfg.count; try++ {
			lx := int(r.NextIntBounded(16))
			lz := int(r.NextIntBounded(16))
			span := int32(cfg.yMax - cfg.yMin + 1)
			var y int
			if cfg.triangle {
				y = cfg.yMin + int(r.NextIntBounded(span)+r.NextIntBounded(span))/2
			} else {
				y = cfg.yMin + int(r.NextIntBounded(span))
			}
			if cfg.mountain {
				bid := cols[lz][lx].biome.ID
				if bid != "minecraft:jagged_peaks" && bid != "minecraft:frozen_peaks" &&
					bid != "minecraft:stony_peaks" && bid != "minecraft:snowy_slopes" &&
					bid != "minecraft:grove" && bid != "minecraft:meadow" &&
					bid != "minecraft:windswept_hills" {
					continue
				}
			}
			g.placeVein(c, r, lx, y, lz, cfg)
		}
	}
}

// placeVein grows a vein as a short random walk from the anchor,
// padding ~half the visited neighbours, which reads like the classic
// vanilla blob. Cells outside this chunk are skipped (veins clip at
// borders; invisible in practice).
func (g *Generator) placeVein(c *world.Chunk, r *noise.WorldgenRandom, x, y, z int, cfg oreConfig) {
	for i := 0; i < cfg.size; i++ {
		g.setOreCell(c, x, y, z, cfg)
		if r.NextBoolean() {
			g.setOreCell(c, x+int(r.NextIntBounded(3))-1, y+int(r.NextIntBounded(3))-1, z+int(r.NextIntBounded(3))-1, cfg)
		}
		x += int(r.NextIntBounded(3)) - 1
		y += int(r.NextIntBounded(3)) - 1
		z += int(r.NextIntBounded(3)) - 1
	}
}

// setOreCell replaces a stone-family cell with the ore (deepslate
// variant in the deepslate layer).
func (g *Generator) setOreCell(c *world.Chunk, x, y, z int, cfg oreConfig) {
	if x < 0 || x > 15 || z < 0 || z > 15 || y <= minY || y > maxY {
		return
	}
	cur := c.GetBlock(x, y, z).ID()
	switch cur {
	case g.ids.stone, g.ids.granite, g.ids.diorite, g.ids.andesite:
		c.SetBlock(x, y, z, world.BlockByID(cfg.block))
	case g.ids.deepslate, g.ids.tuff:
		if cfg.deep != 0 {
			c.SetBlock(x, y, z, world.BlockByID(cfg.deep))
		} else if cfg.block == g.ids.dirt || cfg.block == g.ids.gravel {
			c.SetBlock(x, y, z, world.BlockByID(cfg.block))
		}
	}
}
