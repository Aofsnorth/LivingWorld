// Package pipeline is the Overworld's per-chunk generator. It produces
// modern-vanilla-style (1.18+) terrain:
//
//	climate noise (temperature, humidity, continentalness, erosion,
//	weirdness) → terrain shaping splines (continents, ridges, jagged
//	peaks, rivers) → biome decision tree → surface rules (grass / sand
//	/ terracotta bands / snow, deepslate layer, bedrock floor) → cave
//	carving (cheese + spaghetti) → 1.18-style ore distribution →
//	decoration (trees, plants, snow layers).
//
// Everything is a pure function of (worldSeed, blockX, blockZ): the
// same seed always regenerates the identical world, and features that
// straddle chunk borders (tree canopies) are re-derived from their
// anchor chunk so they come out whole no matter which chunk generates
// first.
package pipeline

import (
	"livingworld/internal/world"
)

// blockIDs caches every canonical block-state ID the generator places.
// Resolving the names once at construction keeps the hot loops free of
// string lookups (and would surface a palette mismatch immediately:
// unknown names resolve to air).
type blockIDs struct {
	bedrock, stone, deepslate              int32
	dirt, grassBlock, podzol, mycelium     int32
	sand, sandstone, redSand, gravel       int32
	water, lava, ice, snowBlock, snowLayer int32
	mud                                    int32
	terracotta, orangeTerracotta           int32
	yellowTerracotta, whiteTerracotta      int32
	brownTerracotta                        int32
	granite, diorite, andesite, tuff       int32
	oakLog, oakLeaves                      int32
	birchLog, birchLeaves                  int32
	spruceLog, spruceLeaves                int32
	jungleLog, jungleLeaves                int32
	acaciaLog, acaciaLeaves                int32
	cherryLog, cherryLeaves                int32
	shortGrass, fern, dandelion, poppy     int32
	cactus, deadBush                       int32
}

func resolveBlockIDs() blockIDs {
	id := world.StateID
	return blockIDs{
		bedrock: id("minecraft:bedrock"), stone: id("minecraft:stone"), deepslate: id("minecraft:deepslate"),
		dirt: id("minecraft:dirt"), grassBlock: id("minecraft:grass_block"),
		podzol: id("minecraft:podzol"), mycelium: id("minecraft:mycelium"),
		sand: id("minecraft:sand"), sandstone: id("minecraft:sandstone"),
		redSand: id("minecraft:red_sand"), gravel: id("minecraft:gravel"),
		water: id("minecraft:water"), lava: id("minecraft:lava"), ice: id("minecraft:ice"),
		snowBlock: id("minecraft:snow_block"), snowLayer: id("minecraft:snow"),
		mud:        id("minecraft:mud"),
		terracotta: id("minecraft:terracotta"), orangeTerracotta: id("minecraft:orange_terracotta"),
		yellowTerracotta: id("minecraft:yellow_terracotta"), whiteTerracotta: id("minecraft:white_terracotta"),
		brownTerracotta: id("minecraft:brown_terracotta"),
		granite:         id("minecraft:granite"), diorite: id("minecraft:diorite"),
		andesite: id("minecraft:andesite"), tuff: id("minecraft:tuff"),
		oakLog: id("minecraft:oak_log"), oakLeaves: id("minecraft:oak_leaves"),
		birchLog: id("minecraft:birch_log"), birchLeaves: id("minecraft:birch_leaves"),
		spruceLog: id("minecraft:spruce_log"), spruceLeaves: id("minecraft:spruce_leaves"),
		jungleLog: id("minecraft:jungle_log"), jungleLeaves: id("minecraft:jungle_leaves"),
		acaciaLog: id("minecraft:acacia_log"), acaciaLeaves: id("minecraft:acacia_leaves"),
		cherryLog: id("minecraft:cherry_log"), cherryLeaves: id("minecraft:cherry_leaves"),
		shortGrass: id("minecraft:short_grass"), fern: id("minecraft:fern"),
		dandelion: id("minecraft:dandelion"), poppy: id("minecraft:poppy"),
		cactus: id("minecraft:cactus"), deadBush: id("minecraft:dead_bush"),
	}
}

// Generator is the world.ChunkGenerator implementation for the
// Overworld. Built once per world via NewGenerator; all fields are
// immutable after construction, so a single Generator is safe to use
// from concurrent chunk loads.
type Generator struct {
	seed int64
	ids  blockIDs
	ores []oreConfig

	// 2D climate / shaping fields.
	temperature *octaveNoise
	humidity    *octaveNoise
	continental *octaveNoise
	erosion     *octaveNoise
	weirdness   *octaveNoise
	detail      *octaveNoise
	jagged      *octaveNoise
	surfNoise   *octaveNoise

	// 3D cave fields.
	cheese     *octaveNoise
	spaghetti1 *octaveNoise
	spaghetti2 *octaveNoise

	// bandOffset shifts the badlands terracotta strata per world.
	bandOffset int64
}

// NewGenerator builds the generator for the given world seed.
func NewGenerator(seed int64) *Generator {
	g := &Generator{
		seed: seed,
		ids:  resolveBlockIDs(),

		temperature: newOctaveNoise(seed, 1001, 3, 1.0/650),
		humidity:    newOctaveNoise(seed, 1002, 3, 1.0/520),
		continental: newOctaveNoise(seed, 1003, 5, 1.0/900),
		erosion:     newOctaveNoise(seed, 1004, 4, 1.0/700),
		weirdness:   newOctaveNoise(seed, 1005, 4, 1.0/420),
		detail:      newOctaveNoise(seed, 1006, 3, 1.0/60),
		jagged:      newOctaveNoise(seed, 1007, 4, 1.0/130),
		surfNoise:   newOctaveNoise(seed, 1008, 2, 1.0/40),

		cheese:     newOctaveNoise(seed, 2001, 2, 1.0/110),
		spaghetti1: newOctaveNoise(seed, 2002, 1, 1.0/72),
		spaghetti2: newOctaveNoise(seed, 2003, 1, 1.0/72),

		bandOffset: (seed%7 + 7) % 7,
	}
	g.ores = g.buildOreConfigs()
	return g
}

// Generate builds chunk (cx, cz): terrain → surface → water → caves →
// ores → decoration. Returns a fully materialised *world.Chunk.
func (g *Generator) Generate(cx, cz int) *world.Chunk {
	c := world.NewChunk()

	// Pass 1: shape all 256 columns (height, biome, climate).
	var cols [16][16]column
	for lz := 0; lz < 16; lz++ {
		for lx := 0; lx < 16; lx++ {
			cols[lz][lx] = g.shapeColumn(cx*16+lx, cz*16+lz)
		}
	}

	// Pass 2: materialise the columns (stone body, surface, water).
	for lz := 0; lz < 16; lz++ {
		for lx := 0; lx < 16; lx++ {
			g.buildColumn(c, lx, lz, cx*16+lx, cz*16+lz, cols[lz][lx])
		}
	}

	// Pass 3: caves, then ores (veins show on cave walls), then trees /
	// plants / snow.
	g.carveCaves(c, &cols, cx, cz)
	g.placeOres(c, &cols, cx, cz)
	g.decorate(c, &cols, cx, cz)

	// Heightmap for consumers that read it before the light engine
	// recomputes it.
	for lz := 0; lz < 16; lz++ {
		for lx := 0; lx < 16; lx++ {
			c.SetHeightmap(lx, lz, int32(g.surfaceYIn(c, lx, lz, clampI(cols[lz][lx].height+24, minY, maxY))))
		}
	}
	return c
}
