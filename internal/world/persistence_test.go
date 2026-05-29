package world

import (
	"path/filepath"
	"testing"
)

func TestChunkEncodeDecodeRoundTrip(t *testing.T) {
	c := NewChunk()
	stone := StateID("minecraft:stone")
	bedrock := StateID("minecraft:bedrock")
	c.SetBlock(3, 5, 9, BlockByID(stone))
	c.SetBlock(0, 0, 0, BlockByID(bedrock))
	c.SetBlock(15, 200, 15, BlockByID(stone))

	got, err := DecodeChunk(c.Encode())
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if id := got.GetBlock(3, 5, 9).ID(); id != stone {
		t.Errorf("block (3,5,9) = %d, want %d", id, stone)
	}
	if id := got.GetBlock(0, 0, 0).ID(); id != bedrock {
		t.Errorf("block (0,0,0) = %d, want %d", id, bedrock)
	}
	if id := got.GetBlock(15, 200, 15).ID(); id != stone {
		t.Errorf("block (15,200,15) = %d, want %d", id, stone)
	}
	if id := got.GetBlock(1, 1, 1).ID(); id != AirID {
		t.Errorf("untouched block = %d, want air", id)
	}
}

func TestDiskStorageRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "world")
	store, err := NewDiskStorage(dir)
	if err != nil {
		t.Fatalf("new storage: %v", err)
	}
	defer store.Close()

	c := NewChunk()
	stone := StateID("minecraft:stone")
	c.SetBlock(7, 70, 7, BlockByID(stone))

	if err := store.SaveChunk(2, -3, c); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, ok, err := store.LoadChunk(2, -3)
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	if id := loaded.GetBlock(7, 70, 7).ID(); id != stone {
		t.Errorf("loaded block = %d, want %d", id, stone)
	}

	if _, ok, _ := store.LoadChunk(99, 99); ok {
		t.Errorf("expected missing chunk to report ok=false")
	}
}

func TestWorldPersistsEditsAcrossReload(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "world")

	store1, _ := NewDiskStorage(dir)
	w1 := NewWorld("world")
	w1.SetStorage(store1)
	stone := StateID("minecraft:stone")
	w1.SetBlock(20, 65, 20, BlockByID(stone))
	if err := w1.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Fresh world + storage simulates a restart.
	store2, _ := NewDiskStorage(dir)
	w2 := NewWorld("world")
	w2.SetStorage(store2)
	if id := w2.GetBlock(20, 65, 20).ID(); id != stone {
		t.Fatalf("edit did not persist across reload: got %d want %d", id, stone)
	}
}

func TestSaveOnlyWritesDirtyChunks(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "world")
	store, _ := NewDiskStorage(dir)
	w := NewWorld("world")
	w.SetStorage(store)

	// LoadChunk without edits should not be dirty.
	c := w.LoadChunk(0, 0)
	if c.Dirty() {
		t.Errorf("freshly loaded chunk should not be dirty")
	}
	w.SetBlock(1, 64, 1, BlockByName("minecraft:stone"))
	if !w.GetChunk(0, 0).Dirty() {
		t.Errorf("edited chunk should be dirty")
	}
	if err := w.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	if w.GetChunk(0, 0).Dirty() {
		t.Errorf("chunk should be clean after save")
	}
}
