package surface

import "livingworld/internal/dimension/overworld/biome"

// NewOverworldProgram returns the compiled surface rule for the
// Overworld. It mirrors the rules in
// data/minecraft/worldgen/noise_settings/overworld.json
// (surface_rule block) — bedrock floor, then the per-biome
// above_preliminary_surface branch, then the fall-through default.
//
// The list is read top-to-bottom; the first match wins. The last
// (implicit) rule is "DefaultTop / DefaultFiller" which the pipeline
// fills in from the biome's Surface row.
func NewOverworldProgram() *Program {
	return &Program{
		Rules: []Rule{
			// 1. Bedrock floor: for any column with stone depth >= 0,
			//    replace the bottom 0..4 cells with bedrock, fading
			//    to stone. Vanilla's noise_settings has this as a
			//    vertical_gradient bedrock floor before
			//    above_preliminary_surface; we model it as a single
			//    sequence the pipeline applies cell-by-cell.
			{
				Cond: StoneDepth{MinDepth: 0},
				Blocks: []Block{
					{Name: "minecraft:bedrock", BelowTop: -5},
					{Name: "minecraft:bedrock", BelowTop: -4},
					{Name: "minecraft:bedrock", BelowTop: -3},
					{Name: "minecraft:bedrock", BelowTop: -2},
					{Name: "minecraft:bedrock", BelowTop: -1},
				},
			},
			// 2. Per-biome underwater surface (water gate). For
			//    beaches use sand; oceans use gravel / sand.
			{
				Cond: Water{SeaLevel: 63},
				Blocks: []Block{
					{Name: "minecraft:sand", BelowTop: 0},
					{Name: "minecraft:sand", BelowTop: 1},
					{Name: "minecraft:sand", BelowTop: 2},
					{Name: "minecraft:sand", BelowTop: 3},
				},
			},
			// 3. Snowy biomes: snow on top, snow_block on the
			//    surface cell, then dirt / stone below.
			{
				Cond: BiomeHasSnow{},
				Blocks: []Block{
					{Name: "minecraft:snow", BelowTop: 0}, // top: snow layer
					{Name: "minecraft:snow_block", BelowTop: -1},
					{Name: "minecraft:dirt", BelowTop: 0},
					{Name: "minecraft:dirt", BelowTop: 1},
					{Name: "minecraft:dirt", BelowTop: 2},
					{Name: "minecraft:dirt", BelowTop: 3},
				},
			},
		},
		// Default rule (implicit, filled per-biome by the pipeline).
		DefaultTop:    "minecraft:grass_block",
		DefaultFiller: "minecraft:dirt",
	}
}

// EvaluateForBiome returns the Block list for a column with the
// given biome. The pipeline calls this once per column after it
// knows the column's biome; DefaultTop / DefaultFiller are filled
// from the biome's Surface row.
func EvaluateForBiome(p *Program, x, z, surfaceY int, b biome.Parameters) []Block {
	if p == nil {
		p = NewOverworldProgram()
	}
	// The implicit last rule is the biome's default surface.
	top := b.Surface.Top
	filler := b.Surface.Filler
	// Water override: replace top with the underwater material.
	if surfaceY < 63 {
		if b.Surface.Underwater != "" {
			top = b.Surface.Underwater
			if filler == "" {
				filler = b.Surface.Underwater
			}
		}
	}
	p.DefaultTop = top
	p.DefaultFiller = filler
	return p.Eval(x, z, surfaceY, b)
}
