package world

import "sync"

// BlockUpdateSource identifies which protocol originated a block update.
type BlockUpdateSource string

const (
	BlockUpdateSourceJava    BlockUpdateSource = "java"
	BlockUpdateSourceBedrock BlockUpdateSource = "bedrock"
	BlockUpdateSourceServer  BlockUpdateSource = "server"
)

// BlockUpdateEvent is emitted after a block changes in the shared world model.
// BlockID is LivingWorld's canonical block ID, which equals the vanilla Java
// global block-state ID (see registry.go). Java uses it directly; Bedrock maps
// it through the block name.
type BlockUpdateEvent struct {
	Source  BlockUpdateSource
	X, Y, Z int
	BlockID int32
}

// BlockEventBus is intentionally small and dependency-free. Both Java and
// Bedrock servers subscribe to it and translate BlockID into their protocol's
// runtime/state IDs.
type BlockEventBus struct {
	mu          sync.RWMutex
	subscribers map[string]chan BlockUpdateEvent
}

func NewBlockEventBus() *BlockEventBus {
	return &BlockEventBus{subscribers: make(map[string]chan BlockUpdateEvent)}
}

func (b *BlockEventBus) Subscribe(id string, buffer int) <-chan BlockUpdateEvent {
	if buffer <= 0 {
		buffer = 64
	}
	ch := make(chan BlockUpdateEvent, buffer)
	b.mu.Lock()
	b.subscribers[id] = ch
	b.mu.Unlock()
	return ch
}

func (b *BlockEventBus) Unsubscribe(id string) {
	b.mu.Lock()
	if ch, ok := b.subscribers[id]; ok {
		delete(b.subscribers, id)
		close(ch)
	}
	b.mu.Unlock()
}

func (b *BlockEventBus) Publish(ev BlockUpdateEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.subscribers {
		select {
		case ch <- ev:
		default:
		}
	}
}

// LightUpdateEvent is emitted after a chunk's sky/block light is recomputed and
// actually changed (see LightEngine.ProcessUpdates). It carries only the chunk
// coordinates; subscribers (the Java bridge) re-send that chunk's light to any
// player who already has it loaded, so cross-chunk relights become visible
// without a fresh chunk-stream. Bedrock computes light client-side and ignores
// this.
type LightUpdateEvent struct {
	X, Z int
}

// LightEventBus mirrors BlockEventBus: a small, dependency-free fan-out with
// non-blocking drop-on-full delivery. A dropped light event is harmless — the
// chunk simply keeps its prior (still-lit) state until the next relight.
type LightEventBus struct {
	mu          sync.RWMutex
	subscribers map[string]chan LightUpdateEvent
}

func NewLightEventBus() *LightEventBus {
	return &LightEventBus{subscribers: make(map[string]chan LightUpdateEvent)}
}

func (b *LightEventBus) Subscribe(id string, buffer int) <-chan LightUpdateEvent {
	if buffer <= 0 {
		buffer = 64
	}
	ch := make(chan LightUpdateEvent, buffer)
	b.mu.Lock()
	b.subscribers[id] = ch
	b.mu.Unlock()
	return ch
}

func (b *LightEventBus) Unsubscribe(id string) {
	b.mu.Lock()
	if ch, ok := b.subscribers[id]; ok {
		delete(b.subscribers, id)
		close(ch)
	}
	b.mu.Unlock()
}

func (b *LightEventBus) Publish(ev LightUpdateEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.subscribers {
		select {
		case ch <- ev:
		default:
		}
	}
}
