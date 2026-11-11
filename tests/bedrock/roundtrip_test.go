package bedrocktests

import (
	"testing"

	bedrockskin "livingworld/internal/bedrock/skin"
	bedrockworld "livingworld/internal/bedrock/world"
	"livingworld/internal/dimension/overworld"
	"livingworld/internal/player"
	lwworld "livingworld/internal/world"

	dfworld "github.com/df-mc/dragonfly/server/world"
	dfbiome "github.com/df-mc/dragonfly/server/world/biome"
	dfchunk "github.com/df-mc/dragonfly/server/world/chunk"
	"github.com/google/uuid"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
)

// TestSubChunkEncodeDecodeRoundTrip builds a dragonfly chunk the exact way
// SendChunk does — from the real overworld generator through
// LivingWorldBlockIDToBedrockRID — then round-trips every non-empty sub-chunk
// through EncodeSubChunk -> NetworkDecode and asserts every block survives.
// This catches encoding regressions independently of the client.
func TestSubChunkEncodeDecodeRoundTrip(t *testing.T) {
	dfworld.DefaultBlockRegistry.Finalize()
	rng := dfworld.Overworld.Range()

	gen := overworld.New(12345)
	w := lwworld.NewWorld("roundtrip-test")
	w.SetGenerator(gen)
	wchunk := w.LoadChunk(4, -4)

	ch := dfchunk.New(dfworld.DefaultBlockRegistry, rng)
	set := 0
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			for y := int(rng.Min()); y <= int(rng.Max()); y++ {
				id := wchunk.GetBlock(x, y, z).ID()
				if id == 0 {
					continue
				}
				rid, found := bedrockworld.LivingWorldBlockIDToBedrockRID(id)
				if !found {
					t.Fatalf("worldgen block %s (state %d) unresolved in chunk build", lwworld.StateName(id), id)
				}
				ch.SetBlock(uint8(x), int16(y), uint8(z), 0, rid)
				set++
			}
		}
	}
	if set == 0 {
		t.Fatal("generated chunk had no non-air blocks; generator regression?")
	}
	for y := int16(rng.Min()); y <= int16(rng.Max()); y += 4 {
		for x := uint8(0); x < 16; x += 4 {
			for z := uint8(0); z < 16; z += 4 {
				ch.SetBiome(x, y, z, uint32(dfbiome.Plains{}.EncodeBiome()))
			}
		}
	}

	// Encode all sub-chunks plus biomes once (the full stream decodable by
	// NetworkDecode), then verify each sub-chunk independently by slicing.
	biomes := dfchunk.EncodeBiomes(ch, dfchunk.NetworkEncoding)
	mismatches := 0
	for ind := 0; ind < int(rng.Height()>>4)+1; ind++ {
		if ch.Sub()[ind].Empty() {
			continue
		}
		payload := dfchunk.EncodeSubChunk(ch, dfchunk.NetworkEncoding, ind)
		full := append(append([]byte{}, payload...), biomes...)
		decoded, err := dfchunk.NetworkDecode(dfworld.DefaultBlockRegistry, full, 1, rng)
		if err != nil {
			t.Fatalf("sub-chunk %d: NetworkDecode failed: %v", ind, err)
		}
		baseY := int16(ind*16) + int16(rng.Min())
		for x := 0; x < 16; x++ {
			for z := 0; z < 16; z++ {
				for dy := 0; dy < 16; dy++ {
					y := baseY + int16(dy)
					a := ch.Block(uint8(x), y, uint8(z), 0)
					b := decoded.Block(uint8(x), y, uint8(z), 0)
					if a != b {
						if mismatches < 5 {
							t.Logf("sub-chunk %d mismatch at (%d,%d,%d): sent %d decoded %d", ind, x, y, z, a, b)
						}
						mismatches++
					}
				}
			}
		}
	}
	if mismatches > 0 {
		t.Fatalf("encode/decode round trip lost %d blocks", mismatches)
	}
}

