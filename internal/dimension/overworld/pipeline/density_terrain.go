package pipeline

import (
	"sync"
)

// Vanilla-driven terrain shaping. The overworld's final_density tree — parsed
// verbatim from vanilla 26.3's noise_settings (see the density package) — is
// sampled at the noise-cell corners of each chunk and trilinearly
// interpolated per block. A block is solid where the density is positive:
// the terrain shape (continents, mountain chains, river cuts) is therefore
// vanilla's own, not an approximation.
//
// Sampling follows vanilla's 4x8x4 noise-cell grid. The cell sizes come from
// noise_settings (size_horizontal=1, size_vertical=2 -> CellWidth=4,
// CellHeight=8); the grid is aligned so nested "interpolated" nodes collapse
// to a single corner (see Interpolated.Eval).
//
// Corner columns (x, z both multiples of 4) are the unit of work and are
// memoized in densColumns: a chunk sweep needs 25 of them, and the tree-plan
// pass — which re-derives ground heights for anchors in the 3x3 chunk ring —
// shares most of those columns with the sweep and with neighbouring chunks.

const (
	densCellW  = 4
	densCellH  = 8
	densLayers = (maxY+1-minY)/densCellH + 1 // y = -64..320 step 8 → 49
)

// densColumns memoizes final_density samples for whole (x, z) corner columns
// across chunks. Bounded: entries are dropped wholesale past the cap, which is
// safe because evaluation is pure.
type densColumns struct {
	mu sync.Mutex
	m  map[[2]int]*[densLayers]float64
}

// column returns the memoized density column at 4-aligned (x, z), evaluating
// it with a fresh cache when absent.
func (g *Generator) column(x, z int) *[densLayers]float64 {
	key := [2]int{x, z}
	g.cols.mu.Lock()
	if col, ok := g.cols.m[key]; ok {
		g.cols.mu.Unlock()
		return col
	}
	g.cols.mu.Unlock()

	var out [densLayers]float64
	dc := g.densTmpl.Fresh()
	for yi := 0; yi < densLayers; yi++ {
		out[yi] = g.dens.Eval(dc, x, minY+yi*densCellH, z)
	}

	g.cols.mu.Lock()
	if len(g.cols.m) > 4096 {
		g.cols.m = make(map[[2]int]*[densLayers]float64, 1024)
	}
	g.cols.m[key] = &out
	g.cols.mu.Unlock()
	return &out
}

// chunkDensity holds the density columns covering one chunk's noise grid.
type chunkDensity struct {
	cols [5][5]*[densLayers]float64
}

// buildChunkDensity gathers the chunk's corner columns (shared via the
// column cache).
func (g *Generator) buildChunkDensity(cx, cz int) *chunkDensity {
	var d chunkDensity
	x0, z0 := cx*16, cz*16
	for xi := 0; xi <= 4; xi++ {
		for zi := 0; zi <= 4; zi++ {
			d.cols[xi][zi] = g.column(x0+xi*densCellW, z0+zi*densCellW)
		}
	}
	return &d
}

// at returns the interpolated density at chunk-local block (lx, y, lz).
func (d *chunkDensity) at(lx, y, lz int) float64 {
	xi := lx / densCellW
	zi := lz / densCellW
	yi := (y - minY) / densCellH
	tx := float64(lx-xi*densCellW) / densCellW
	ty := float64(y-minY-yi*densCellH) / densCellH
	tz := float64(lz-zi*densCellW) / densCellW

	c00 := d.cols[xi][zi]
	c10 := d.cols[xi+1][zi]
	c01 := d.cols[xi][zi+1]
	c11 := d.cols[xi+1][zi+1]
	return trilerp(tx, ty, tz,
		c00[yi], c10[yi], c00[yi+1], c10[yi+1],
		c01[yi], c11[yi], c01[yi+1], c11[yi+1])
}

