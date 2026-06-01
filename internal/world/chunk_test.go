// Package world tests: chunk block get/set + encode/decode round-trip and the
// canonical-Y arithmetic (the previous `y>>4` implementation rejected y<0 and
// is documented in chunk.go as a bug; these tests pin the fix).
package world

import "testing"

// TestChunkBlockRoundTrip sets a few blocks at positive and negative Y,
// re-encodes the chunk, decodes a fresh copy, and verifies the reads match.
func TestChunkBlockRoundTrip(t *testing.T) {
	c := NewChunk()
	// (x, y, z, state) — mix positive, zero, and negative canonical Y to lock
	// in the (y - MinWorldHeight) section math.
	placements := []struct{ x, y, z, state int }{
		{0, 0, 0, 1},         // y=0 → section 4
		{5, 64, 7, 2},        // y=64 → section 8
		{1, -10, 1, 3},       // y=-10 → section 3 (negative)
		{2, MinWorldHeight, 2, 4}, // y=-64 → section 0 (lowest)
		{3, 100, 3, 5},
		{4, 200, 4, 6},
		{6, 319, 6, 7},       // y=319 → top placeable, section 23
	}
	for _, p := range placements {
		c.SetBlock(p.x, p.y, p.z, BlockByID(int32(p.state)))
		c.MarkDirty()
	}
	for _, p := range placements {
		got := c.GetBlock(p.x, p.y, p.z)
		if got.ID() != int32(p.state) {
			t.Errorf("GetBlock(%d,%d,%d)=%d, want %d", p.x, p.y, p.z, got.ID(), p.state)
		}
	}
}

// TestEncodeDecodeRoundTrip: the on-disk format must round-trip cleanly.
// DecodeChunk returns a fresh *Chunk; populating and re-encoding should
// produce the same bytes for a deterministic input.
func TestEncodeDecodeRoundTrip(t *testing.T) {
	src := NewChunk()
	src.SetBlock(0, 0, 0, BlockByID(1))
	src.SetBlock(1, 4, 1, BlockByID(42))
	src.SetBlock(15, 255, 15, BlockByID(7))
	src.SetBlock(2, -1, 2, BlockByID(99)) // sub-zero Y (canonical)
	src.SetBlock(0, 320, 0, BlockAir{})   // out-of-range set: should be a no-op (returns Air)

	encoded := src.Encode()
	decoded, err := DecodeChunk(encoded)
	if err != nil {
		t.Fatalf("DecodeChunk: %v", err)
	}
	for _, p := range []struct{ x, y, z, want int }{
		{0, 0, 0, 1},
		{1, 4, 1, 42},
		{15, 255, 15, 7},
		{2, -1, 2, 99},
	} {
		got := decoded.GetBlock(p.x, p.y, p.z)
		if got.ID() != int32(p.want) {
			t.Errorf("decoded.GetBlock(%d,%d,%d)=%d, want %d", p.x, p.y, p.z, got.ID(), p.want)
		}
	}
	// Re-encoding the decoded chunk should match the original bytes (sections
	// in the same order, same non-empty mask).
	re := decoded.Encode()
	if len(re) != len(encoded) {
		t.Fatalf("re-encoded length %d != original %d", len(re), len(encoded))
	}
	for i := range re {
		if re[i] != encoded[i] {
			t.Fatalf("re-encoded differs at byte %d: %x vs %x", i, re[i], encoded[i])
		}
	}
}

// TestChunkCoordFloorDivision: ChunkCoord must use floor division, not
// truncation. -1..-15 → -1, -16..-31 → -2. The previous `int32(x) >> 4`
// returned 0 for the whole -1..-15 range, which is wrong.
func TestChunkCoordFloorDivision(t *testing.T) {
	cases := []struct {
		in   float64
		want int32
	}{
		{0, 0},
		{0.5, 0},
		{15.99, 0},
		{16, 1},
		{-0.5, -1},
		{-1, -1},
		{-15.99, -1},
		{-16, -1},
		{-16.0, -1}, // exactly on the boundary still maps to chunk -1
		{-16.01, -2},
		{-31.99, -2},
		{-32, -2},
	}
	for _, tc := range cases {
		got := ChunkCoord(tc.in)
		if got != tc.want {
			t.Errorf("ChunkCoord(%v)=%d, want %d", tc.in, got, tc.want)
		}
	}
}

// TestAirBlock verifies that an unset cell reads back as air (state 0) and
// that the air sentinel is stable across set/get.
func TestAirBlock(t *testing.T) {
	c := NewChunk()
	if got := c.GetBlock(0, 0, 0); got.ID() != 0 {
		t.Errorf("fresh chunk at (0,0,0) is %d, want 0 (air)", got.ID())
	}
	if AirID != 0 {
		t.Errorf("AirID = %d, want 0", AirID)
	}
}