// TestSubChunkHeightMapClassification pins the dragonfly-equivalent heightmap
// classifier used in SubChunk responses: a surface-touching sub-chunk carries
// HeightMapDataHasData with a real map; sub-chunks fully below/above the
// surface carry TooLow/TooHigh. Sending None everywhere (the old behavior)
// forces the client to approximate and can render misplaced terrain bands.
func TestSubChunkHeightMapClassification(t *testing.T) {
	dfworld.DefaultBlockRegistry.Finalize()
	rng := dfworld.Overworld.Range()

	ch := dfchunk.New(dfworld.DefaultBlockRegistry, rng)
	// Solid slab at y=64..66 → surface sub-chunk index (64>>4)+... base -64 →
	// index (64 - (-64)) / 16 = 8.
	for x := uint8(0); x < 16; x++ {
		for z := uint8(0); z < 16; z++ {
			for y := int16(64); y <= 66; y++ {
				rid, _ := bedrockworld.LivingWorldBlockIDToBedrockRID(lwworld.StateID("minecraft:stone"))
				ch.SetBlock(x, y, z, 0, rid)
			}
		}
	}
	surfaceInd := int16((64 - int(rng.Min())) / 16)

	hmType, hmData, hasData := subChunkHeightMapForTest(ch, surfaceInd)
	if hmType != dfchunkSubChunkHasData || !hasData {
		t.Fatalf("surface sub-chunk %d: got type %d, want HeightMapDataHasData", surfaceInd, hmType)
	}
	if _, set := hmData.Value(); !set {
		t.Fatal("surface sub-chunk heightmap data missing")
	}

	// Fully-below sub-chunk (bedrock floor at y=-64) → TooLow.
	for x := uint8(0); x < 16; x++ {
		for z := uint8(0); z < 16; z++ {
			rid, _ := bedrockworld.LivingWorldBlockIDToBedrockRID(lwworld.StateID("minecraft:bedrock"))
			ch.SetBlock(x, int16(rng.Min()), z, 0, rid)
		}
	}
	// Dragonfly semantics (session/chunk.go subChunkEntry): higher=true means
	// every column's surface sub-chunk is ABOVE this one → the sub-chunk lies
	// fully below terrain → HeightMapDataTooHigh. lower=true (surface below)
	// → the sub-chunk lies fully above terrain → HeightMapDataTooLow.
	lowType, _, _ := subChunkHeightMapForTest(ch, 0)
	if lowType != dfchunkSubChunkTooHigh {
		t.Fatalf("below-surface sub-chunk: got type %d, want HeightMapDataTooHigh", lowType)
	}
	topInd := int16((rng.Height() >> 4))
	aboveType, _, _ := subChunkHeightMapForTest(ch, topInd)
	if aboveType != dfchunkSubChunkTooLow {
		t.Fatalf("above-surface sub-chunk: got type %d, want HeightMapDataTooLow", aboveType)
	}
}

