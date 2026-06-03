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
	// EffectStatus (M1/M6) is a mob-applied status effect (hunger /
	// poison / wither / slowness / instant_damage). EffectID is the
	// vanilla effect id (1-30; same numbering on Java and Bedrock),
	// Data is the amplifier (0 = I, 1 = II, ...), Aux is the
	// duration in ticks. Bridges translate to their edition's status
	// packet (Java ClientboundUpdateMobEffect id 132; Bedrock
	// packet.MobEffect with Operation=MobEffectAdd).
	EffectStatus WorldEffectKind = "status-effect"
	// EffectStatusRemove (M6) clears one effect from a player. The
	// bridges translate to ClientboundRemoveMobEffect (Java id 78)
	// or packet.MobEffect with Operation=MobEffectRemove (Bedrock).
	// EffectID identifies which effect; the event arrives once per
	// effect expiry (per-effect bag in the player manager).
	EffectStatusRemove WorldEffectKind = "status-effect-remove"
)

// Status-effect kind codes (Data field of EffectStatus). Matches the
// vanilla Java Effect ID range (1-30) where it makes sense, but the
// bridges do their own mapping; the codes here are for clarity.
const (
	WorldEffectStatus = EffectStatus // alias for clarity at call sites
)

// WorldEffectEvent carries one cross-edition action effect.
//
// M1: Target field added so the bridges can address a specific
// player for the EffectStatus path. The Source field is the
// attacker (e.g. wither skeleton) or zero for environment effects.
type WorldEffectEvent struct {
	Kind    WorldEffectKind
	Source  BlockUpdateSource
	X, Y, Z int
	BlockID int32     // canonical id of the destroyed block (EffectBlockDestroy)
	Stage   int32     // crack stage: >=0 = progress, <0 = stop/clear overlay
	Breaker uuid.UUID // breaking player (Java BlockDestruction keys the overlay by entity id)
	Target  uuid.UUID // M1: target player (for EffectStatus / EffectStatusRemove)
	Data    int32     // M1: effect level (amplifier+1 convention) for EffectStatus
	Aux     int32     // M1: duration in ticks (or extra data) for EffectStatus
	// EffectID is the vanilla effect id (1-30) for EffectStatus /
	// EffectStatusRemove. Both Java 1.21 and Bedrock gophertunnel
	// number MobEffect identically (EffectSpeed=1..EffectSlowFalling=27),
	// so a single id travels through both bridges unchanged.
	EffectID int32
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
