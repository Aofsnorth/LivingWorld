package pipeline

import (
	"livingworld/internal/dimension/overworld/biome"
	"livingworld/internal/dimension/overworld/noise"
	"livingworld/internal/world"
)

// treeKind enumerates the tree shapes the decorator can build.
type treeKind int

const (
	treeOak treeKind = iota
	treeBirch
	treeSpruce
	treeJungle
	treeAcacia
	treeCherry
	treeDarkOak
)

// vegProfile is the per-biome decoration recipe.
type vegProfile struct {
	trees      []treeKind // candidate kinds, picked uniformly
	treeCount  int        // tree attempts per chunk
	treeChance float64    // probability the chunk gets trees at all (1 = always)
	grass      int        // short_grass attempts
	ferns      int        // fern attempts (taiga family)
	flowers    int        // dandelion/poppy attempts
	cactus     int        // cactus attempts (desert)
	deadBush   int        // dead bush attempts
}

func vegFor(id biome.ID) vegProfile {
	switch id {
	case "minecraft:forest", "minecraft:windswept_forest":
		return vegProfile{trees: []treeKind{treeOak, treeOak, treeOak, treeBirch}, treeCount: 6, treeChance: 1, grass: 8, flowers: 2}
	case "minecraft:flower_forest":
		return vegProfile{trees: []treeKind{treeOak, treeBirch}, treeCount: 3, treeChance: 1, grass: 8, flowers: 24}
	case "minecraft:birch_forest", "minecraft:old_growth_birch_forest":
		return vegProfile{trees: []treeKind{treeBirch}, treeCount: 6, treeChance: 1, grass: 8, flowers: 2}
	case "minecraft:dark_forest":
		return vegProfile{trees: []treeKind{treeDarkOak, treeDarkOak, treeOak}, treeCount: 8, treeChance: 1, grass: 6}
	case "minecraft:taiga", "minecraft:old_growth_pine_taiga", "minecraft:old_growth_spruce_taiga":
		return vegProfile{trees: []treeKind{treeSpruce}, treeCount: 7, treeChance: 1, grass: 4, ferns: 8}
	case "minecraft:snowy_taiga":
		return vegProfile{trees: []treeKind{treeSpruce}, treeCount: 4, treeChance: 1, ferns: 3}
	case "minecraft:grove":
		return vegProfile{trees: []treeKind{treeSpruce}, treeCount: 3, treeChance: 1}
	case "minecraft:snowy_plains":
		return vegProfile{trees: []treeKind{treeSpruce}, treeCount: 1, treeChance: 0.15, grass: 2}
	case "minecraft:jungle", "minecraft:bamboo_jungle":
		return vegProfile{trees: []treeKind{treeJungle, treeJungle, treeOak}, treeCount: 8, treeChance: 1, grass: 14, flowers: 1}
	case "minecraft:sparse_jungle":
		return vegProfile{trees: []treeKind{treeJungle, treeOak}, treeCount: 2, treeChance: 1, grass: 12}
	case "minecraft:savanna", "minecraft:savanna_plateau", "minecraft:windswept_savanna":
		return vegProfile{trees: []treeKind{treeAcacia}, treeCount: 1, treeChance: 0.8, grass: 22}
	case "minecraft:plains", "minecraft:sunflower_plains":
		return vegProfile{trees: []treeKind{treeOak}, treeCount: 1, treeChance: 0.25, grass: 18, flowers: 4}
	case "minecraft:meadow":
		return vegProfile{trees: []treeKind{treeOak, treeBirch}, treeCount: 1, treeChance: 0.1, grass: 20, flowers: 8}
	case "minecraft:cherry_grove":
		return vegProfile{trees: []treeKind{treeCherry}, treeCount: 3, treeChance: 1, grass: 12, flowers: 4}
	case "minecraft:swamp":
		return vegProfile{trees: []treeKind{treeOak}, treeCount: 2, treeChance: 1, grass: 10}
	case "minecraft:wooded_badlands":
		return vegProfile{trees: []treeKind{treeOak}, treeCount: 2, treeChance: 1, grass: 4, deadBush: 2}
	case "minecraft:desert":
		return vegProfile{cactus: 3, deadBush: 3}
	case "minecraft:badlands", "minecraft:eroded_badlands":
		return vegProfile{deadBush: 4}
	case "minecraft:windswept_hills", "minecraft:windswept_gravelly_hills":
		return vegProfile{trees: []treeKind{treeSpruce, treeOak}, treeCount: 1, treeChance: 0.3, grass: 6}
	default:
		return vegProfile{}
	}
}

