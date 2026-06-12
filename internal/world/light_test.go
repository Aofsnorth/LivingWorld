package world

import (
	"sync"
	"testing"
	"time"
)

func TestLightProcessUpdatesHandlesNegativeChunks(t *testing.T) {
	w := NewWorld("test")
	w.LoadChunk(-8, -7)
	w.Light().QueueUpdate(-8, -7)
	w.Light().ProcessUpdates()
}

func TestLightProcessUpdatesSerializesConcurrentChunkLoads(t *testing.T) {
	w := NewWorld("test")
	w.LoadChunk(-8, -7)
	w.Light().QueueUpdate(-8, -7)

	started := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		close(started)
		deadline := time.Now().Add(50 * time.Millisecond)
		for time.Now().Before(deadline) {
			w.LoadChunk(-9, -7)
			w.LoadChunk(-8, -8)
		}
	}()
	<-started
	w.Light().ProcessUpdates()
	wg.Wait()
}
