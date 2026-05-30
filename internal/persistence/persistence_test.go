package persistence

import (
	"reflect"
	"testing"
)

func sampleSection(fill int32) []int32 {
	s := make([]int32, SectionBlocks)
	for i := range s {
		s[i] = fill
	}
	return s
}

func TestChunkEncodeDecodeRoundTrip(t *testing.T) {
	cd := &ChunkData{CX: 2, CZ: -3, Sections: make([][]int32, 24)}
	cd.Sections[0] = sampleSection(0)
	cd.Sections[0][5] = 42
	cd.Sections[4] = sampleSection(7)

	got, err := DecodeChunk(cd.Encode())
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.CX != 2 || got.CZ != -3 {
		t.Errorf("coords = (%d,%d), want (2,-3)", got.CX, got.CZ)
	}
	if got.Sections[0][5] != 42 {
		t.Errorf("section0[5] = %d, want 42", got.Sections[0][5])
	}
	if got.Sections[4][0] != 7 {
		t.Errorf("section4[0] = %d, want 7", got.Sections[4][0])
	}
	if got.Sections[1] != nil {
		t.Errorf("empty section should decode to nil, got len %d", len(got.Sections[1]))
	}
}

func TestDecodeChunkRejectsBadData(t *testing.T) {
	if _, err := DecodeChunk([]byte{1, 2}); err == nil {
		t.Error("expected error for short data")
	}
	bad := (&ChunkData{}).Encode()
	bad[0] = 99 // bogus version
	if _, err := DecodeChunk(bad); err == nil {
		t.Error("expected error for unsupported version")
	}
}

func TestStoreChunkRoundTrip(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer store.Close()

	cd := &ChunkData{CX: 10, CZ: -7, Sections: make([][]int32, 24)}
	cd.Sections[3] = sampleSection(99)
	if err := store.SaveChunk(cd); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, ok, err := store.LoadChunk(10, -7)
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	if got.Sections[3][0] != 99 {
		t.Errorf("loaded section3[0] = %d, want 99", got.Sections[3][0])
	}
	if _, ok, _ := store.LoadChunk(123, 456); ok {
		t.Errorf("missing chunk should report ok=false")
	}
}

func TestPlayerRoundTrip(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer store.Close()

	p := &PlayerData{
		UUID: "abc-123", Name: "Steve",
		X: 1.5, Y: 64, Z: -20.25,
		Yaw: 90, Pitch: -10,
		Health: 18.5, Food: 17, HeldSlot: 2,
		Inventory: []ItemStack{
			{ID: "minecraft:stone", Count: 64},
			{ID: "minecraft:diamond_sword", Count: 1, Meta: 3},
		},
	}
	if err := store.SavePlayer(p); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, ok, err := store.LoadPlayer("abc-123")
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	if !reflect.DeepEqual(p, got) {
		t.Errorf("round-trip mismatch:\n got %+v\nwant %+v", got, p)
	}
	if _, ok, _ := store.LoadPlayer("nobody"); ok {
		t.Errorf("missing player should report ok=false")
	}
	if err := store.SavePlayer(&PlayerData{}); err == nil {
		t.Error("expected error saving player with empty UUID")
	}
}

func TestLevelRoundTrip(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer store.Close()

	if _, ok, _ := store.LoadLevel(); ok {
		t.Errorf("unsaved level should report ok=false")
	}
	l := &LevelData{Name: "world", Seed: 123456789, SpawnY: 64, Time: 6000, Generator: "vanilla"}
	if err := store.SaveLevel(l); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, ok, err := store.LoadLevel()
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	if !reflect.DeepEqual(l, got) {
		t.Errorf("level mismatch:\n got %+v\nwant %+v", got, l)
	}
}
