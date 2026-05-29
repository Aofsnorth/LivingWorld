package world

import (
	"github.com/Tnze/go-mc/level/block"
)

// Canonical block identity in LivingWorld.
//
// A block's canonical world ID equals the vanilla Java *global block-state ID*
// (the dense 0..N enumeration of every block state in the game). This is the
// universal Minecraft block-state space:
//
//   - The Java protocol uses these IDs directly on the wire, so Java chunk and
//     block-update packets need no translation at all (identity mapping).
//   - The Bedrock side maps a state ID to its namespaced name and resolves the
//     Bedrock runtime ID from dragonfly's palette (see the bedrock world pkg).
//
// go-mc ships the full 26.1 palette (~29.8k states), so every vanilla block is
// representable without hand-maintained tables.

// AirID is the canonical ID of air. In the vanilla global palette air is state 0.
const AirID int32 = 0

// StateCount returns the number of known block states in the global palette.
func StateCount() int { return len(block.StateList) }

// StateID resolves the canonical world ID for a namespaced block name
// (e.g. "minecraft:stone"). Properties default to the block's default state.
// Unknown names resolve to air.
func StateID(name string) int32 {
	if b, ok := block.FromID[name]; ok {
		return int32(block.ToStateID[b])
	}
	return AirID
}

// StateName returns the namespaced name for a canonical world block ID
// (properties are not encoded in the name). Out-of-range IDs return air.
func StateName(id int32) string {
	if id < 0 || int(id) >= len(block.StateList) {
		return "minecraft:air"
	}
	return block.StateList[id].ID()
}

// ValidStateID reports whether id is within the global palette range.
func ValidStateID(id int32) bool {
	return id >= 0 && int(id) < len(block.StateList)
}

// IsAir reports whether id is air.
func IsAir(id int32) bool { return id == AirID }
