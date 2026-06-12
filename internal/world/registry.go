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

// WaterID is the canonical ID of a water source block. M1: used by
// the enderman's water-damage probe (`def.WaterSensitive`).
// Computed lazily through StateID so a future world re-mapping
// (e.g. a different palette) doesn't need to update this
// constant. Note: the canonical state 0 is air, state 1 is stone,
// and water is one of the early states in the global palette
// (≈ 33 for `minecraft:water` with default level=0). We resolve
// it once at init via block.ToStateID.
var WaterID = func() int32 {
	if b, ok := block.FromID["minecraft:water"]; ok {
		return int32(block.ToStateID[b])
	}
	return AirID
}()

// StateCount returns the number of known block states in the global palette.
func StateCount() int { return len(block.StateList) }

// defaultStateOverrides fixes blocks whose Go zero-value property struct
// is NOT a valid vanilla state. go-mc's FromID returns zero-value structs:
// for leaves that means distance=0 (valid range 1..7) and for snow layers
// layers=0 (valid range 1..8) — ToStateID misses and the lookup silently
// produced 0 (air), which made every worldgen tree generate bare trunks.
// Logs/deepslate additionally default to axis=x (Axis zero value) where
// vanilla's default state is axis=y.
var defaultStateOverrides = map[string]block.Block{
	"minecraft:oak_leaves":      block.OakLeaves{Distance: 7},
	"minecraft:spruce_leaves":   block.SpruceLeaves{Distance: 7},
	"minecraft:birch_leaves":    block.BirchLeaves{Distance: 7},
	"minecraft:jungle_leaves":   block.JungleLeaves{Distance: 7},
	"minecraft:acacia_leaves":   block.AcaciaLeaves{Distance: 7},
	"minecraft:cherry_leaves":   block.CherryLeaves{Distance: 7},
	"minecraft:dark_oak_leaves": block.DarkOakLeaves{Distance: 7},
	"minecraft:mangrove_leaves": block.MangroveLeaves{Distance: 7},
	"minecraft:snow":            block.Snow{Layers: 1},
	"minecraft:oak_log":         block.OakLog{Axis: block.Y},
	"minecraft:spruce_log":      block.SpruceLog{Axis: block.Y},
	"minecraft:birch_log":       block.BirchLog{Axis: block.Y},
	"minecraft:jungle_log":      block.JungleLog{Axis: block.Y},
	"minecraft:acacia_log":      block.AcaciaLog{Axis: block.Y},
	"minecraft:cherry_log":      block.CherryLog{Axis: block.Y},
	"minecraft:dark_oak_log":    block.DarkOakLog{Axis: block.Y},
	"minecraft:deepslate":       block.Deepslate{Axis: block.Y},
}

// StateID resolves the canonical world ID for a namespaced block name
// (e.g. "minecraft:stone"). Properties default to the block's default state.
// Unknown names resolve to air.
func StateID(name string) int32 {
	if b, ok := defaultStateOverrides[name]; ok {
		if sid, found := block.ToStateID[b]; found {
			return int32(sid)
		}
	}
	if b, ok := block.FromID[name]; ok {
		if sid, found := block.ToStateID[b]; found {
			return int32(sid)
		}
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