// decorate places vegetation for the chunk. Trees anchored in any of
// the 3×3 neighbouring chunks are evaluated too, and only the blocks
// that land inside THIS chunk are written — that is what lets a canopy
// straddle a chunk border without the generator ever touching a
// neighbouring chunk.
func (g *Generator) decorate(c *world.Chunk, cols *[16][16]column, cx, cz int) {
	for dcz := -1; dcz <= 1; dcz++ {
		for dcx := -1; dcx <= 1; dcx++ {
			g.placeTreesFrom(c, cx, cz, cx+dcx, cz+dcz)
		}
	}
	g.placePlants(c, cols, cx, cz)
	g.topLayer(c, cols, cx, cz)
}

// treePlan is one tree's deterministic placement, derived purely from
// the anchor chunk's coordinates + the world seed.
type treePlan struct {
	wx, wz int
	kind   treeKind
	r0     int64 // per-tree random stream seed
}

// treePlansFor rolls the tree anchors for chunk (acx, acz).
func (g *Generator) treePlansFor(acx, acz int) []treePlan {
	r := noise.NewWorldgenRandom(g.seed)
	r.SetPopulationSeed(uint64(g.seed), int32(acx*16), int32(acz*16))
	popSeed := r.NextLong()
	r.SetFeatureSeed(popSeed, 0, 9 /* vegetal_decoration step */)

	// The biome at the chunk centre decides the recipe — close enough to
	// vanilla's per-column check and keeps the plan cheap.
	centre := g.shapeColumn(acx*16+8, acz*16+8)
	veg := vegFor(centre.biome.ID)
	if len(veg.trees) == 0 || veg.treeCount == 0 {
		return nil
	}
	if veg.treeChance < 1 && r.NextDouble() > veg.treeChance {
		return nil
	}
	plans := make([]treePlan, 0, veg.treeCount)
	for i := 0; i < veg.treeCount; i++ {
		plans = append(plans, treePlan{
			wx:   acx*16 + int(r.NextIntBounded(16)),
			wz:   acz*16 + int(r.NextIntBounded(16)),
			kind: veg.trees[int(r.NextIntBounded(int32(len(veg.trees))))],
			r0:   r.NextLong(),
		})
	}
	return plans
}

// placeTreesFrom evaluates the tree plan of anchor chunk (acx, acz) and
// writes the blocks that fall inside target chunk (cx, cz).
func (g *Generator) placeTreesFrom(c *world.Chunk, cx, cz, acx, acz int) {
	for _, p := range g.treePlansFor(acx, acz) {
		// Ground at the anchor: recomputed (pure function), then checked
		// against the actual chunk if the anchor is local — caves may
		// have eaten the surface.
		ground := g.shapeColumn(p.wx, p.wz)
		gy := ground.height
		if gy < seaLevel || gy > 250 {
			continue // underwater or absurd
		}
		if p.wx>>4 == cx && p.wz>>4 == cz {
			top := c.GetBlock(p.wx&15, gy, p.wz&15).ID()
			if top != g.ids.grassBlock && top != g.ids.dirt && top != g.ids.podzol && top != g.ids.mud {
				continue // surface was carved or isn't soil
			}
		}
		tr := noise.NewWorldgenRandom(p.r0)
		g.buildTree(c, cx, cz, p.wx, gy+1, p.wz, p.kind, tr)
	}
}

// set places a block by world coordinates iff it falls inside chunk
// (cx, cz). onlyAir limits writes to empty cells (leaves must not eat
// trunks or terrain).
func (g *Generator) set(c *world.Chunk, cx, cz, wx, wy, wz int, id int32, onlyAir bool) {
	if wx>>4 != cx || wz>>4 != cz || wy < minY || wy > maxY {
		return
	}
	if onlyAir && c.GetBlock(wx&15, wy, wz&15).ID() != world.AirID {
		return
	}
	c.SetBlock(wx&15, wy, wz&15, world.BlockByID(id))
}