// TestSubChunkHeightMapOddYAndSubChunkBoundary verifies that odd local Y values
// (especially local Y=15 at the top of a sub-chunk, such as Y=63 in SubChunk 7)
// return Y+1 (64, in SubChunk 8) rather than falling victim to dragonfly
// v0.11.5's `int16(y) | chunk.SubY(index) + 1` operator-precedence bug.
func TestSubChunkHeightMapOddYAndSubChunkBoundary(t *testing.T) {
	stoneRID, ok := bedrockworld.LivingWorldBlockIDToBedrockRID(lwworld.StateID("minecraft:stone"))
	if !ok {
		t.Fatalf("stone not resolvable")
	}

	rng := dfworld.Overworld.Range()
	ch := dfchunk.New(dfworld.DefaultBlockRegistry, rng)

	// Fill every column up to Y=63 (top block of SubChunk 7: Y=48..63, local Y=15).
	for z := uint8(0); z < 16; z++ {
		for x := uint8(0); x < 16; x++ {
			ch.SetBlock(x, 63, z, 0, stoneRID)
		}
	}

	// Because all columns have highest blocker at Y=63, the heightmap Y (first
	// air above highest blocker) is Y=64, which is at local Y=0 of SubChunk 8!
	// Therefore SubChunk 7 is completely below the heightmap (HeightMapDataTooHigh),
	// and SubChunk 8 has local heightmap 0 across all columns.
	sc7Type, _, sc7HasData := subChunkHeightMapForTest(ch, 7)
	if sc7Type != dfchunkSubChunkTooHigh || sc7HasData {
		t.Fatalf("SubChunk 7 (all columns top block at Y=63): got type=%d hasData=%v, want HeightMapDataTooHigh", sc7Type, sc7HasData)
	}

	sc8Type, sc8Opt, sc8HasData := subChunkHeightMapForTest(ch, 8)
	if sc8Type != dfchunkSubChunkHasData || !sc8HasData {
		t.Fatalf("SubChunk 8 (heightmap at Y=64, local 0): got type=%d hasData=%v, want HeightMapDataHasData", sc8Type, sc8HasData)
	}
	sc8Map, _ := sc8Opt.Value()
	if sc8Map[0][0] != 0 {
		t.Fatalf("SubChunk 8 subMap[0][0]: got %d, want 0", sc8Map[0][0])
	}
}

// TestSubChunkHeightMapIncludesTreeLeavesCanopy ensures light-diffusing blocks
// such as oak_leaves (FilteringBlock=1) are included in the Bedrock sub-chunk
// heightmap so tree canopies in SubChunk 8 above ground do not get sent with
// subMap=-1 ("pure air"), which previously caused Bedrock's client-side
// SubChunkLighter to render trees and the ground/player under them pitch black.
func TestSubChunkHeightMapIncludesTreeLeavesCanopy(t *testing.T) {
	grassRID, ok := bedrockworld.LivingWorldBlockIDToBedrockRID(lwworld.StateID("minecraft:grass_block"))
	if !ok {
		t.Fatalf("grass_block not resolvable")
	}
	logRID, ok := bedrockworld.LivingWorldBlockIDToBedrockRID(lwworld.StateID("minecraft:oak_log"))
	if !ok {
		t.Fatalf("oak_log not resolvable")
	}
	leavesRID, ok := bedrockworld.LivingWorldBlockIDToBedrockRID(lwworld.StateID("minecraft:oak_leaves"))
	if !ok {
		t.Fatalf("oak_leaves not resolvable")
	}

	rng := dfworld.Overworld.Range()
	ch := dfchunk.New(dfworld.DefaultBlockRegistry, rng)

	// Flat ground at Y=63 across the chunk.
	for z := uint8(0); z < 16; z++ {
		for x := uint8(0); x < 16; x++ {
			ch.SetBlock(x, 63, z, 0, grassRID)
		}
	}

	// Place a tree at (8, 8): oak_log from Y=64..68, oak_leaves canopy at Y=67..70
	// around it (including (7, 8) which has ONLY leaves above grass at Y=63).
	for y := int16(64); y <= 68; y++ {
		ch.SetBlock(8, y, 8, 0, logRID)
	}
	ch.SetBlock(8, 69, 8, 0, leavesRID)
	ch.SetBlock(7, 67, 8, 0, leavesRID)
	ch.SetBlock(7, 68, 8, 0, leavesRID)

	// SubChunk 8 covers Y=64..79.
	// - Column (8, 8) has highest blocker at Y=69 (oak_leaves) -> heightmap Y=70 -> local 6
	// - Column (7, 8) has highest blocker at Y=68 (oak_leaves) -> heightmap Y=69 -> local 5
	// - Column (0, 0) has highest blocker at Y=63 (grass_block) -> heightmap Y=64 -> local 0
	sc8Type, sc8Opt, sc8HasData := subChunkHeightMapForTest(ch, 8)
	if sc8Type != dfchunkSubChunkHasData || !sc8HasData {
		t.Fatalf("SubChunk 8: got type=%d hasData=%v, want HeightMapDataHasData", sc8Type, sc8HasData)
	}
	sc8Map, _ := sc8Opt.Value()
	if got := sc8Map[8][8]; got != 6 {
		t.Fatalf("tree trunk+top-leaf column (8,8) local heightmap: got %d, want 6 (Y=70)", got)
	}
	if got := sc8Map[8][7]; got != 5 {
		t.Fatalf("tree leaf-only canopy column (7,8) local heightmap: got %d, want 5 (Y=69)", got)
	}
	if got := sc8Map[0][0]; got != 0 {
		t.Fatalf("open ground column (0,0) at Y=63 local heightmap in SubChunk 8: got %d, want 0 (Y=64)", got)
	}
}

