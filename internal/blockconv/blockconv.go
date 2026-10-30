// Package blockconv translates Bedrock block states, as reported by
// dragonfly's block registry (a namespaced name plus a property map), into
// canonical Java block states (a namespaced name plus string properties),
// which internal/world resolves to global Java state IDs.
//
// The package is deliberately dependency-free (stdlib only): the Bedrock
// runtime-ID lookup stays in dragonfly-importing packages, and the final
// Java state resolution stays in internal/world next to the palette. That
// keeps blockconv importable from worldgen, world conversion, and the
// Bedrock networking layer without dragging dragonfly or gophertunnel into
// any of them.
//
// Translation policy: every Java property of the target block is either
// translated from its Bedrock counterpart or synthesized (usually a decay /
// ticking default such as leaves distance). Bedrock-only properties are
// dropped. Any property left untranslated yields a partial property set,
// which callers must reject — a partial set resolves to go-mc's zero-value
// struct, whose property values are frequently invalid states.
package blockconv

import (
	"strconv"
	"strings"
)

// BedrockNameToJavaName maps Bedrock block identifiers whose Java
// identifier differs onto the Java name. It is the single source of truth
// for cross-edition name aliases; JavaNameToBedrockName is its exact
// inverse, derived at init so the two directions cannot drift.
//
// This subsumes the aliases previously hand-maintained in both directions
// by the Bedrock networking layer, plus legacy Bedrock names emitted by
// current generators (magma, web) that previously failed resolution.
var BedrockNameToJavaName = map[string]string{
	"minecraft:snow_layer":    "minecraft:snow", // Bedrock full block is "snow"
	"minecraft:snow":          "minecraft:snow_block",
	"minecraft:hardened_clay": "minecraft:terracotta",
	"minecraft:deadbush":      "minecraft:dead_bush",
	"minecraft:reeds":         "minecraft:sugar_cane",
	"minecraft:magma":         "minecraft:magma_block",
	"minecraft:web":           "minecraft:cobweb",
	"minecraft:quartz_ore":    "minecraft:nether_quartz_ore",
}

// JavaNameToBedrockName is the exact inverse of BedrockNameToJavaName.
var JavaNameToBedrockName = invertNames(BedrockNameToJavaName)

func invertNames(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for bedrock, java := range m {
		out[java] = bedrock
	}
	return out
}

// Facing convention (verified against dragonfly v0.11.5 source and the
// vanilla Java palette): in BOTH editions a *_stairs / wall-torch `facing`
// names the horizontal direction the block's front faces, and the compass
// directions agree (cube.Direction north=-Z == Java north=-Z). No
// mirroring is applied anywhere in this package. dragonfly encodes wall
// torches as the plain torch name with a non-"top"
// torch_facing_direction; Java splits them into wall_torch blocks whose
// facing is the direction from the wall into the room — the same compass
// direction Bedrock stores (Torch.Facing is "direction from the torch to
// the block" it is mounted on, inverted on encode, so the stored string
// already names the room side).

// bedrockFacingByWeirdo converts a Bedrock weirdo_direction value
// (stairs/fence-gate facing) to a Java horizontal facing. dragonfly's
// toStairsDirection is 3-cube.Direction with north=0, south=1, west=2,
// east=3, so weirdo 0=east, 1=west, 2=south, 3=north. Java stairs face
// the same compass direction.
var bedrockFacingByWeirdo = map[int32]string{
	0: "east",
	1: "west",
	2: "south",
	3: "north",
}

// propSpec describes how one Java block expects to see its property set:
// direct maps known Java property keys to Bedrock property keys whose
// values pass through unchanged (after direction mirroring for the keys
// listed in mirrored), while convert derives the Java values that have no
// 1:1 Bedrock key. Synthesized defaults are applied afterwards.
type propSpec struct {
	direct   map[string]string // Java key -> Bedrock key, verbatim string value
	mirrored map[string]string // Java key -> Bedrock key, horizontal direction mirrored
	convert  func(get func(string) (any, bool), out map[string]string)
}

