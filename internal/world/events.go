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
