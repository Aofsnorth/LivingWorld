package blockconvtests

import (
	"reflect"
	"testing"

	"livingworld/internal/blockconv"
	lwworld "livingworld/internal/world"
)

// resolve runs the same two-step the adapter and importers run: translate
// the Bedrock state, then resolve the canonical Java state ID.
func resolve(t *testing.T, bedrockName string, props map[string]any) int32 {
	t.Helper()
	javaName, javaProps, ok := blockconv.Translate(bedrockName, props)
	if !ok {
		t.Fatalf("Translate(%s, %v) reported untranslatable", bedrockName, props)
	}
	id := lwworld.StateIDWithProperties(javaName, javaProps)
	if javaName != "minecraft:air" && id == lwworld.AirID {
		t.Fatalf("Translate(%s, %v) -> %s %v resolved to air", bedrockName, props, javaName, javaProps)
	}
	return id
}

// javaPropsOf returns the Java property set a canonical state carries, by
// decoding its name through the go-mc palette is not possible (names drop
// properties) — instead tests pin distinctness against other states.
func mustState(t *testing.T, name string, props map[string]string) int32 {
	t.Helper()
	id := lwworld.StateIDWithProperties(name, props)
	if id == lwworld.AirID {
		t.Fatalf("expected state %s %v to exist in Java palette", name, props)
	}
	return id
}

func TestPillarAxisPreserved(t *testing.T) {
	yAxis := resolve(t, "minecraft:deepslate", map[string]any{"pillar_axis": "y"})
	xAxis := resolve(t, "minecraft:deepslate", map[string]any{"pillar_axis": "x"})
	if want := mustState(t, "minecraft:deepslate", map[string]string{"axis": "y"}); yAxis != want {
		t.Fatalf("pillar_axis=y -> state %d, want %d (%s)", yAxis, want, lwworld.StateName(yAxis))
	}
	if yAxis == xAxis {
		t.Fatalf("pillar_axis x/y collapsed to the same state %d", yAxis)
	}
}

func TestStairsFacingMirrored(t *testing.T) {
	// Bedrock weirdo_direction 0 faces east in Bedrock's packing, which is
	// Java's west. Java stairs face the opposite of Bedrock's stored value.
	east := resolve(t, "minecraft:oak_stairs", map[string]any{
		"weirdo_direction": int32(0), "upside_down_bit": false, "minecraft:corner": "none",
	})
	want := mustState(t, "minecraft:oak_stairs", map[string]string{
		"facing": "west", "half": "bottom", "shape": "straight", "waterlogged": "false",
	})
	if east != want {
		t.Fatalf("oak_stairs weirdo=0 -> state %d, want %d", east, want)
	}
	top := resolve(t, "minecraft:oak_stairs", map[string]any{
		"weirdo_direction": int32(0), "upside_down_bit": true, "minecraft:corner": "inner_left",
	})
	if top == east {
		t.Fatal("upside_down_bit did not change the stair state")
	}
}

func TestSnowLayerThickness(t *testing.T) {
	one := resolve(t, "minecraft:snow_layer", map[string]any{"height": int32(0), "covered_bit": false})
	eight := resolve(t, "minecraft:snow_layer", map[string]any{"height": int32(7), "covered_bit": false})
	if want := mustState(t, "minecraft:snow", map[string]string{"layers": "1"}); one != want {
		t.Fatalf("snow_layer height=0 -> %d, want layers=1 state %d", one, want)
	}
	if want := mustState(t, "minecraft:snow", map[string]string{"layers": "8"}); eight != want {
		t.Fatalf("snow_layer height=7 -> %d, want layers=8 state %d", eight, want)
	}
}

func TestFluidLevels(t *testing.T) {
	source := resolve(t, "minecraft:water", map[string]any{"liquid_depth": int32(0)})
	falling := resolve(t, "minecraft:flowing_water", map[string]any{"liquid_depth": int32(8)})
	if want := mustState(t, "minecraft:water", map[string]string{"level": "0"}); source != want {
		t.Fatalf("water depth 0 -> %d, want %d", source, want)
	}
	if want := mustState(t, "minecraft:water", map[string]string{"level": "8"}); falling != want {
		t.Fatalf("flowing_water depth 8 -> %d, want %d", falling, want)
	}
}

func TestSeagrassSplit(t *testing.T) {
	single := resolve(t, "minecraft:seagrass", map[string]any{"sea_grass_type": "default"})
	lower := resolve(t, "minecraft:seagrass", map[string]any{"sea_grass_type": "double_bot"})
	upper := resolve(t, "minecraft:seagrass", map[string]any{"sea_grass_type": "double_top"})
	if single == lower || lower == upper || single == upper {
		t.Fatalf("seagrass variants collapsed: %d %d %d", single, lower, upper)
	}
	if want := mustState(t, "minecraft:tall_seagrass", map[string]string{"half": "lower"}); lower != want {
		t.Fatalf("double_bot -> %d, want tall_seagrass lower %d", lower, want)
	}
}

func TestLeavesDefaultsSynthesized(t *testing.T) {
	// Bedrock leaves carry persistent/update bits only; Java also requires
	// distance + waterlogged. distance=7 is the decay-frozen default the
	// canonical palette uses for generated leaves.
	id := resolve(t, "minecraft:oak_leaves", map[string]any{"persistent_bit": false, "update_bit": false})
	want := mustState(t, "minecraft:oak_leaves", map[string]string{
		"distance": "7", "persistent": "false", "waterlogged": "false",
	})
	if id != want {
		t.Fatalf("oak_leaves -> %d, want %d", id, want)
	}
}

func TestLegacyNameAliases(t *testing.T) {
	if id := resolve(t, "minecraft:magma", nil); id != lwworld.StateID("minecraft:magma_block") {
		t.Fatalf("magma -> %s, want magma_block", lwworld.StateName(id))
	}
	if id := resolve(t, "minecraft:web", nil); id != lwworld.StateID("minecraft:cobweb") {
		t.Fatalf("web -> %s, want cobweb", lwworld.StateName(id))
	}
	// The alias table must be exactly invertible (single source of truth).
	for bedrock, java := range blockconv.BedrockNameToJavaName {
		if back, ok := blockconv.JavaNameToBedrockName[java]; !ok || back != bedrock {
			t.Fatalf("alias %s->%s does not invert (got %q, %v)", bedrock, java, back, ok)
		}
	}
}

func TestUnknownPropertyShapeRefused(t *testing.T) {
	// A Bedrock state whose properties were not translated at all must be
	// reported untranslatable so callers fall back to the default state
	// instead of resolving a partial (zero-value) property set.
	_, _, ok := blockconv.Translate("minecraft:oak_stairs", map[string]any{"some_future_prop": "x"})
	if ok {
		t.Fatal("stairs with only unknown props translated successfully; want refusal")
	}
	// ...but a state with no Bedrock properties at all is fine (plain block).
	name, props, ok := blockconv.Translate("minecraft:stone", map[string]any{"infiniburn_bit": false})
	if !ok || name != "minecraft:stone" || len(props) != 0 {
		t.Fatalf("stone with bedrock-only prop -> (%s, %v, %v)", name, props, ok)
	}
}

func TestTranslateDoesNotMutateInput(t *testing.T) {
	props := map[string]any{"pillar_axis": "y"}
	before := map[string]any{"pillar_axis": "y"}
	if _, _, ok := blockconv.Translate("minecraft:oak_log", props); !ok {
		t.Fatal("oak_log pillar_axis did not translate")
	}
	if !reflect.DeepEqual(props, before) {
		t.Fatalf("Translate mutated its input: %v", props)
	}
}
