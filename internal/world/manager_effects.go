package world

// SetEffectTickCallback (M6) wires the per-tick effect engine. Called
// once at server boot from server.go, after both the world manager and
// the player manager are constructed. The closure is invoked from
// Phase 4e of runOneTick and is responsible for walking every connected
// player's effect bag, applying per-tick damage, and publishing
// EffectStatusRemove for expired effects.
//
// Kept separate from SetMobAICallbacks so the existing 10-arg signature
// stays stable and the player manager (which knows about the per-player
// effect bag) owns the implementation.
func (m *Manager) SetEffectTickCallback(fn func()) {
	m.locatorMu.Lock()
	m.aiTickEffects = fn
	m.locatorMu.Unlock()
}

// SetIFramesTickCallback (M7) wires the I-frames countdown engine.
// The closure is invoked from Phase 4e (right after the effect
// tick) and walks the player map, decrementing any IFrames > 0
// counter. The world package stays protocol-agnostic; the
// player manager owns the IFrames field.
func (m *Manager) SetIFramesTickCallback(fn func()) {
	m.locatorMu.Lock()
	m.aiTickIFrames = fn
	m.locatorMu.Unlock()
}
