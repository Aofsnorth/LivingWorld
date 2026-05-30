package world

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRegionStorageRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "world")
	store, err := NewRegionStorage(dir)
	if err != nil {
		t.Fatalf("new region storage: %v", err)
	}
	stone := StateID("minecraft:stone")

	// Two chunks in the SAME region (so they share one file).
	c1 := NewChunk()
	c1.SetBlock(1, 64, 1, BlockByID(stone))
	c2 := NewChunk()
	c2.SetBlock(2, 70, 2, BlockByID(stone))
	if err := store.SaveChunk(0, 0, c1); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveChunk(5, 7, c2); err != nil {
		t.Fatal(err)
	}
	if err := store.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	// Exactly one region file should exist for chunks (0,0) and (5,7).
	files, _ := filepath.Glob(filepath.Join(dir, "region", "*.lwr"))
	if len(files) != 1 {
		t.Fatalf("expected 1 region file, got %d: %v", len(files), files)
	}

	// Reopen from scratch (simulates restart) and verify both chunks survive.
	store2, _ := NewRegionStorage(dir)
	got1, ok, err := store2.LoadChunk(0, 0)
	if err != nil || !ok {
		t.Fatalf("load (0,0): ok=%v err=%v", ok, err)
	}
	if got1.GetBlock(1, 64, 1).ID() != stone {
		t.Errorf("chunk (0,0) block lost")
	}
	got2, ok, _ := store2.LoadChunk(5, 7)
	if !ok || got2.GetBlock(2, 70, 2).ID() != stone {
		t.Errorf("chunk (5,7) block lost")
	}
	if _, ok, _ := store2.LoadChunk(31, 31); ok {
		t.Errorf("unsaved chunk should be absent")
	}
}

func TestRegionStorageRewritePreservesOtherChunks(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "world")
	store, _ := NewRegionStorage(dir)
	stone := StateID("minecraft:stone")
	dirt := StateID("minecraft:dirt")

	a := NewChunk()
	a.SetBlock(0, 64, 0, BlockByID(stone))
	b := NewChunk()
	b.SetBlock(0, 64, 0, BlockByID(dirt))
	_ = store.SaveChunk(1, 1, a)
	_ = store.SaveChunk(2, 2, b)
	_ = store.Flush()

	// Reopen, change only one chunk, flush again.
	store2, _ := NewRegionStorage(dir)
	b2, _, _ := store2.LoadChunk(2, 2)
	b2.SetBlock(1, 65, 1, BlockByID(stone))
	_ = store2.SaveChunk(2, 2, b2)
	_ = store2.Flush()

	// The untouched chunk (1,1) must still be intact.
	store3, _ := NewRegionStorage(dir)
	a3, ok, _ := store3.LoadChunk(1, 1)
	if !ok || a3.GetBlock(0, 64, 0).ID() != stone {
		t.Fatalf("untouched chunk (1,1) was lost after rewriting its region")
	}
}

func TestRegionFewFilesForManyChunks(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "world")
	store, _ := NewRegionStorage(dir)
	stone := StateID("minecraft:stone")
	// Write a 16×16 area of chunks (256 chunks) — all within region (0,0).
	for cx := 0; cx < 16; cx++ {
		for cz := 0; cz < 16; cz++ {
			c := NewChunk()
			c.SetBlock(0, 64, 0, BlockByID(stone))
			_ = store.SaveChunk(cx, cz, c)
		}
	}
	_ = store.Flush()
	entries, _ := os.ReadDir(filepath.Join(dir, "region"))
	if len(entries) != 1 {
		t.Fatalf("256 chunks should fit in 1 region file, got %d entries", len(entries))
	}
}
