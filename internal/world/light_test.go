// Package world tests: Phase 4b lighting engine — propagation correctness.
//
// These tests pin the spec contract (Master_Plan.md §6 Phase 4b DoD):
//   1. Sky light propagates from top down, blocked by opaque blocks.
//   2. Block light propagates from sources (torches, glowstone) with falloff.
//   3. Light respects block opacity (solid vs transparent).
//   4. Light spreads horizontally into caves and overhangs.
//
// Tests use manual world/chunk setup to isolate the light engine behavior
// from world generation and persistence.
package world

import (
	"testing"
)

// TestSkyLightOpenColumn: sky light should be 15 from top to bottom in an open column.
func TestSkyLightOpenColumn(t *testing.T) {
	w := NewWorld("test")
	cx, cz := 0, 0
	chunk := NewChunk()
	w.SetChunk(cx, cz, chunk)

	// All air column - should have full sky light from top to bottom
	for y := MinWorldHeight; y < MinWorldHeight+SectionsPerChunk*16; y++ {
		chunk.SetBlock(0, y, 0, BlockAir{})
	}

	// Compute light
	w.light.ComputeChunkLight(chunk, cx, cz)

	// Verify sky light is 15 throughout the column
	for y := MinWorldHeight; y < MinWorldHeight+SectionsPerChunk*16; y++ {
		got := chunk.GetSkyLight(0, y, 0)
		if got != 15 {
			t.Errorf("sky light at y=%d: got %d, want 15", y, got)
		}
	}
}

// TestSkyLightBlockedByOpaque: sky light should be blocked by a solid layer of opaque blocks.
func TestSkyLightBlockedByOpaque(t *testing.T) {
	w := NewWorld("test")
	cx, cz := 0, 0
	chunk := NewChunk()
	w.SetChunk(cx, cz, chunk)

	// Air above and below, solid stone layer at y=100 (all 16x16 blocks)
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			for y := MinWorldHeight; y < MinWorldHeight+SectionsPerChunk*16; y++ {
				if y == 100 {
					chunk.SetBlock(x, y, z, StateBlock{State: StateID("minecraft:stone")})
				} else {
					chunk.SetBlock(x, y, z, BlockAir{})
				}
			}
		}
	}

	// Compute light
	w.light.ComputeChunkLight(chunk, cx, cz)

	// Above stone should have full sky light
	if got := chunk.GetSkyLight(8, 101, 8); got != 15 {
		t.Errorf("sky light above stone layer: got %d, want 15", got)
	}

	// Below stone should have no sky light (blocked by solid layer)
	if got := chunk.GetSkyLight(8, 99, 8); got != 0 {
		t.Errorf("sky light below solid stone layer: got %d, want 0", got)
	}
}

// TestSkyLightThroughTransparent: sky light should pass through transparent blocks with falloff.
func TestSkyLightThroughTransparent(t *testing.T) {
	w := NewWorld("test")
	cx, cz := 0, 0
	chunk := NewChunk()
	w.SetChunk(cx, cz, chunk)

	// Air above and below, solid glass layer at y=100 (all 16x16 blocks)
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			for y := MinWorldHeight; y < MinWorldHeight+SectionsPerChunk*16; y++ {
				if y == 100 {
					chunk.SetBlock(x, y, z, StateBlock{State: StateID("minecraft:glass")})
				} else {
					chunk.SetBlock(x, y, z, BlockAir{})
				}
			}
		}
	}

	// Compute light
	w.light.ComputeChunkLight(chunk, cx, cz)

	// Above glass should have full sky light
	if got := chunk.GetSkyLight(8, 101, 8); got != 15 {
		t.Errorf("sky light above glass layer: got %d, want 15", got)
	}

	// At glass should have 15 - 1 (glass opacity) = 14
	if got := chunk.GetSkyLight(8, 100, 8); got != 14 {
		t.Errorf("sky light at glass layer: got %d, want 14", got)
	}

	// Below glass should have 14 - 1 (distance) = 13
	if got := chunk.GetSkyLight(8, 99, 8); got != 13 {
		t.Errorf("sky light below glass layer: got %d, want 13", got)
	}
}

