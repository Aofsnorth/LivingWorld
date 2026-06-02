package world

import (
	"path/filepath"
	"testing"

	iworld "livingworld/internal/world"
	gen "livingworld/internal/world/generator"

	"github.com/Tnze/go-mc/level"
)

// skyLightSections counts sections that carry real sky-light data. A section
// with SkyLight == nil is sent to the client in the EMPTY sky-light mask, which
// the vanilla Java client renders as pitch black regardless of the time of day.
func skyLightSections(lc *level.Chunk) int {
	n := 0
	for i := range lc.Sections {
		if lc.Sections[i].SkyLight != nil {
			n++
		}
	}
	return n
}

// TestDiskLoadedSuperflatKeepsSkyLight reproduces the "Java world is pitch black
// at noon" symptom: a superflat chunk that is generated, autosaved, and then
// reloaded from disk (as happens on every server restart) must still carry sky
// light when converted to a LevelChunkWithLight packet. Persistence stores only
// blocks, so the world must recompute light when a chunk comes back from disk —
// otherwise the Java client (which trusts server light, unlike Bedrock which
// computes its own) sees sky light 0 everywhere and renders the surface black.
func TestDiskLoadedSuperflatKeepsSkyLight(t *testing.T) {
	dir := t.TempDir()

	// World #1: generate the spawn chunk (light is computed on generate) and
	// persist it the same way autosave does.
	store1, err := iworld.NewRegionStorage(filepath.Join(dir, "world"))
	if err != nil {
		t.Fatalf("NewRegionStorage: %v", err)
	}
	w1 := iworld.NewWorld("world")
	w1.SetGenerator(gen.NewSuperflat())
	w1.SetStorage(store1)

	fresh := w1.LoadChunk(0, 0)
	if got := skyLightSections(ConvertToLevelChunk(fresh)); got == 0 {
		t.Fatalf("control failed: freshly generated superflat chunk has 0 sky-light sections")
	}
	if err := store1.SaveChunk(0, 0, fresh); err != nil {
		t.Fatalf("SaveChunk: %v", err)
	}
	if err := store1.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	// World #2 simulates a server restart: fresh in-memory state, same on-disk
	// region directory. LoadChunk now serves the chunk from disk.
	store2, err := iworld.NewRegionStorage(filepath.Join(dir, "world"))
	if err != nil {
		t.Fatalf("NewRegionStorage(2): %v", err)
	}
	w2 := iworld.NewWorld("world")
	w2.SetGenerator(gen.NewSuperflat())
	w2.SetStorage(store2)

	loaded := w2.LoadChunk(0, 0)
	if got := skyLightSections(ConvertToLevelChunk(loaded)); got == 0 {
		t.Fatalf("chunk loaded from disk has 0 sky-light sections → Java renders pitch black at noon")
	}
}