// buildTree writes one tree with its base (trunk bottom) at (wx, y, wz).
func (g *Generator) buildTree(c *world.Chunk, cx, cz, wx, y, wz int, kind treeKind, r *noise.WorldgenRandom) {
	switch kind {
	case treeSpruce:
		h := 7 + int(r.NextIntBounded(4))
		for i := 0; i < h; i++ {
			g.set(c, cx, cz, wx, y+i, wz, g.ids.spruceLog, false)
		}
		// Conical leaf layers from the top down.
		rad := 0
		for ly := y + h; ly >= y+2; ly-- {
			d := y + h - ly
			rad = (d+1)/2 + 1
			if rad > 3 {
				rad = 3
			}
			if d == 0 {
				rad = 0
			}
			for dx := -rad; dx <= rad; dx++ {
				for dz := -rad; dz <= rad; dz++ {
					if dx*dx+dz*dz > rad*rad+1 {
						continue
					}
					g.set(c, cx, cz, wx+dx, ly, wz+dz, g.ids.spruceLeaves, true)
				}
			}
			if d > 0 && d%2 == 0 {
				rad--
			}
		}
		g.set(c, cx, cz, wx, y+h, wz, g.ids.spruceLeaves, true)
	case treeAcacia:
		h := 5 + int(r.NextIntBounded(2))
		for i := 0; i < h; i++ {
			g.set(c, cx, cz, wx, y+i, wz, g.ids.acaciaLog, false)
		}
		for dx := -3; dx <= 3; dx++ {
			for dz := -3; dz <= 3; dz++ {
				if dx*dx+dz*dz <= 10 {
					g.set(c, cx, cz, wx+dx, y+h-1, wz+dz, g.ids.acaciaLeaves, true)
				}
				if dx*dx+dz*dz <= 3 {
					g.set(c, cx, cz, wx+dx, y+h, wz+dz, g.ids.acaciaLeaves, true)
				}
			}
		}
	case treeJungle:
		h := 8 + int(r.NextIntBounded(5))
		g.blobTree(c, cx, cz, wx, y, wz, h, 2, g.ids.jungleLog, g.ids.jungleLeaves)
	case treeBirch:
		h := 5 + int(r.NextIntBounded(3))
		g.blobTree(c, cx, cz, wx, y, wz, h, 2, g.ids.birchLog, g.ids.birchLeaves)
	case treeCherry:
		h := 4 + int(r.NextIntBounded(2))
		g.blobTree(c, cx, cz, wx, y, wz, h, 3, g.ids.cherryLog, g.ids.cherryLeaves)
	case treeDarkOak:
		h := 6 + int(r.NextIntBounded(2))
		g.blobTree(c, cx, cz, wx, y, wz, h, 3, g.ids.oakLog, g.ids.oakLeaves)
	default: // oak
		h := 4 + int(r.NextIntBounded(3))
		g.blobTree(c, cx, cz, wx, y, wz, h, 2, g.ids.oakLog, g.ids.oakLeaves)
	}
}

// blobTree is the classic oak shape: straight trunk, two wide leaf
// layers under the top, narrow cap above.
func (g *Generator) blobTree(c *world.Chunk, cx, cz, wx, y, wz, h, rad int, logID, leafID int32) {
	for i := 0; i < h; i++ {
		g.set(c, cx, cz, wx, y+i, wz, logID, false)
	}
	for ly := y + h - 3; ly <= y+h-1; ly++ {
		rr := rad
		if ly == y+h-1 {
			rr = rad - 1
		}
		for dx := -rr; dx <= rr; dx++ {
			for dz := -rr; dz <= rr; dz++ {
				if dx*dx+dz*dz > rr*rr+1 {
					continue
				}
				g.set(c, cx, cz, wx+dx, ly, wz+dz, leafID, true)
			}
		}
	}
	g.set(c, cx, cz, wx, y+h, wz, leafID, true)
	g.set(c, cx, cz, wx+1, y+h, wz, leafID, true)
	g.set(c, cx, cz, wx-1, y+h, wz, leafID, true)
	g.set(c, cx, cz, wx, y+h, wz+1, leafID, true)
	g.set(c, cx, cz, wx, y+h, wz-1, leafID, true)
}