// TestSkyLightHorizontalSpread: sky light should spread horizontally into caves.
func TestSkyLightHorizontalSpread(t *testing.T) {
	w := NewWorld("test")
	chunk := NewChunk()
	w.chunks[ChunkPos{0, 0}] = chunk
	
	// Create a fully enclosed cave with a small opening
	// Cave interior: x=4-11, y=50-60, z=4-11 (all air)
	// Walls: stone at boundaries
	// Ceiling: stone at y=61 except for a 2x2 opening at (7-8, 61, 7-8)
	// Floor: stone at y=49
	
	stoneID := StateID("minecraft:stone")
	
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			for y := MinWorldHeight; y <= MaxWorldHeight; y++ {
				// Default: air (sky above)
				var block Block = BlockAir{}
				
				// Ground below cave (y < 49)
				if y < 49 {
					block = StateBlock{State: stoneID}
				}
				
				// Cave interior (x=4-11, y=50-60, z=4-11)
				if x >= 4 && x <= 11 && y >= 50 && y <= 60 && z >= 4 && z <= 11 {
					block = BlockAir{}
				}
				
				// Cave walls
				if y >= 50 && y <= 60 {
					// Side walls
					if (x == 3 || x == 12) && z >= 3 && z <= 12 {
						block = StateBlock{State: stoneID}
					}
					if (z == 3 || z == 12) && x >= 3 && x <= 12 {
						block = StateBlock{State: stoneID}
					}
				}
				
				// Cave floor (y=49, x=3-12, z=3-12)
				if y == 49 && x >= 3 && x <= 12 && z >= 3 && z <= 12 {
					block = StateBlock{State: stoneID}
				}
				
				// Cave ceiling (y=61, x=3-12, z=3-12) with 2x2 opening
				if y == 61 && x >= 3 && x <= 12 && z >= 3 && z <= 12 {
					// Opening at (7-8, 61, 7-8)
					if (x == 7 || x == 8) && (z == 7 || z == 8) {
						block = BlockAir{} // Opening
					} else {
						block = StateBlock{State: stoneID} // Ceiling
					}
				}
				
				// Fill above cave with air (sky)
				if y > 61 {
					block = BlockAir{}
				}
				
				chunk.SetBlock(x, y, z, block)
			}
		}
	}
	
	le := NewLightEngine(w)
	le.ComputeChunkLight(chunk, 0, 0)
	
	// Light at opening should be full (15)
	lightAtOpening := chunk.GetSkyLight(7, 61, 7)
	if lightAtOpening != 15 {
		t.Errorf("light at opening: got %d, want 15", lightAtOpening)
	}
	
	// Light inside cave near opening should be high (14-15)
	lightNearOpening := chunk.GetSkyLight(7, 60, 7)
	if lightNearOpening < 14 {
		t.Errorf("light near opening: got %d, want >= 14", lightNearOpening)
	}
	
	// Light in far corner of cave should be reduced (horizontal spread with falloff)
	lightInCorner := chunk.GetSkyLight(4, 50, 4)
	if lightInCorner >= lightNearOpening {
		t.Errorf("light in corner (%d) should be < light near opening (%d)", 
			lightInCorner, lightNearOpening)
	}
	
	// But corner should still have some light (not 0)
	if lightInCorner == 0 {
		t.Errorf("light in corner should be > 0, got 0")
	}
	
	// Corner light should be around 8-12 (distance ~7 blocks from opening)
	if lightInCorner > 12 {
		t.Errorf("light in corner should be <= 12, got %d", lightInCorner)
	}
}

// TestBlockLightTorch: block light should propagate from a torch with falloff.
func TestBlockLightTorch(t *testing.T) {
	w := NewWorld("test")
	cx, cz := 0, 0
	chunk := NewChunk()
	w.SetChunk(cx, cz, chunk)

	// All air, place a torch at (8, 100, 8)
	for y := MinWorldHeight; y < MinWorldHeight+SectionsPerChunk*16; y++ {
		for x := 0; x < 16; x++ {
			for z := 0; z < 16; z++ {
				chunk.SetBlock(x, y, z, BlockAir{})
			}
		}
	}
	torchID := StateID("minecraft:torch")
	chunk.SetBlock(8, 100, 8, StateBlock{State: torchID})

	// Compute light
	w.light.ComputeChunkLight(chunk, cx, cz)

	// At torch should have emission value (torch=14)
	if got := chunk.GetBlockLight(8, 100, 8); got != 14 {
		t.Errorf("block light at torch: got %d, want 14", got)
	}

	// One block away should have 14-1=13 (air opacity=0, falloff=1)
	if got := chunk.GetBlockLight(9, 100, 8); got != 13 {
		t.Errorf("block light 1 block from torch: got %d, want 13", got)
	}

	// Two blocks away should have 13-1=12
	if got := chunk.GetBlockLight(10, 100, 8); got != 12 {
		t.Errorf("block light 2 blocks from torch: got %d, want 12", got)
	}

	// Far away (14 blocks) should have 14-14=0
	if got := chunk.GetBlockLight(0, 100, 0); got != 0 {
		t.Errorf("block light 14 blocks from torch: got %d, want 0", got)
	}
}

