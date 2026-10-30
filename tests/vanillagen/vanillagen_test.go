package vanillagentests

import (
	"testing"

	bedrockworld "livingworld/internal/bedrock/world"
	"livingworld/internal/dimension"
	"livingworld/internal/dimension/overworld/vanillagen"
	lwworld "livingworld/internal/world"
)

func countNonAir(c *lwworld.Chunk) int {
	n := 0
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			for y := lwworld.MinWorldHeight; y < lwworld.MinWorldHeight+lwworld.SectionsPerChunk*16; y++ {
				if c.GetBlock(x, y, z).ID() != lwworld.AirID {
					n++
				}
			}
		}
	}
	return n
}

// TestAdapterGeneratesNonTrivialTerrain pins that the shipped overworld
// generator (vanilla-gen via the dimension seam) produces real terrain:
// bedrock floor, stone body, surface variation — not air or a flat stub.
func TestAdapterGeneratesNonTrivialTerrain(t *testing.T) {
	gen := dimension.NewOverworld().Generator(12345)
	c := gen.Generate(0, 0)
	if n := countNonAir(c); n < 10000 {
		t.Fatalf("chunk (0,0) has only %d non-air blocks; terrain looks empty", n)
	}
	bedrock := lwworld.StateID("minecraft:bedrock")
	foundFloor := false
	for x := 0; x < 16 && !foundFloor; x++ {
		for z := 0; z < 16; z++ {
			if c.GetBlock(x, lwworld.MinWorldHeight, z).ID() == bedrock {
				foundFloor = true
				break
			}
		}
	}
	if !foundFloor {
		t.Fatal("no bedrock at the world floor")
	}
	// Heightmap must agree with the actual column contents.
	for _, col := range [][2]int{{0, 0}, {8, 8}, {15, 15}} {
		x, z := col[0], col[1]
		want := int32(lwworld.MinWorldHeight - 1)
		for y := lwworld.MinWorldHeight + lwworld.SectionsPerChunk*16 - 1; y >= lwworld.MinWorldHeight; y-- {
			if c.GetBlock(x, y, z).ID() != lwworld.AirID {
				want = int32(y)
				break
			}
		}
		if got := c.GetHeightmap(x, z); got != want {
			t.Fatalf("heightmap at (%d,%d) = %d, want %d", x, z, got, want)
		}
	}
}

// TestAdapterDeterministic pins seed determinism: the same generator must
// reproduce a chunk block-for-block (the internal world's persistence and
// cross-edition consistency assume this).
func TestAdapterDeterministic(t *testing.T) {
	g1 := vanillagen.New(999)
	g2 := vanillagen.New(999)
	a, b := g1.Generate(3, -7), g2.Generate(3, -7)
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			for y := lwworld.MinWorldHeight; y < lwworld.MinWorldHeight+lwworld.SectionsPerChunk*16; y++ {
				if a.GetBlock(x, y, z).ID() != b.GetBlock(x, y, z).ID() {
					t.Fatalf("chunk (3,-7) differs at (%d,%d,%d) between two seed-999 generators", x, y, z)
				}
			}
		}
	}
	other := vanillagen.New(1000).Generate(3, -7)
	if countNonAir(other) == countNonAir(a) {
		// Extremely unlikely unless the seed is ignored; check one column too.
		same := true
		for y := lwworld.MinWorldHeight; y < lwworld.MinWorldHeight+lwworld.SectionsPerChunk*16; y++ {
			if a.GetBlock(0, y, 0).ID() != other.GetBlock(0, y, 0).ID() {
				same = false
				break
			}
		}
		if same {
			t.Fatal("different seeds produced identical terrain; seed ignored?")
		}
	}
}

// TestAdapterBlocksResolveToBedrock is the palette-coverage gate for the
// new generator: every non-air canonical block the adapter can emit in a
// sample of chunks must resolve back to a Bedrock runtime ID, or Bedrock
// clients would see air holes. It also asserts nothing was dropped to air
// during generation itself.
func TestAdapterBlocksResolveToBedrock(t *testing.T) {
	g := vanillagen.New(12345)
	seen := map[int32]bool{}
	for _, pos := range [][2]int{{0, 0}, {4, -4}, {-3, 9}, {12, 30}} {
		c := g.Generate(pos[0], pos[1])
		for x := 0; x < 16; x++ {
			for z := 0; z < 16; z++ {
				for y := lwworld.MinWorldHeight; y < lwworld.MinWorldHeight+lwworld.SectionsPerChunk*16; y++ {
					id := c.GetBlock(x, y, z).ID()
					if id != lwworld.AirID {
						seen[id] = true
					}
				}
			}
		}
	}
	if len(seen) < 20 {
		t.Fatalf("only %d distinct canonical blocks across sampled chunks; suspiciously poor palette", len(seen))
	}
	for id := range seen {
		if _, ok := bedrockworld.LivingWorldBlockIDToBedrockRID(id); !ok {
			t.Errorf("generated block %s (state %d) does not resolve to a Bedrock RID", lwworld.StateName(id), id)
		}
	}
	if _, dropped := g.Stats(); dropped != 0 {
		t.Errorf("adapter dropped %d distinct Bedrock states to air (no canonical Java state)", dropped)
	}
}

// TestAdapterNetherAndEnd ensures the nether/end adapters (which replaced
// the stub generators at the dimension seam) also produce terrain.
func TestAdapterNetherAndEnd(t *testing.T) {
	if n := countNonAir(dimension.NewNether().Generator(42).Generate(0, 0)); n == 0 {
		t.Fatal("nether chunk (0,0) is empty")
	}
	if n := countNonAir(dimension.NewEnd().Generator(42).Generate(0, 0)); n == 0 {
		t.Fatal("end chunk (0,0) is empty (main island expected near origin)")
	}
}

// TestChunkFormatVersionRejectsV2 pins the v3 bump: a payload stamped with
// the old terrain source's version must be rejected so LoadChunk
// regenerates instead of serving stale pipeline terrain.
func TestChunkFormatVersionRejectsV2(t *testing.T) {
	c := vanillagen.New(1).Generate(0, 0)
	data := c.Encode()
	if data[0] != 3 {
		t.Fatalf("encoded chunk version = %d, want 3", data[0])
	}
	stale := append([]byte{}, data...)
	stale[0] = 2
	if _, err := lwworld.DecodeChunk(stale); err == nil {
		t.Fatal("v2 chunk decoded successfully; stale terrain would be served")
	}
	if _, err := lwworld.DecodeChunk(data); err != nil {
		t.Fatalf("v3 chunk failed to decode: %v", err)
	}
}
