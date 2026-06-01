package world

import (
	"sync"

	"github.com/google/uuid"
)

// PROBLEM #4 — cross-edition block break/crack effects.
//
// A block-break or crack animation performed on one edition was never rendered on
// the other (Java's HandlePlayerAction had TODOs; Bedrock's crack/particles went
// only to Bedrock viewers). This bus — modeled on BlockEventBus — carries action
// EFFECTS (crack overlay, block-destroy particles+sound) so each edition can
// render the other's actions.
//
// Echo/double-play avoidance: every publisher stamps Source, and every subscriber
// SKIPS its own Source. Same-edition rendering is already handled locally (the
// acting Java client predicts its own break; Bedrock keeps its existing direct
// broadcast), so the bus only needs to reach the OTHER edition. This is why the
// effect bus filters by source even though the block-update bus does not — a
// double LevelEvent would replay the break SOUND on the origin edition.

type WorldEffectKind string

const (
	// EffectCrackProgress is a block-breaking overlay update (Stage) or clear.
	EffectCrackProgress WorldEffectKind = "crack-progress"
	// EffectBlockDestroy is a finished break: break particles + sound at the block.
	EffectBlockDestroy WorldEffectKind = "block-destroy"
)

// WorldEffectEvent carries one cross-edition action effect.
type WorldEffectEvent struct {
	Kind    WorldEffectKind
	Source  BlockUpdateSource
	X, Y, Z int
	BlockID int32     // canonical id of the destroyed block (EffectBlockDestroy)
	Stage   int32     // crack stage: >=0 = progress, <0 = stop/clear overlay
	Breaker uuid.UUID // breaking player (Java BlockDestruction keys the overlay by entity id)
}

// WorldEffectBus is a small dependency-free pub/sub, identical in shape to
// BlockEventBus, with the same non-blocking drop-on-full delivery.
type WorldEffectBus struct {
	mu          sync.RWMutex
	subscribers map[string]chan WorldEffectEvent
}

func NewWorldEffectBus() *WorldEffectBus {
	return &WorldEffectBus{subscribers: make(map[string]chan WorldEffectEvent)}
}

func (b *WorldEffectBus) Subscribe(id string, buffer int) <-chan WorldEffectEvent {
	if buffer <= 0 {
		buffer = 64
	}
	ch := make(chan WorldEffectEvent, buffer)
	b.mu.Lock()
	b.subscribers[id] = ch
	b.mu.Unlock()
	return ch
}

func (b *WorldEffectBus) Unsubscribe(id string) {
	b.mu.Lock()
	if ch, ok := b.subscribers[id]; ok {
		delete(b.subscribers, id)
		close(ch)
	}
	b.mu.Unlock()
}

func (b *WorldEffectBus) Publish(ev WorldEffectEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.subscribers {
		select {
		case ch <- ev:
		default:
		}
	}
}
