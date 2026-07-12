package world

import (
	"github.com/google/uuid"
)

func (m *Manager) SubscribeBlockUpdates(id string, buffer int) <-chan BlockUpdateEvent {
	return m.blockEvents.Subscribe(id, buffer)
}

func (m *Manager) UnsubscribeBlockUpdates(id string) {
	m.blockEvents.Unsubscribe(id)
}

// SubscribeLightUpdates returns a channel of chunks whose light was recomputed
// and changed during a tick. The Java bridge uses it to re-send light to players.
func (m *Manager) SubscribeLightUpdates(id string, buffer int) <-chan LightUpdateEvent {
	return m.lightEvents.Subscribe(id, buffer)
}

func (m *Manager) UnsubscribeLightUpdates(id string) {
	m.lightEvents.Unsubscribe(id)
}

// PublishLightUpdate announces that chunk (cx,cz) had its light changed.
func (m *Manager) PublishLightUpdate(cx, cz int) {
	m.lightEvents.Publish(LightUpdateEvent{X: cx, Z: cz})
}

func (m *Manager) PublishBlockUpdate(source BlockUpdateSource, x, y, z int, blockID int32) {
	m.blockEvents.Publish(BlockUpdateEvent{Source: source, X: x, Y: y, Z: z, BlockID: blockID})
}

func (m *Manager) SetBlockAndPublish(source BlockUpdateSource, x, y, z int, block Block) {
	m.GetDefaultWorld().SetBlock(x, y, z, block)
	m.PublishBlockUpdate(source, x, y, z, block.ID())
}

func (m *Manager) SubscribeWorldEffects(id string, buffer int) <-chan WorldEffectEvent {
	return m.effectEvents.Subscribe(id, buffer)
}

func (m *Manager) UnsubscribeWorldEffects(id string) {
	m.effectEvents.Unsubscribe(id)
}

func (m *Manager) PublishWorldEffect(ev WorldEffectEvent) {
	m.effectEvents.Publish(ev)
}

// PublishCrack publishes a crack-overlay update (stage>=0) or clear (stage<0) for
// a block being broken by breaker, originating from source.
func (m *Manager) PublishCrack(source BlockUpdateSource, breaker uuid.UUID, x, y, z int, stage int32) {
	m.effectEvents.Publish(WorldEffectEvent{
		Kind: EffectCrackProgress, Source: source, X: x, Y: y, Z: z, Stage: stage, Breaker: breaker,
	})
}

// PublishBlockDestroy publishes a finished-break effect (particles+sound) for the
// block of canonical id blockID at (x,y,z), originating from source.
func (m *Manager) PublishBlockDestroy(source BlockUpdateSource, breaker uuid.UUID, x, y, z int, blockID int32) {
	m.effectEvents.Publish(WorldEffectEvent{
		Kind: EffectBlockDestroy, Source: source, X: x, Y: y, Z: z, BlockID: blockID, Breaker: breaker,
	})
}