// TestBlockLightGlowstone: glowstone (emission=15) should light brighter than torch.
func TestBlockLightGlowstone(t *testing.T) {
	w := NewWorld("test")
	cx, cz := 0, 0
	chunk := NewChunk()
	w.SetChunk(cx, cz, chunk)

	// All air, place glowstone at (8, 100, 8)
	for y := MinWorldHeight; y < MinWorldHeight+SectionsPerChunk*16; y++ {
		for x := 0; x < 16; x++ {
			for z := 0; z < 16; z++ {
				chunk.SetBlock(x, y, z, BlockAir{})
			}
		}
	}
	glowstoneID := StateID("minecraft:glowstone")
	chunk.SetBlock(8, 100, 8, StateBlock{State: glowstoneID})

	// Compute light
	w.light.ComputeChunkLight(chunk, cx, cz)

	// At glowstone should have emission value (15)
	if got := chunk.GetBlockLight(8, 100, 8); got != 15 {
		t.Errorf("block light at glowstone: got %d, want 15", got)
	}

	// One block away should have 15-1=14
	if got := chunk.GetBlockLight(9, 100, 8); got != 14 {
		t.Errorf("block light 1 block from glowstone: got %d, want 14", got)
	}
}

// TestBlockLightBlockedByOpaque: block light should be blocked by opaque blocks.
func TestBlockLightBlockedByOpaque(t *testing.T) {
	w := NewWorld("test")
	cx, cz := 0, 0
	chunk := NewChunk()
	w.SetChunk(cx, cz, chunk)

	// All air, place torch at (5, 100, 8), solid stone wall at x=8 (all y and z)
	for x := 0; x < 16; x++ {
		for y := MinWorldHeight; y < MinWorldHeight+SectionsPerChunk*16; y++ {
			for z := 0; z < 16; z++ {
				if x == 8 {
					chunk.SetBlock(x, y, z, StateBlock{State: StateID("minecraft:stone")})
				} else {
					chunk.SetBlock(x, y, z, BlockAir{})
				}
			}
		}
	}
	torchID := StateID("minecraft:torch")
	chunk.SetBlock(5, 100, 8, StateBlock{State: torchID})

	// Compute light
	w.light.ComputeChunkLight(chunk, cx, cz)

	// Before stone wall (7, 100, 8) should have light: 14-2=12 (torch at 5, distance 2)
	if got := chunk.GetBlockLight(7, 100, 8); got != 12 {
		t.Errorf("block light before stone wall: got %d, want 12", got)
	}

	// After stone wall (9, 100, 8) should have 0 light (solid wall blocks all light)
	afterWallLight := chunk.GetBlockLight(9, 100, 8)
	if afterWallLight != 0 {
		t.Errorf("block light after solid stone wall: got %d, want 0", afterWallLight)
	}
}