// TestRIDCacheConcurrency hammers the resolution cache from many goroutines;
// must be race-clean under -race (CI).
func TestRIDCacheConcurrency(t *testing.T) {
	done := make(chan struct{})
	for i := 0; i < 8; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for _, n := range worldgenBlockNames {
				sid := lwworld.StateID(n)
				bedrockworld.LivingWorldBlockIDToBedrockRID(sid)
			}
		}()
	}
	for i := 0; i < 8; i++ {
		<-done
	}
}

// TestJavaFallbackSkinUpgradesLegacy64x32 verifies that legacy 64x32 Java skins
// are upgraded to 64x64 RGBA buffers matching defaultSkinGeometry's 64x64 UV layout,
// and slim skins use the slim geometry resource patch.
func TestJavaFallbackSkinUpgradesLegacy64x32(t *testing.T) {
	legacy := make([]byte, 64*32*4)
	legacy[0] = 0xAA
	snap := player.PlayerSnapshot{
		UUID:     uuid.New(),
		Username: "JavaTester",
		Skin: &player.SkinData{
			Data:  legacy,
			Model: "slim",
		},
	}
	ps := bedrockskin.JavaFallbackSkinForViewer(snap)
	if ps.SkinImageWidth != 64 || ps.SkinImageHeight != 64 {
		t.Fatalf("skin dimensions = %dx%d, want 64x64", ps.SkinImageWidth, ps.SkinImageHeight)
	}
	if len(ps.SkinData) != 64*64*4 {
		t.Fatalf("len(SkinData) = %d, want %d", len(ps.SkinData), 64*64*4)
	}
	if ps.ArmSize != protocol.ArmSizeSlim {
		t.Fatalf("ArmSize = %v, want slim", ps.ArmSize)
	}
	if ps.CapeID == "" {
		t.Fatal("CapeID must not be empty")
	}
}

func TestTallGrassUpperLowerDistinctBedrockRIDs(t *testing.T) {
	lowerID := lwworld.StateIDWithProperties("minecraft:tall_grass", map[string]string{"half": "lower"})
	upperID := lwworld.StateIDWithProperties("minecraft:tall_grass", map[string]string{"half": "upper"})
	if lowerID == upperID {
		t.Fatalf("Java StateID for tall_grass lower and upper are identical: %d", lowerID)
	}
	lowerRID, ok1 := bedrockworld.LivingWorldBlockIDToBedrockRID(lowerID)
	upperRID, ok2 := bedrockworld.LivingWorldBlockIDToBedrockRID(upperID)
	if !ok1 || !ok2 {
		t.Fatalf("failed to resolve tall_grass RIDs: lower=%v upper=%v", ok1, ok2)
	}
	if lowerRID == upperRID {
		t.Fatalf("Bedrock RID for tall_grass[half=lower] (%d) and tall_grass[half=upper] (%d) must be distinct", lowerRID, upperRID)
	}
}
