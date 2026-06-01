// Package world tests: region save/load round-trip. This is the contract
// Phase 3 (persistence consolidation) hardens against; a test here means a
// future change that breaks the on-disk format or the LoadChunk path is
// caught in CI rather than in production.
package world

import (
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

// TestRegionStorageRoundTrip: a chunk written to disk via RegionStorage must
// be readable back with identical block state at the same (x, y, z).
func TestRegionStorageRoundTrip(t *testing.T) {
	dir := t.TempDir()
	rs, err := NewRegionStorage(dir)
	if err != nil {
		t.Fatalf("NewRegionStorage: %v", err)
	}
	defer rs.Close()

	// Place a few non-air blocks at varied (x, y, z) to exercise multiple
	// sections and both positive/negative canonical Y.
	src := NewChunk()
	placements := []struct{ x, y, z, state int }{
		{0, 0, 0, 1},
		{7, 64, 9, 2},
		{15, 255, 15, 3},
		{2, -32, 2, 4}, // negative Y
		{1, MinWorldHeight, 1, 5},
	}
	for _, p := range placements {
		src.SetBlock(p.x, p.y, p.z, BlockByID(int32(p.state)))
	}
	src.MarkDirty()

	if err := rs.SaveChunk(0, 0, src); err != nil {
		t.Fatalf("SaveChunk: %v", err)
	}
	if err := rs.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	// A fresh RegionStorage against the same dir must see the chunk on disk.
	rs2, err := NewRegionStorage(dir)
	if err != nil {
		t.Fatalf("reopen NewRegionStorage: %v", err)
	}
	defer rs2.Close()
	got, ok, err := rs2.LoadChunk(0, 0)
	if err != nil {
		t.Fatalf("LoadChunk: %v", err)
	}
	if !ok {
		t.Fatalf("LoadChunk: chunk not present on disk")
	}
	for _, p := range placements {
		if g := got.GetBlock(p.x, p.y, p.z).ID(); g != int32(p.state) {
			t.Errorf("round-trip GetBlock(%d,%d,%d)=%d, want %d", p.x, p.y, p.z, g, p.state)
		}
	}
}

// TestRegionFileMagic checks the on-disk format starts with the documented
// "LWR1" magic. Phase 3's contract: a bad magic is now quarantined
// automatically, and the next LoadChunk returns ok=false with no error so
// the world can regenerate the surface.
func TestRegionFileMagic(t *testing.T) {
	dir := t.TempDir()
	// Region 0,0 will be touched by LoadChunk(0,0) so create its file manually
	// with a wrong magic and a small gzip body (the gzip wrapper is required
	// because the reader gunzips before checking the magic).
	regionDir := filepath.Join(dir, "region")
	if err := os.MkdirAll(regionDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	zw.Write([]byte("XXXXnotmagic\x00\x00\x00\x00"))
	zw.Close()
	if err := os.WriteFile(filepath.Join(regionDir, "r.0.0.lwr"), buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write bogus region: %v", err)
	}

	rs, err := NewRegionStorage(dir)
	if err != nil {
		t.Fatalf("NewRegionStorage: %v", err)
	}
	defer rs.Close()
	// After Phase 3 quarantine: LoadChunk returns no error (the bad file was
	// quarantined) and ok=false (no chunk data, surface will regenerate).
	_, ok, err := rs.LoadChunk(0, 0)
	if err != nil {
		t.Fatalf("LoadChunk on bad-magic region: expected nil error (quarantined), got %v", err)
	}
	if ok {
		t.Fatalf("LoadChunk on bad-magic region: ok=true, want false")
	}
}

// TestDiskStorageRoundTrip: the simpler per-chunk backend also round-trips.
// This is the storage Phase 3 will fold in; pinning its behavior now makes
// the consolidation PR safe.
func TestDiskStorageRoundTrip(t *testing.T) {
	dir := t.TempDir()
	ds, err := NewDiskStorage(dir)
	if err != nil {
		t.Fatalf("NewDiskStorage: %v", err)
	}
	defer ds.Close()

	src := NewChunk()
	src.SetBlock(3, 12, 4, BlockByID(42))
	src.SetBlock(0, -64, 0, BlockByID(7))
	src.MarkDirty()

	if err := ds.SaveChunk(2, -1, src); err != nil {
		t.Fatalf("SaveChunk: %v", err)
	}
	got, ok, err := ds.LoadChunk(2, -1)
	if err != nil {
		t.Fatalf("LoadChunk: %v", err)
	}
	if !ok {
		t.Fatalf("LoadChunk: chunk not present")
	}
	if g := got.GetBlock(3, 12, 4).ID(); g != 42 {
		t.Errorf("GetBlock(3,12,4)=%d, want 42", g)
	}
	if g := got.GetBlock(0, -64, 0).ID(); g != 7 {
		t.Errorf("GetBlock(0,-64,0)=%d, want 7", g)
	}
}

// TestRegionQuarantineBadMagic: Phase 3's corrupt-region quarantine. A
// region file with a wrong magic must be moved to the `quarantine/` subdir
// on first LoadChunk, and the next LoadChunk call must return ok=false (no
// error) so the world regenerates the surface cleanly.
func TestRegionQuarantineBadMagic(t *testing.T) {
	dir := t.TempDir()
	regionDir := filepath.Join(dir, "region")
	if err := os.MkdirAll(regionDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Write a gzip-wrapped region file with a wrong magic to disk.
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	zw.Write([]byte("XXXXnotmagic\x00\x00\x00\x00"))
	zw.Close()
	if err := os.WriteFile(filepath.Join(regionDir, "r.0.0.lwr"), buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write bogus region: %v", err)
	}

	rs, err := NewRegionStorage(dir)
	if err != nil {
		t.Fatalf("NewRegionStorage: %v", err)
	}
	defer rs.Close()

	// First LoadChunk: should NOT error (the bad region was quarantined).
	// ok=false is correct: the surface was not generated, so the world
	// will regenerate.
	_, ok, err := rs.LoadChunk(0, 0)
	if err != nil {
		t.Errorf("LoadChunk after bad-magic region: expected nil error (quarantined), got %v", err)
	}
	if ok {
		t.Errorf("LoadChunk after bad-magic region: ok=true, want false (no chunk data)")
	}

	// The original file must have moved out of the way.
	if _, err := os.Stat(filepath.Join(regionDir, "r.0.0.lwr")); !os.IsNotExist(err) {
		t.Errorf("original region file still present after quarantine: %v", err)
	}
	// And there must be a `quarantine/` subdir with a *.bad file.
	qdir := filepath.Join(regionDir, "quarantine")
	entries, err := os.ReadDir(qdir)
	if err != nil {
		t.Fatalf("quarantine dir missing: %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("quarantine dir is empty (expected at least one .bad file)")
	}
}

// TestRegionQuarantineKeepsServerAlive: a single bad region must not take
// the world down. After the quarantine, a fresh SaveChunk on a different
// chunk in the same region must succeed.
func TestRegionQuarantineKeepsServerAlive(t *testing.T) {
	dir := t.TempDir()
	regionDir := filepath.Join(dir, "region")
	if err := os.MkdirAll(regionDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Bad region at (0,0) — covers chunks 0..1023.
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	zw.Write([]byte("XXXXnotmagic\x00\x00\x00\x00"))
	zw.Close()
	if err := os.WriteFile(filepath.Join(regionDir, "r.0.0.lwr"), buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	rs, err := NewRegionStorage(dir)
	if err != nil {
		t.Fatalf("NewRegionStorage: %v", err)
	}
	defer rs.Close()

	// Trigger the quarantine.
	if _, _, err := rs.LoadChunk(0, 0); err != nil {
		t.Fatalf("LoadChunk(0,0): %v", err)
	}

	// Now write a fresh chunk into the same region. This should NOT fail
	// just because the old file was bad.
	fresh := NewChunk()
	fresh.SetBlock(0, 0, 0, BlockByID(42))
	fresh.MarkDirty()
	if err := rs.SaveChunk(5, 5, fresh); err != nil {
		t.Fatalf("SaveChunk after quarantine: %v", err)
	}
	if err := rs.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	// Reading it back must succeed.
	got, ok, err := rs.LoadChunk(5, 5)
	if err != nil {
		t.Fatalf("LoadChunk(5,5): %v", err)
	}
	if !ok {
		t.Fatalf("LoadChunk(5,5): ok=false, want true")
	}
	if g := got.GetBlock(0, 0, 0).ID(); g != 42 {
		t.Errorf("GetBlock(0,0,0)=%d, want 42", g)
	}
}