// TestBlockLightThroughGlass: block light should pass through glass with reduced opacity.
func TestBlockLightThroughGlass(t *testing.T) {
	w := NewWorld("test")
	cx, cz := 0, 0
	chunk := NewChunk()
	w.SetChunk(cx, cz, chunk)

	// All air, place torch at (5, 100, 8), glass at (8, 100, 8)
	for x := 0; x < 16; x++ {
		for y := MinWorldHeight; y < MinWorldHeight+SectionsPerChunk*16; y++ {
			for z := 0; z < 16; z++ {
				if x == 8 {
					chunk.SetBlock(x, y, z, StateBlock{State: StateID("minecraft:glass")})
				} else {
					chunk.SetBlock(x, y, z, BlockAir{})
				}
			}
		}
	}
	torchID := StateID("minecraft:torch")
	chunk.SetBlock(5, 100, 8, StateBlock{State: torchID})

	// Compute light
	w.light.ComputeChunkLight(chunk, cx, cz)

	// Before glass (7, 100, 8) should have light: 14-2=12 (torch at 5, distance 2)
	if got := chunk.GetBlockLight(7, 100, 8); got != 12 {
		t.Errorf("block light before glass: got %d, want 12", got)
	}

	// At glass (8, 100, 8): 12 - 1 (distance) - 1 (glass opacity) = 10
	if got := chunk.GetBlockLight(8, 100, 8); got != 10 {
		t.Errorf("block light at glass: got %d, want 10", got)
	}

	// After glass (9, 100, 8): 10 - 1 (distance) - 0 (air opacity) = 9
	if got := chunk.GetBlockLight(9, 100, 8); got != 9 {
		t.Errorf("block light after glass: got %d, want 9", got)
	}
}

// TestMultipleLightSources: multiple torches should combine light correctly.
func TestMultipleLightSources(t *testing.T) {
	w := NewWorld("test")
	cx, cz := 0, 0
	chunk := NewChunk()
	w.SetChunk(cx, cz, chunk)

	// All air, place two torches at (5, 100, 8) and (11, 100, 8)
	for y := MinWorldHeight; y < MinWorldHeight+SectionsPerChunk*16; y++ {
		for x := 0; x < 16; x++ {
			for z := 0; z < 16; z++ {
				chunk.SetBlock(x, y, z, BlockAir{})
			}
		}
	}
	torchID := StateID("minecraft:torch")
	chunk.SetBlock(5, 100, 8, StateBlock{State: torchID})
	chunk.SetBlock(11, 100, 8, StateBlock{State: torchID})

	// Compute light
	w.light.ComputeChunkLight(chunk, cx, cz)

	// Midpoint (8, 100, 8) should receive light from both torches
	// From left torch: 14-3=11
	// From right torch: 14-3=11
	// Should take max, so 11
	midLight := chunk.GetBlockLight(8, 100, 8)
	if midLight != 11 {
		t.Errorf("block light at midpoint: got %d, want 11", midLight)
	}
}

// TestHeightmapComputation: heightmap should record the highest non-air block.
func TestHeightmapComputation(t *testing.T) {
	w := NewWorld("test")
	cx, cz := 0, 0
	chunk := NewChunk()
	w.SetChunk(cx, cz, chunk)

	// Air above y=100, stone at y=100, air below
	for y := MinWorldHeight; y < MinWorldHeight+SectionsPerChunk*16; y++ {
		if y <= 100 {
			chunk.SetBlock(0, y, 0, StateBlock{State: StateID("minecraft:stone")})
		} else {
			chunk.SetBlock(0, y, 0, BlockAir{})
		}
	}

	// Compute heightmap
	w.light.computeHeightmap(chunk, cx, cz)

	// Heightmap at (0, 0) should be 100
	got := chunk.GetHeightmap(0, 0)
	if got != 100 {
		t.Errorf("heightmap at (0,0): got %d, want 100", got)
	}
}

// TestLightUpdateQueueing: block changes should queue light updates.
func TestLightUpdateQueueing(t *testing.T) {
	w := NewWorld("test")
	cx, cz := 0, 0

	// Load a chunk (this computes initial light)
	chunk := w.LoadChunk(cx, cz)

	// Clear the pending queue (from initial load)
	w.light.ProcessUpdates()

	// Place a torch - should queue an update
	torchID := StateID("minecraft:torch")
	w.SetBlock(8, 100, 8, StateBlock{State: torchID})

	// Check that the chunk is queued
	w.light.mu.Lock()
	_, queued := w.light.pending[ChunkPos{cx, cz}]
	w.light.mu.Unlock()

	if !queued {
		t.Errorf("chunk not queued for light update after block change")
	}

	// Process updates
	w.light.ProcessUpdates()

	// Verify light is computed
	if got := chunk.GetBlockLight(8, 100, 8); got != 14 {
		t.Errorf("block light at torch after update: got %d, want 14", got)
	}
}
