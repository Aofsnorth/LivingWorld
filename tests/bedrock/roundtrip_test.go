package bedrocktests

import (
	"testing"

	bedrockworld "livingworld/internal/bedrock/world"
	generator "livingworld/internal/dimension/overworld/pipeline"
	lwworld "livingworld/internal/world"

	dfworld "github.com/df-mc/dragonfly/server/world"
	dfbiome "github.com/df-mc/dragonfly/server/world/biome"
	dfchunk "github.com/df-mc/dragonfly/server/world/chunk"
)

// TestSubChunkEncodeDecodeRoundTrip builds a dragonfly chunk the exact way
// SendChunk does — from the real overworld generator through
// LivingWorldBlockIDToBedrockRID — then round-trips every non-empty sub-chunk
// through EncodeSubChunk -> NetworkDecode and asserts every block survives.
// This catches encoding regressions independently of the client.
func TestSubChunkEncodeDecodeRoundTrip(t *testing.T) {
	dfworld.DefaultBlockRegistry.Finalize()
	rng := dfworld.Overworld.Range()

	gen := generator.NewGenerator(12345)
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
	if len(hmData) != 256 {
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