// propSpecs is keyed by Java block base name (no namespace).
var propSpecs = map[string]propSpec{
	// Pillar blocks: Bedrock pillar_axis == Java axis for logs, stems,
	// deepslate, basalt, quartz pillars, chains, etc.
	"default": {
		direct: map[string]string{"axis": "pillar_axis"},
	},
}

// javaDefaults synthesizes Java properties that Bedrock states do not
// carry: decay/ticking clocks and the waterlogged flag (waterlogging is
// translated from layer 1 by the caller, never from the base state).
var javaDefaults = map[string]map[string]string{
	"leaves":          {"distance": "7", "waterlogged": "false"},
	"leaves2":         {"distance": "7", "waterlogged": "false"},
	"azalea_leaves":   {"distance": "7", "waterlogged": "false"},
	"mangrove_leaves": {"distance": "7", "persistent": "false", "waterlogged": "false"},
	// Bedrock encodes snow-layer thickness as height 0..7 (+1 = layers)
	// and the covered bit has no Java counterpart.
	"snow": {"layers": "1"},
}

// Translate converts a Bedrock block state (name + properties, as returned
// by dragonfly's RuntimeIDToState) into the equivalent Java block state:
// the Java name and the complete Java property set for that block.
//
// The boolean result is false when the state cannot be represented
// completely — an unknown per-block property shape, or a Bedrock property
// value outside the known range — in which case callers must fall back to
// the block's default Java state rather than resolve a partial property
// set. Waterlogging is not handled here: it arrives via chunk layer 1 and
// is the caller's concern.
func Translate(bedrockName string, props map[string]any) (javaName string, javaProps map[string]string, ok bool) {
	name := bedrockName
	if alias, isAlias := BedrockNameToJavaName[name]; isAlias {
		name = alias
	}
	base := strings.TrimPrefix(name, "minecraft:")

	get := func(key string) (any, bool) {
		v, present := props[key]
		return v, present
	}
	out := map[string]string{}

	// Name-splitting translations run first: they change the target block
	// itself, not just its properties.
	switch {
	case base == "flowing_water" || base == "flowing_lava":
		// Bedrock distinguishes still/flowing by name; Java uses one
		// block whose level encodes both (0=source, 8-15=falling).
		base = strings.TrimPrefix(base, "flowing_")
		name = "minecraft:" + base
		putInt(out, "level", props, "liquid_depth")
		return finish(name, out)
	case base == "seagrass":
		switch str(props["sea_grass_type"]) {
		case "double_bot":
			return finish("minecraft:tall_seagrass", map[string]string{"half": "lower"})
		case "double_top":
			return finish("minecraft:tall_seagrass", map[string]string{"half": "upper"})
		default:
			return finish("minecraft:seagrass", out)
		}
	case strings.HasSuffix(base, "_double_slab"):
		base = strings.TrimSuffix(base, "_double_slab") + "_slab"
		name = "minecraft:" + base
		out["type"] = "double"
		out["waterlogged"] = "false"
		return finish(name, out)
	case base == "torch":
		if facing, isWall := wallTorchFacing(props); isWall {
			return finish("minecraft:wall_torch", map[string]string{"facing": facing})
		}
		return finish(name, out)
	case base == "redstone_torch":
		if facing, isWall := wallTorchFacing(props); isWall {
			return finish("minecraft:redstone_wall_torch", map[string]string{"facing": facing, "lit": "true"})
		}
		out["lit"] = "true"
		return finish(name, out)
	case base == "soul_torch":
		if facing, isWall := wallTorchFacing(props); isWall {
			return finish("minecraft:soul_wall_torch", map[string]string{"facing": facing})
		}
		return finish(name, out)
	case base == "copper_torch":
		if facing, isWall := wallTorchFacing(props); isWall {
			return finish("minecraft:copper_wall_torch", map[string]string{"facing": facing})
		}
		return finish(name, out)
	case base == "unlit_redstone_torch":
		// Bedrock splits the lit flag into the block name; Java keeps one
		// block with a lit property.
		if facing, isWall := wallTorchFacing(props); isWall {
			return finish("minecraft:redstone_wall_torch", map[string]string{"facing": facing, "lit": "false"})
		}
		out["lit"] = "false"
		return finish("minecraft:redstone_torch", out)
	}

	// Per-block and per-property-signature derivation.
	derived := false
	switch {
	case strings.HasSuffix(base, "_leaves") || base == "leaves" || base == "leaves2" || base == "azalea_leaves":
		if v, present := get("persistent_bit"); present {
			out["persistent"] = boolStr(v)
			derived = true
		}
		// update_bit has no Java equivalent; dropped.
	case base == "snow":
		if v, present := get("height"); present {
			if h, isInt := intVal(v); isInt && h >= 0 && h <= 7 {
				out["layers"] = strconv.Itoa(int(h) + 1)
				derived = true
			}
		}
		// covered_bit has no Java equivalent; dropped.
	case strings.HasSuffix(base, "_stairs"):
		if v, present := get("weirdo_direction"); present {
			if facing, isDir := bedrockFacingByWeirdo[int32Val(v)]; isDir {
				out["facing"] = facing
				derived = true
			}
		}
		if v, present := get("upside_down_bit"); present {
			if truthy(v) {
				out["half"] = "top"
			} else {
				out["half"] = "bottom"
			}
			derived = true
		}
		if v, present := get("minecraft:corner"); present {
			if s := str(v); s == "none" {
				out["shape"] = "straight"
			} else {
				out["shape"] = s
			}
			derived = true
		}
		out["waterlogged"] = "false"
	case strings.HasSuffix(base, "_slab"):
		if v, present := get("minecraft:vertical_half"); present {
			out["type"] = str(v)
			derived = true
		}
		out["waterlogged"] = "false"
	case base == "water" || base == "lava":
		putInt(out, "level", props, "liquid_depth")
		derived = true
	case strings.HasSuffix(base, "_fence"):
		// Bedrock stores the four connection booleans explicitly (all
		// keys always present); Java has the same four keys. Copy the
		// values through — presence alone does not mean connected.
		for _, dir := range []string{"east", "north", "south", "west"} {
			out[dir] = "false"
			if v, present := get("minecraft:connection_" + dir); present {
				out[dir] = boolStr(v)
			}
		}
		out["waterlogged"] = "false"
		derived = true
	case base == "vine":
		if v, present := get("vine_direction_bits"); present {
			bits := int32Val(v)
			out["south"] = strconv.FormatBool(bits&1 != 0)
			out["west"] = strconv.FormatBool(bits&2 != 0)
			out["north"] = strconv.FormatBool(bits&4 != 0)
			out["east"] = strconv.FormatBool(bits&8 != 0)
			derived = true
		}
	case base == "rail":
		if v, present := get("rail_direction"); present {
			if shape, isShape := railShape(int32Val(v)); isShape {
				out["shape"] = shape
				derived = true
			}
		}
	case base == "kelp":
		putInt(out, "age", props, "kelp_age")
		derived = true
	case base == "kelp_plant":
		derived = true // no properties either side
	case strings.HasSuffix(base, "_wall_sign") || strings.HasSuffix(base, "_sign"):
		out["waterlogged"] = "false"
		derived = true
	}

	// Direct key translations (pillar_axis -> axis) and string direction
	// keys apply on top of whatever was derived above. Compass directions
	// agree between editions (see the facing-convention note), so values
	// pass through verbatim.
	spec := propSpecs["default"]
	for javaKey, bedrockKey := range spec.direct {
		if v, present := get(bedrockKey); present {
			out[javaKey] = str(v)
			derived = true
		}
	}
	for _, key := range []string{"facing_direction", "minecraft:cardinal_direction"} {
		if v, present := get(key); present {
			if s := str(v); isCompass(s) {
				out["facing"] = s
				derived = true
			}
		}
	}

	// Synthesized defaults (distance, waterlogged, ...) fill the gaps.
	for key, val := range javaDefaultsFor(base) {
		if _, present := out[key]; !present {
			out[key] = val
		}
	}

	// Bedrock properties that have no Java counterpart and were not
	// consumed above are dropped. But if Bedrock carried meaningful
	// properties and nothing at all was translated or synthesized, the
	// property shape is unknown: refuse rather than silently default the
	// block. Bedrock-only noise keys (infiniburn_bit and friends) do not
	// count as meaningful.
	if !derived && len(out) == 0 && hasMeaningfulProps(name, props) {
		return name, nil, false
	}
	return finish(name, out)
}

