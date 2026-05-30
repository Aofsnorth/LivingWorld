package plugin

// World is a handle to a single world instance handed to plugins (DESIGN §7).
// Like Host it is intentionally primitive-typed so the plugin package stays
// dependency-free; the server adapts its *world.World to this surface. Block IDs
// are LivingWorld canonical state IDs (= vanilla global block-state IDs).
type World interface {
	// Name returns the world's identifier (e.g. "world").
	Name() string
	// GetBlock returns the block state ID at a world position.
	GetBlock(x, y, z int) int32
	// SetBlock sets the block state ID at a world position and notifies clients.
	SetBlock(x, y, z int, stateID int32)
	// StateID resolves a block state ID from a namespaced name (e.g. "minecraft:stone").
	StateID(name string) int32
}

// WorldHost is the optional Host capability that exposes a world handle
// (DESIGN §7: Host.World()). It is kept additive — separate from the core Host
// interface — so existing Host implementers keep satisfying Host without change;
// a Host that can hand out a world simply also implements WorldHost. Resolve it
// with WorldOf instead of asserting inline.
type WorldHost interface {
	World() World
}

// WorldOf returns the host's world handle, or nil if the host doesn't expose one.
func WorldOf(h Host) World {
	if wh, ok := h.(WorldHost); ok {
		return wh.World()
	}
	return nil
}
