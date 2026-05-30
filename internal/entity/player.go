package entity

import "livingworld/internal/registry"

// PlayerType is the canonical entity-type id for players.
const PlayerType = "minecraft:player"

// Player is the canonical, edition-agnostic player model: a registry.Entity
// base (id/uuid/pos/vel/meta) plus the shared player state that the network
// edges (entity_sync) and the dfcompat adapter map onto, so no lane redefines
// its own player. Yaw/Pitch are canonical f64; downcast to f32 at the edge.
type Player struct {
	registry.Entity
	Name     string
	Yaw      float64
	Pitch    float64
	OnGround bool
	Sneaking bool
}