func finish(name string, out map[string]string) (string, map[string]string, bool) {
	base := strings.TrimPrefix(name, "minecraft:")
	for key, val := range javaDefaultsFor(base) {
		if _, present := out[key]; !present {
			out[key] = val
		}
	}
	return name, out, true
}

// wallTorchFacing extracts the Java wall-torch facing from a Bedrock
// torch state. dragonfly encodes floor torches with
// torch_facing_direction "top" (or "unknown" for the placement-less
// state) and wall torches with the compass direction of the room the
// torch faces into — identical to Java's wall_torch facing.
func wallTorchFacing(props map[string]any) (string, bool) {
	switch s := str(props["torch_facing_direction"]); s {
	case "east", "west", "north", "south":
		return s, true
	default:
		return "", false
	}
}

func isCompass(s string) bool {
	switch s {
	case "north", "south", "east", "west":
		return true
	}
	return false
}

// bedrockOnlyKeys are Bedrock properties with no Java counterpart that
// may appear on otherwise property-less blocks. They are dropped, and
// they never make a state "meaningful" for refusal purposes.
var bedrockOnlyKeys = map[string]bool{
	"infiniburn_bit": true,
}

// hasMeaningfulProps reports whether the Bedrock property set carries
// anything beyond bedrockOnlyKeys noise.
func hasMeaningfulProps(name string, props map[string]any) bool {
	for key := range props {
		if !bedrockOnlyKeys[key] {
			return true
		}
	}
	return false
}