// scanColumn fills out with the per-block solidity of chunk-local column
// (lx, lz): out[i] reports whether the trilinearly interpolated final_density
// is positive at y = minY+i. Inside one 8-block cell the interpolated density
// is linear in y, so each cell costs two corner blends plus one lerp per
// block — the same values `at` produces, gathered in one pass.
func (d *chunkDensity) scanColumn(lx, lz int, out []bool) {
	xi := lx / densCellW
	zi := lz / densCellW
	tx := float64(lx-xi*densCellW) / densCellW
	tz := float64(lz-zi*densCellW) / densCellW

	c00 := d.cols[xi][zi]
	c10 := d.cols[xi+1][zi]
	c01 := d.cols[xi][zi+1]
	c11 := d.cols[xi+1][zi+1]

	wx0, wx1 := 1-tx, tx
	wz0, wz1 := 1-tz, tz
	for yi := 0; yi+1 < densLayers; yi++ {
		p := wz0*(wx0*c00[yi]+wx1*c10[yi]) + wz1*(wx0*c01[yi]+wx1*c11[yi])
		q := wz0*(wx0*c00[yi+1]+wx1*c10[yi+1]) + wz1*(wx0*c01[yi+1]+wx1*c11[yi+1])
		delta := (q - p) / densCellH
		base := yi * densCellH
		for k := 0; k < densCellH; k++ {
			out[base+k] = p > 0
			p += delta
		}
	}
}

// topAt returns the Y of the highest solid block of the column, or minY when
// the column is all air. It scans the same per-block solidity field
// buildColumn materialises, so the two can never disagree.
func (d *chunkDensity) topAt(lx, lz int) int {
	var solid [maxY + 1 - minY]bool
	d.scanColumn(lx, lz, solid[:])
	for y := maxY; y >= minY; y-- {
		if solid[y-minY] {
			return y
		}
	}
	return minY
}

// terrainTop returns the vanilla-density terrain top of one world column.
// Tree anchors re-derive it for every chunk in their 3×3 ring, so results
// are memoized in a bounded cache (evaluation is a pure function of the
// world seed and the column coordinates).
func (g *Generator) terrainTop(wx, wz int) int {
	key := [2]int{wx, wz}
	g.tops.mu.Lock()
	if v, ok := g.tops.m[key]; ok {
		g.tops.mu.Unlock()
		return v
	}
	g.tops.mu.Unlock()

	v := g.columnTop(wx, wz)

	g.tops.mu.Lock()
	if len(g.tops.m) > 4096 {
		g.tops.m = make(map[[2]int]int, 1024)
	}
	g.tops.m[key] = v
	g.tops.mu.Unlock()
	return v
}

// columnTop blends the four surrounding noise-cell corner columns the same
// way chunkDensity.at does, then scans for the topmost solid block.
func (g *Generator) columnTop(wx, wz int) int {
	x0 := wx >> 2 << 2
	z0 := wz >> 2 << 2
	tx := float64(wx-x0) / densCellW
	tz := float64(wz-z0) / densCellW

	c00 := g.column(x0, z0)
	c10 := g.column(x0+densCellW, z0)
	c01 := g.column(x0, z0+densCellW)
	c11 := g.column(x0+densCellW, z0+densCellW)

	for y := maxY; y >= minY; y-- {
		yi := (y - minY) / densCellH
		ty := float64(y-minY-yi*densCellH) / densCellH
		v := trilerp(tx, ty, tz,
			c00[yi], c10[yi], c00[yi+1], c10[yi+1],
			c01[yi], c11[yi], c01[yi+1], c11[yi+1])
		if v > 0 {
			return y
		}
	}
	return minY
}

// trilerp blends the eight cell corners at weights (tx, ty, tz). Corner
// order: bit0 = x, bit1 = y, bit2 = z (matching the density package's
// lerp3 so both interpolation paths agree).
func trilerp(tx, ty, tz, v000, v100, v010, v110, v001, v101, v011, v111 float64) float64 {
	lerp := func(t, a, b float64) float64 { return a + t*(b-a) }
	x00 := lerp(tx, v000, v100)
	x01 := lerp(tx, v001, v101)
	x10 := lerp(tx, v010, v110)
	x11 := lerp(tx, v011, v111)
	y0 := lerp(ty, x00, x10)
	y1 := lerp(ty, x01, x11)
	return lerp(tz, y0, y1)
}
