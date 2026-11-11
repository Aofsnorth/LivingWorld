package vanillagentests

import (
	"context"
	"encoding/binary"
	"io"
	"log/slog"
	"testing"

	bedrockworld "livingworld/internal/bedrock/world"
	"livingworld/internal/dimension"
	"livingworld/internal/dimension/overworld"
	lwworld "livingworld/internal/world"

	dfworld "github.com/df-mc/dragonfly/server/world"
	dfbiome "github.com/df-mc/dragonfly/server/world/biome"
	dfchunk "github.com/df-mc/dragonfly/server/world/chunk"
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
// generator (SteelMC worldgen-lab via the dimension seam) produces real terrain:
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
	g1, err := overworld.Open(999, overworld.Options{
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		Workers: 2,
	})
	if err != nil {
		t.Fatalf("open g1: %v", err)
	}
	defer g1.Close()

	g2, err := overworld.Open(999, overworld.Options{
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		Workers: 2,
	})
	if err != nil {
		t.Fatalf("open g2: %v", err)
	}
	defer g2.Close()

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

	gOther, err := overworld.Open(1000, overworld.Options{
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		Workers: 2,
	})
	if err != nil {
		t.Fatalf("open gOther: %v", err)
	}
	defer gOther.Close()

	other := gOther.Generate(3, -7)
	if countNonAir(other) == countNonAir(a) {
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
// SteelMC worldgen-lab generator: every non-air canonical block the generator
// emits in a sample of chunks must resolve to a Bedrock runtime ID.
func TestAdapterBlocksResolveToBedrock(t *testing.T) {
	g := overworld.New(12345)
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
	if stats := g.Stats(); stats.UnmappedStates != 0 {
		t.Errorf("generator had %d unmapped Java states", stats.UnmappedStates)
	}
}

// TestAdapterNetherAndEnd ensures the nether/end generators at the dimension
// seam also produce terrain.
func TestAdapterNetherAndEnd(t *testing.T) {
	if n := countNonAir(dimension.NewNether().Generator(42).Generate(0, 0)); n == 0 {
		t.Fatal("nether chunk (0,0) is empty")
	}
	if n := countNonAir(dimension.NewEnd().Generator(42).Generate(0, 0)); n == 0 {
		t.Fatal("end chunk (0,0) is empty (main island expected near origin)")
	}
}

// TestChunkFormatVersionRejectsV2 pins the v5 bump: a payload stamped with
// an older terrain source's version must be rejected so LoadChunk
// regenerates instead of serving stale pipeline terrain, and 3D section biomes
// round-trip through Encode/DecodeChunk.
func TestChunkFormatVersionRejectsV2(t *testing.T) {
	c := overworld.New(12345).Generate(0, 0)
	if c.SectionBiomes(4) == nil {
		t.Fatal("expected overworld generator to populate section biomes")
	}
	data := c.Encode()
	if data[0] != 5 {
		t.Fatalf("encoded chunk version = %d, want 5", data[0])
	}
	for _, oldVer := range []byte{2, 3, 4} {
		stale := append([]byte{}, data...)
		stale[0] = oldVer
		if _, err := lwworld.DecodeChunk(stale); err == nil {
			t.Fatalf("v%d chunk decoded successfully; stale terrain would be served", oldVer)
		}
	}
	decoded, err := lwworld.DecodeChunk(data)
	if err != nil {
		t.Fatalf("v5 chunk failed to decode: %v", err)
	}
	if decoded.SectionBiomes(4) == nil || *decoded.SectionBiomes(4) != *c.SectionBiomes(4) {
		t.Fatal("section biomes did not round-trip through Encode/DecodeChunk")
	}
}

// TestDragonflyFillChunkConversion verifies SWG1 decoding into a Dragonfly
// chunk (exact parity with worldgen-lab's conversion_test.go).
func TestDragonflyFillChunkConversion(t *testing.T) {
	dfworld.DefaultBlockRegistry.Finalize()
	gen, err := overworld.Open(777, overworld.Options{
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		Workers: 1,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer gen.Close()

	// Build a synthetic SWG1 wire payload with 24 sections.
	data := make([]byte, 0, 260000)
	data = append(data, "SWG1"...)
	var hdr [4]byte
	binary.LittleEndian.PutUint16(hdr[0:2], 0xFFC0)
	binary.LittleEndian.PutUint16(hdr[2:4], 24)
	data = append(data, hdr[:]...)

	appendStr := func(s string) {
		var l [2]byte
		binary.LittleEndian.PutUint16(l[:], uint16(len(s)))
		data = append(data, l[:]...)
		data = append(data, s...)
	}

	for sec := 0; sec < 24; sec++ {
		if sec == 0 {
			var count [2]byte
			binary.LittleEndian.PutUint16(count[:], 2)
			data = append(data, count[:]...)
			appendStr("minecraft:air")
			appendStr("")
			data = append(data, 0)
			appendStr("minecraft:seagrass")
			appendStr("minecraft:seagrass")
			data = append(data, 2) // flagWaterlogged

			indices := make([]byte, 8192)
			binary.LittleEndian.PutUint16(indices[0:2], 1)
			data = append(data, indices...)
		} else {
			var count [2]byte
			binary.LittleEndian.PutUint16(count[:], 1)
			data = append(data, count[:]...)
			appendStr("minecraft:air")
			appendStr("")
			data = append(data, 0)
			data = append(data, make([]byte, 8192)...)
		}
		var bCount [2]byte
		binary.LittleEndian.PutUint16(bCount[:], 1)
		data = append(data, bCount[:]...)
		appendStr("minecraft:plains")
		data = append(data, make([]byte, 128)...)
	}

	ch := dfchunk.New(dfworld.DefaultBlockRegistry, dfworld.Overworld.Range())
	placed, err := gen.FillDragonflyChunk(ch, data)
	if err != nil {
		t.Fatalf("FillDragonflyChunk: %v", err)
	}
	if placed != 1 {
		t.Fatalf("placed = %d, want 1", placed)
	}
	waterRID, ok := overworld.LookupBedrockRID(0, "minecraft:water[level=0]")
	if !ok {
		t.Fatal("failed to resolve water RID")
	}
	if got := ch.Block(0, -64, 0, 1); got != waterRID {
		t.Fatalf("waterlogged layer 1 = %d, want %d", got, waterRID)
	}
	if got := ch.Biome(0, -64, 0); got != uint32(dfbiome.Plains{}.EncodeBiome()) {
		t.Fatalf("biome = %d, want plains", got)
	}
}

// TestDragonflyGenerateAndLifecycle verifies GenerateDragonfly and Close semantics.
func TestDragonflyGenerateAndLifecycle(t *testing.T) {
	dfworld.DefaultBlockRegistry.Finalize()
	gen, err := overworld.Open(12345, overworld.Options{
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		Workers: 2,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ch := dfchunk.New(dfworld.DefaultBlockRegistry, dfworld.Overworld.Range())
	if err := gen.GenerateDragonfly(context.Background(), dfworld.ChunkPos{0, 0}, ch); err != nil {
		t.Fatalf("GenerateDragonfly: %v", err)
	}
	stats := gen.Stats()
	if stats.Chunks == 0 || stats.Blocks == 0 {
		t.Fatalf("unexpected stats after generation: %+v", stats)
	}
	if err := gen.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := gen.GenerateDragonfly(context.Background(), dfworld.ChunkPos{0, 0}, ch); err != overworld.ErrClosed {
		t.Fatalf("after Close, GenerateDragonfly returned %v, want ErrClosed", err)
	}
}

// TestGrassBlockAndShortGrassLighting verifies that grass_block is opaque (15)
// and short_grass is transparent (0), and that surface columns with short_grass
// above grass_block get SkyLight=15 in the short_grass and SkyLight=0 in dirt below.
func TestGrassBlockAndShortGrassLighting(t *testing.T) {
	grassBlockID := lwworld.StateID("minecraft:grass_block")
	shortGrassID := lwworld.StateID("minecraft:short_grass")
	dirtID := lwworld.StateID("minecraft:dirt")

	if op := lwworld.GetLightProps(grassBlockID).Opacity; op != 15 {
		t.Fatalf("grass_block opacity = %d, want 15", op)
	}
	if op := lwworld.GetLightProps(shortGrassID).Opacity; op != 0 {
		t.Fatalf("short_grass opacity = %d, want 0", op)
	}

	w := lwworld.NewWorld("light-test")
	c := w.LoadChunk(0, 0)
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			c.SetBlock(x, 63, z, lwworld.BlockByID(dirtID))
			c.SetBlock(x, 64, z, lwworld.BlockByID(grassBlockID))
			c.SetBlock(x, 65, z, lwworld.BlockByID(shortGrassID))
		}
	}
	w.Light().ComputeChunkLight(c, 0, 0)

	if sky := c.GetSkyLight(8, 65, 8); sky != 15 {
		t.Fatalf("short_grass at y=65 sky light = %d, want 15", sky)
	}
	if sky := c.GetSkyLight(8, 64, 8); sky != 0 {
		t.Fatalf("grass_block at y=64 sky light = %d, want 0", sky)
	}
	if sky := c.GetSkyLight(8, 63, 8); sky != 0 {
		t.Fatalf("dirt at y=63 sky light = %d, want 0", sky)
	}
}