func javaDefaultsFor(base string) map[string]string {
	if strings.HasSuffix(base, "_leaves") {
		if base == "mangrove_leaves" {
			return javaDefaults["mangrove_leaves"]
		}
		return javaDefaults["leaves"]
	}
	return javaDefaults[base]
}

// railShape maps Bedrock's rail_direction (activation-rail encoding, also
// used by dragonfly for plain rails) to Java's rail shape names.
func railShape(d int32) (string, bool) {
	switch d {
	case 0:
		return "north_south", true
	case 1:
		return "east_west", true
	case 2:
		return "ascending_east", true
	case 3:
		return "ascending_west", true
	case 4:
		return "ascending_north", true
	case 5:
		return "ascending_south", true
	case 6:
		return "south_east", true
	case 7:
		return "south_west", true
	case 8:
		return "north_west", true
	case 9:
		return "north_east", true
	}
	return "", false
}

func putInt(out map[string]string, javaKey string, props map[string]any, bedrockKey string) {
	if v, present := props[bedrockKey]; present {
		if n, isInt := intVal(v); isInt {
			out[javaKey] = strconv.Itoa(int(n))
		}
	}
}

func str(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case int32:
		return strconv.Itoa(int(t))
	case int:
		return strconv.Itoa(t)
	case uint8:
		return strconv.Itoa(int(t))
	default:
		return ""
	}
}

func boolStr(v any) string { return strconv.FormatBool(truthy(v)) }

func truthy(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case int32:
		return t != 0
	case int:
		return t != 0
	case uint8:
		return t != 0
	case string:
		return t == "true"
	}
	return false
}

func intVal(v any) (int64, bool) {
	switch t := v.(type) {
	case int32:
		return int64(t), true
	case int:
		return int64(t), true
	case uint8:
		return int64(t), true
	case int64:
		return t, true
	}
	return 0, false
}

func int32Val(v any) int32 {
	n, _ := intVal(v)
	return int32(n)
}
