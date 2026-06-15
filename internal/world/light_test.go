package world

import "testing"

// Light-engine regression tests for the vanilla-faithful rewrite of
// computeSkyLight / computeBlockLight (see light.go).

func stone() Block { return BlockByName("minecraft:stone") }
func glow() Block  { return BlockByName("minecraft:glowstone") }

// newLitWorld returns a generator-less world whose chunks are created empty
// (all air). Blocks are placed via SetBlock; callers then drain ProcessUpdates.
func newLitWorld() *World { return NewWorld("lighttest") }

func relight(w *World, chunks ...[2]int) {
	// Iterate to a fixpoint: re-queue the chunks and process until no light
	// changes. Mirrors how queueNeighborRelight + the tick converge a seam.
	for i := 0; i < 6; i++ {
		for _, c := range chunks {
			w.Light().QueueUpdate(c[0], c[1])
		}
		if len(w.Light().ProcessUpdates()) == 0 {
			return
		}
	}
}

// TestSkyLightCanopyNoDepthFalloff is the headline regression for the "dark
// patches under trees" bug. The old column scan darkened sky light by 1 per
// block going DOWN below the heightmap, so under a roof the ground was far
// darker than the air just beneath the roof. Vanilla re-lights under-canopy
// cells horizontally, so at a fixed column every under-roof cell has the SAME
// sky light regardless of its depth below the roof.
func TestSkyLightCanopyNoDepthFalloff(t *testing.T) {
	w := newLitWorld()
	const floorY, roofY = 60, 70
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			w.SetBlock(x, floorY, z, stone())
			// Roof over the central 12x12; leave a 2-wide open border ring so
			// light enters from the sides at full column height.
			if x >= 2 && x <= 13 && z >= 2 && z <= 13 {
				w.SetBlock(x, roofY, z, stone())
			}
		}
	}
	w.Light().ProcessUpdates()

	// Open border column is fully sky-lit.
	if got := w.GetSkyLight(0, 65, 0); got != 15 {
		t.Fatalf("open border sky light = %d, want 15", got)
	}

	// Under the roof, the same column at different depths must be equal and > 0.
	top := w.GetSkyLight(8, roofY-1, 8)  // just below roof
	bot := w.GetSkyLight(8, floorY+1, 8) // just above floor
	if top == 0 {
		t.Fatalf("under-roof sky light is 0 (canopy gloom regression)")
	}
	if top != bot {
		t.Fatalf("under-roof sky light varies with depth: y=%d->%d, y=%d->%d (want equal)",
			roofY-1, top, floorY+1, bot)
	}
}

// TestSkyLightOpenColumn confirms an open column is 15 from top to just above
// the floor (straight-down propagation through transparent air loses nothing).
func TestSkyLightOpenColumn(t *testing.T) {
	w := newLitWorld()
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			w.SetBlock(x, 0, z, stone())
		}
	}
	w.Light().ProcessUpdates()
	for _, y := range []int{1, 50, 100, 200, 318} {
		if got := w.GetSkyLight(8, y, 8); got != 15 {
			t.Fatalf("open column sky light at y=%d = %d, want 15", y, got)
		}
	}
}

// TestBlockLightFalloff confirms block light loses exactly 1 per air block
// (max(1,opacity)), not the old opacity+1 = 2.
func TestBlockLightFalloff(t *testing.T) {
	w := newLitWorld()
	w.SetBlock(8, 64, 8, glow())
	w.Light().ProcessUpdates()

	checks := []struct {
		x, want int
	}{{8, 15}, {9, 14}, {10, 13}, {11, 12}}
	for _, c := range checks {
		if got := int(w.GetBlockLight(c.x, 64, 8)); got != c.want {
			t.Fatalf("block light at x=%d = %d, want %d", c.x, got, c.want)
		}
	}
}

// canopyGen builds a flat floor plus a roof spanning the chunk boundary at
// world-x 15/16, so under-roof light in either chunk depends on the other.
type canopyGen struct{}

func (canopyGen) Generate(cx, cz int) *Chunk {
	c := NewChunk()
	const floorY, roofY = 60, 70
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			wx := cx*16 + x
			c.SetBlock(x, floorY, z, stone())
			if wx >= 10 && wx <= 21 { // roof straddles the chunk-0/chunk-1 seam
				c.SetBlock(x, roofY, z, stone())
			}
		}
	}
	return c
}

// TestSkyLightSeamOrderIndependence loads two adjacent chunks under a shared
// roof in BOTH orders and asserts the converged sky light is identical and shows
// no artificial dark seam across the boundary.
func TestSkyLightSeamOrderIndependence(t *testing.T) {
	collect := func(firstA bool) map[[3]int]uint8 {
		w := newLitWorld()
		w.SetGenerator(canopyGen{})
		if firstA {
			w.LoadChunk(0, 0)
			w.LoadChunk(1, 0)
		} else {
			w.LoadChunk(1, 0)
			w.LoadChunk(0, 0)
		}
		relight(w, [2]int{0, 0}, [2]int{1, 0})

		out := make(map[[3]int]uint8)
		for _, wx := range []int{14, 15, 16, 17} { // cells straddling the seam
			for y := 61; y <= 69; y++ {
				for z := 0; z < 16; z++ {
					out[[3]int{wx, y, z}] = w.GetSkyLight(wx, y, z)
				}
			}
		}
		return out
	}

	ab := collect(true)
	ba := collect(false)

	for k, v := range ab {
		if ba[k] != v {
			t.Fatalf("seam light depends on load order at %v: AB=%d BA=%d", k, v, ba[k])
		}
	}
	// No artificial seam: adjacent air cells across the x=15/16 boundary differ
	// by at most 1 (the normal per-block falloff for transparent air).
	for y := 61; y <= 69; y++ {
		for z := 0; z < 16; z++ {
			l := int(ab[[3]int{15, y, z}])
			r := int(ab[[3]int{16, y, z}])
			if d := l - r; d > 1 || d < -1 {
				t.Fatalf("dark seam at boundary y=%d z=%d: x15=%d x16=%d", y, z, l, r)
			}
		}
	}
}
