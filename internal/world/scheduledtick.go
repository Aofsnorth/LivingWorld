package world

import (
	"container/heap"
	"sync"
)

// ScheduledTick is a block update scheduled for a future tick. Vanilla
// uses a priority queue of (position, block, delay) sorted by tick number.
// Redstone components, fluid flow, gravity blocks, and crop growth all
// schedule their updates through this system.
//
// LivingWorld implements the priority queue but only dispatches to a
// generic "neighbor notify" for now. Full redstone/fluid dispatch will
// be added in Phase 4d.

// ScheduledTickEntry is one entry in the tick queue.
type ScheduledTickEntry struct {
	X, Y, Z   int
	BlockName string
	TickNum   uint64 // the tick number when this should fire
	Priority  int    // lower = earlier (vanilla: 0 = normal)
	index     int    // heap index
}

// TickQueue is a min-heap of scheduled block updates, ordered by TickNum.
type TickQueue []*ScheduledTickEntry

func (q TickQueue) Len() int            { return len(q) }
func (q TickQueue) Less(i, j int) bool  { return q[i].TickNum < q[j].TickNum }
func (q TickQueue) Swap(i, j int)       { q[i], q[j] = q[j], q[i]; q[i].index = i; q[j].index = j }
func (q *TickQueue) Push(x interface{}) { e := x.(*ScheduledTickEntry); e.index = len(*q); *q = append(*q, e) }
func (q *TickQueue) Pop() interface{}   { old := *q; n := len(old); e := old[n-1]; old[n-1] = nil; *q = old[:n-1]; return e }

// ScheduledTickSystem manages the priority queue of future block updates
// and dispatches them when their tick number arrives.
type ScheduledTickSystem struct {
	mu       sync.Mutex
	queue    TickQueue
	current  uint64 // current tick number
	handlers map[string]BlockTickHandler
}

// BlockTickHandler is called when a scheduled tick fires for a block type.
// The handler receives the world and position; it can schedule more ticks,
// modify blocks, etc.
type BlockTickHandler func(w *World, x, y, z int)

// NewScheduledTickSystem creates a new tick scheduler.
func NewScheduledTickSystem() *ScheduledTickSystem {
	return &ScheduledTickSystem{
		handlers: make(map[string]BlockTickHandler),
	}
}

// RegisterHandler registers a tick handler for a block type name.
func (s *ScheduledTickSystem) RegisterHandler(blockName string, handler BlockTickHandler) {
	s.mu.Lock()
	s.handlers[blockName] = handler
	s.mu.Unlock()
}

// Schedule adds a block update to the queue, to fire after `delay` ticks.
func (s *ScheduledTickSystem) Schedule(x, y, z int, blockName string, delay uint64) {
	s.mu.Lock()
	entry := &ScheduledTickEntry{
		X:         x,
		Y:         y,
		Z:         z,
		BlockName: blockName,
		TickNum:   s.current + delay,
		Priority:  0,
	}
	heap.Push(&s.queue, entry)
	s.mu.Unlock()
}

// Process fires all scheduled ticks whose tick number has arrived.
// Called once per 20 Hz tick from the unified tick loop.
func (s *ScheduledTickSystem) Process(w *World) {
	s.mu.Lock()
	s.current++
	current := s.current

	// Collect all entries that are due.
	var due []*ScheduledTickEntry
	for s.queue.Len() > 0 && s.queue[0].TickNum <= current {
		entry := heap.Pop(&s.queue).(*ScheduledTickEntry)
		due = append(due, entry)
	}
	// Copy handlers map to avoid holding the lock during dispatch.
	handlers := make(map[string]BlockTickHandler, len(s.handlers))
	for k, v := range s.handlers {
		handlers[k] = v
	}
	s.mu.Unlock()

	// Dispatch each due tick.
	for _, entry := range due {
		if handler, ok := handlers[entry.BlockName]; ok {
			handler(w, entry.X, entry.Y, entry.Z)
		}
	}

	// Safety: limit queue size to prevent runaway scheduling.
	s.mu.Lock()
	const maxQueueSize = 100000
	for s.queue.Len() > maxQueueSize {
		heap.Pop(&s.queue) // drop oldest excess entries
	}
	s.mu.Unlock()
}

// NeighborNotify sends a block update to all 6 neighbors of (x, y, z).
// This is the vanilla "neighbor update" that triggers redstone wire
// recalculation, comparator reads, observer triggers, etc.
//
// For now, this schedules a tick for each neighbor's block type so that
// registered handlers fire on the next tick. Full redstone dispatch
// will extend this in Phase 4d.
func (s *ScheduledTickSystem) NeighborNotify(w *World, x, y, z int) {
	offsets := [][3]int{
		{1, 0, 0}, {-1, 0, 0},
		{0, 1, 0}, {0, -1, 0},
		{0, 0, 1}, {0, 0, -1},
	}
	for _, off := range offsets {
		nx, ny, nz := x+off[0], y+off[1], z+off[2]
		blockID := w.GetBlock(nx, ny, nz).ID()
		if blockID == AirID {
			continue
		}
		name := StateName(blockID)
		s.mu.Lock()
		_, hasHandler := s.handlers[name]
		s.mu.Unlock()
		if hasHandler {
			s.Schedule(nx, ny, nz, name, 1)
		}
	}
}

// QueueSize returns the number of pending scheduled ticks.
func (s *ScheduledTickSystem) QueueSize() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.queue.Len()
}