// placePlants scatters grass / ferns / flowers / cactus / dead bushes —
// strictly chunk-local one-block features.
func (g *Generator) placePlants(c *world.Chunk, cols *[16][16]column, cx, cz int) {
	r := noise.NewWorldgenRandom(g.seed)
	r.SetPopulationSeed(uint64(g.seed), int32(cx*16), int32(cz*16))
	popSeed := r.NextLong()
	r.SetFeatureSeed(popSeed, 1, 9)

	place := func(attempts int, want int32, block int32) {
		for i := 0; i < attempts; i++ {
			lx, lz := int(r.NextIntBounded(16)), int(r.NextIntBounded(16))
			top := g.surfaceYIn(c, lx, lz, cols[lz][lx].height)
			if top < seaLevel-1 || top >= maxY {
				continue
			}
			if c.GetBlock(lx, top, lz).ID() != want {
				continue
			}
			if c.GetBlock(lx, top+1, lz).ID() != world.AirID {
				continue
			}
			c.SetBlock(lx, top+1, lz, world.BlockByID(block))
		}
	}

	// One profile per chunk keeps the RNG stream simple; mixed-biome
	// chunks just borrow the centre column's recipe.
	veg := vegFor(cols[8][8].biome.ID)
	if veg.grass > 0 {
		place(veg.grass, g.ids.grassBlock, g.ids.shortGrass)
	}
	if veg.ferns > 0 {
		place(veg.ferns, g.ids.grassBlock, g.ids.fern)
		place(veg.ferns, g.ids.podzol, g.ids.fern)
	}
	if veg.flowers > 0 {
		place(veg.flowers, g.ids.grassBlock, g.ids.dandelion)
		place(veg.flowers/2+1, g.ids.grassBlock, g.ids.poppy)
	}
	if veg.deadBush > 0 {
		place(veg.deadBush, g.ids.sand, g.ids.deadBush)
		place(veg.deadBush, g.ids.redSand, g.ids.deadBush)
	}
	if veg.cactus > 0 {
		for i := 0; i < veg.cactus; i++ {
			lx, lz := int(r.NextIntBounded(16)), int(r.NextIntBounded(16))
			top := g.surfaceYIn(c, lx, lz, cols[lz][lx].height)
			if c.GetBlock(lx, top, lz).ID() != g.ids.sand {
				continue
			}
			h := 1 + int(r.NextIntBounded(3))
			for d := 1; d <= h; d++ {
				if c.GetBlock(lx, top+d, lz).ID() != world.AirID {
					break
				}
				c.SetBlock(lx, top+d, lz, world.BlockByID(g.ids.cactus))
			}
		}
	}
}

// surfaceYIn finds the actual top solid block of column (lx, lz),
// starting the scan a little above the noise height so trees/caves are
// accounted for.
func (g *Generator) surfaceYIn(c *world.Chunk, lx, lz, noiseTop int) int {
	for y := clampI(noiseTop, minY, maxY); y > minY; y-- {
		if c.GetBlock(lx, y, lz).ID() != world.AirID {
			return y
		}
	}
	return minY
}

// topLayer is the snow pass: in snowy biomes a snow layer goes on top
// of every exposed solid block (including tree leaves).
func (g *Generator) topLayer(c *world.Chunk, cols *[16][16]column, cx, cz int) {
	for lz := 0; lz < 16; lz++ {
		for lx := 0; lx < 16; lx++ {
			col := cols[lz][lx]
			snowy := col.biome.HasSnow || (col.height > 130 && col.temp < 0.3) || col.height > 160
			if !snowy {
				continue
			}
			scanFrom := clampI(col.height+16, minY+1, maxY)
			y := scanFrom
			for ; y > minY; y-- {
				if c.GetBlock(lx, y, lz).ID() != world.AirID {
					break
				}
			}
			top := c.GetBlock(lx, y, lz).ID()
			if top == g.ids.water || top == g.ids.ice || top == g.ids.snowLayer || y >= maxY {
				continue
			}
			// Snow can't rest on plants.
			if top == g.ids.shortGrass || top == g.ids.fern || top == g.ids.dandelion ||
				top == g.ids.poppy || top == g.ids.deadBush || top == g.ids.cactus {
				continue
			}
			c.SetBlock(lx, y+1, lz, world.BlockByID(g.ids.snowLayer))
		}
	}
}
