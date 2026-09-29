package world_test

import (
	"os"
	"testing"

	"livingworld/internal/dimension/overworld/pipeline"
	"livingworld/internal/world"
)

// BenchGenerate measures the overworld pipeline, which is the heaviest thing a
// player walking into new terrain triggers.
func BenchmarkGenerate(b *testing.B) {
	g := pipeline.NewGenerator(12345)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g.Generate(i%64, i%64)
	}
}

func BenchmarkGenerateParallel(b *testing.B) {
	g := pipeline.NewGenerator(12345)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			g.Generate(i%64, (i*7)%64)
			i++
		}
	})
}

// BenchEncode measures the per-chunk serialisation that persistence pays for.
func BenchmarkEncode(b *testing.B) {
	c := pipeline.NewGenerator(12345).Generate(3, 7)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.Encode()
	}
}

// BenchmarkRegionSave measures a full region round trip: encode + compress +
// write, which is what every generated chunk now costs because generated
// chunks are persisted so that terrain does not change under a version bump.
func BenchmarkRegionSave(b *testing.B) {
	chunks := make([]*world.Chunk, 0, 64)
	for i := 0; i < 64; i++ {
		chunks = append(chunks, pipeline.NewGenerator(12345).Generate(i%8, i/8))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dir := b.TempDir()
		s, err := world.NewRegionStorage(dir)
		if err != nil {
			b.Fatal(err)
		}
		for j, c := range chunks {
			if err := s.SaveChunk(j%8, j/8, c); err != nil {
				b.Fatal(err)
			}
		}
		if err := s.Flush(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRegionSaveOnly isolates the cost of persisting an already-built
// chunk, i.e. what my "persist generated chunks" change added to a fresh world.
func BenchmarkRegionSaveOnly(b *testing.B) {
	dir := b.TempDir()
	defer os.RemoveAll(dir)
	store, err := world.NewRegionStorage(dir)
	if err != nil {
		b.Fatal(err)
	}
	c := pipeline.NewGenerator(12345).Generate(2, 2)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := store.SaveChunk(i%8, i/8, c); err != nil {
			b.Fatal(err)
		}
	}
}
