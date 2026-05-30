// Package registry holds LivingWorld's canonical, edition-agnostic data model
// (DESIGN §4) together with the Java<->Bedrock id maps (R8). The canonical
// block-state id IS the vanilla Java global state id; every other package
// builds on these types and translates to edition-specific ids only at the
// network edge. Other packages must import these types, not redefine them.
package registry

import "github.com/google/uuid"

// Canonical primitives.
type (
	BlockState uint32         // canonical = Java global state id
	NBT        map[string]any // item components / block-entity data
	MetaMap    map[uint8]any  // entity metadata, by field index
)

// Pos is integer block coordinates.
type Pos struct{ X, Y, Z int }

// Vec3 is a continuous position/vector. Stored as f64 (canonical); downcast to
// f32 only at the Bedrock edge.
type Vec3 struct{ X, Y, Z float64 }

// ItemStack is an edition-agnostic item stack keyed by namespaced id.
type ItemStack struct {
	ID         string
	Count      uint8
	Meta       int16
	Components NBT
}

// Entity is the canonical entity base. Behavior (AI, pathfinding, spawning,
// metadata sync) lives in internal/entity on top of this type.
type Entity struct {
	ID   int32
	UUID uuid.UUID
	Type string
	Pos  Vec3
	Vel  Vec3
	Meta MetaMap
}
