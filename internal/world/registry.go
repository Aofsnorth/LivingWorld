package world

import (
	"reflect"

	"github.com/Tnze/go-mc/level/biome"
	"github.com/Tnze/go-mc/level/block"
	"github.com/Tnze/go-mc/nbt"
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

// CaveAirID and VoidAirID are the canonical IDs of cave_air and void_air.
var (
	CaveAirID = func() int32 {
		if b, ok := block.FromID["minecraft:cave_air"]; ok {
			return int32(block.ToStateID[b])
		}
		return AirID
	}()
	VoidAirID = func() int32 {
		if b, ok := block.FromID["minecraft:void_air"]; ok {
			return int32(block.ToStateID[b])
		}
		return AirID
	}()
)

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
	"minecraft:dark_oak_log":            block.DarkOakLog{Axis: block.Y},
	"minecraft:pale_oak_log":            block.PaleOakLog{Axis: block.Y},
	"minecraft:pale_oak_leaves":         block.PaleOakLeaves{Distance: 7},
	"minecraft:azalea_leaves":           block.AzaleaLeaves{Distance: 7},
	"minecraft:flowering_azalea_leaves": block.FloweringAzaleaLeaves{Distance: 7},
	"minecraft:deepslate":               block.Deepslate{Axis: block.Y},
	"minecraft:vine":                    block.Vine{North: true, South: true, East: true, West: true},
	"minecraft:wildflowers":             block.Wildflowers{Facing: block.North, FlowerAmount: 1},
	"minecraft:pink_petals":             block.PinkPetals{Facing: block.North, FlowerAmount: 1},
	"minecraft:leaf_litter":             block.LeafLitter{Facing: block.North, SegmentAmount: 1},
}

var firstValidStateByName map[string]int32

func init() {
	firstValidStateByName = make(map[string]int32, len(block.FromID))
	for sid, b := range block.StateList {
		if sid == 0 {
			continue
		}
		name := b.ID()
		if _, exists := firstValidStateByName[name]; !exists {
			firstValidStateByName[name] = int32(sid)
		}
	}
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
	if sid, ok := firstValidStateByName[name]; ok {
		return sid
	}
	return AirID
}

// StateIDWithProperties resolves the canonical world ID for a namespaced
// block name with a complete Java property set (property name -> string
// value, e.g. {"axis": "y"}), falling back to the block's default state
// whenever the property set is empty, fails to decode, or does not denote
// a valid state. It exists for Bedrock->Java translation (blockconv):
// a PARTIAL property set must never be resolved directly, because go-mc
// decodes properties onto the zero-value block struct, whose values
// (axis=x, distance=0, layers=0, ...) are frequently invalid states that
// silently miss ToStateID. Callers pass complete sets only.
func StateIDWithProperties(name string, props map[string]string) int32 {
	if len(props) == 0 {
		return StateID(name)
	}
	raw, err := propertiesToRawMessage(props)
	if err != nil {
		return StateID(name)
	}
	b, err := (&block.State{Name: name, Properties: raw}).Block()
	if err != nil {
		return StateID(name)
	}
	if sid, found := block.ToStateID[b]; found {
		return int32(sid)
	}
	return StateID(name)
}

// propertiesToRawMessage encodes a Java property set as the NBT compound
// payload go-mc's State.Block expects (mirrors java/registry's marshalNBT:
// nbt.Marshal prefixes the compound type+name header, which RawMessage
// carries separately).
func propertiesToRawMessage(props map[string]string) (nbt.RawMessage, error) {
	data, err := nbt.Marshal(props)
	if err != nil {
		return nbt.RawMessage{}, err
	}
	return nbt.RawMessage{Type: nbt.TagCompound, Data: data[3:]}, nil
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

// DefaultBiomeID is the canonical Java biome ID for minecraft:plains (41).
// Note: biome.Type(0) in go-mc's alphabetical list is minecraft:badlands, NOT plains!
var DefaultBiomeID = func() int32 {
	var b biome.Type
	if err := b.UnmarshalText([]byte("minecraft:plains")); err == nil {
		return int32(b)
	}
	return 41
}()

// BiomeID resolves a namespaced Java biome name (e.g. "minecraft:plains" or "plains")
// into its canonical Java protocol biome ID (go-mc biome.Type), falling back to DefaultBiomeID.
func BiomeID(name string) int32 {
	var b biome.Type
	if err := b.UnmarshalText([]byte(name)); err == nil {
		return int32(b)
	}
	return DefaultBiomeID
}

// BiomeName returns the namespaced Java biome name (e.g. "minecraft:plains")
// for a canonical biome ID.
func BiomeName(id int32) string {
	text, err := biome.Type(id).MarshalText()
	if err != nil {
		return "minecraft:plains"
	}
	return string(text)
}

var (
	waterStateLUT []bool
	lavaStateLUT  []bool
)

func init() {
	n := len(block.StateList)
	waterStateLUT = make([]bool, n)
	lavaStateLUT = make([]bool, n)
	for sid, b := range block.StateList {
		if sid == 0 || b == nil {
			continue
		}
		switch b.ID() {
		case "minecraft:water", "minecraft:flowing_water", "minecraft:bubble_column",
			"minecraft:seagrass", "minecraft:tall_seagrass", "minecraft:kelp", "minecraft:kelp_plant":
			waterStateLUT[sid] = true
			continue
		case "minecraft:lava", "minecraft:flowing_lava":
			lavaStateLUT[sid] = true
			continue
		}
		rv := reflect.ValueOf(b)
		if rv.Kind() == reflect.Struct {
			if f := rv.FieldByName("Waterlogged"); f.IsValid() && f.Kind() == reflect.Bool && f.Bool() {
				waterStateLUT[sid] = true
			}
		}
	}
}

// IsWaterBlockID reports whether a canonical block state ID represents water
// or a waterlogged underwater plant/block.
func IsWaterBlockID(id int32) bool {
	if id <= 0 || int(id) >= len(waterStateLUT) {
		return false
	}
	return waterStateLUT[id]
}

// IsLavaBlockID reports whether a canonical block state ID represents lava.
func IsLavaBlockID(id int32) bool {
	if id <= 0 || int(id) >= len(lavaStateLUT) {
		return false
	}
	return lavaStateLUT[id]
}

// IsFluidBlockID reports whether a canonical block state ID contains a fluid
// (water, waterlogged block/plant, or lava) for chunk section FluidCount tracking.
func IsFluidBlockID(id int32) bool {
	if id <= 0 {
		return false
	}
	return (int(id) < len(waterStateLUT) && waterStateLUT[id]) ||
		(int(id) < len(lavaStateLUT) && lavaStateLUT[id])
}
