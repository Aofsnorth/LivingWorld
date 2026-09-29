package server

import (
	"encoding/json"
	"testing"
)

// loadTimeline decodes a bundled timeline element back out of the embedded FS.
func loadTimeline(t *testing.T, name string) map[string]any {
	t.Helper()
	raw, err := timelineFS.ReadFile("registrydata/timeline/" + name + ".json")
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	return m
}

// 26.3 changed the colour tracks of minecraft:visual/cloud_color from a packed
// int to a hex string. A stale copy fails the client's timeline decode, which
// leaves minecraft:timeline unbound and makes dimension_type's "timelines"
// reference unresolvable — the client then reports
//
//	minecraft:dimension_type/minecraft:overworld: Failed to parse value {...}
//
// This test pins the 26.3 shape so the bundled copies get re-extracted when
// the target version moves.
func TestTimelineColorTracksAreHexStrings(t *testing.T) {
	for _, name := range []string{"day", "early_game", "moon", "villager_schedule"} {
		t.Run(name, func(t *testing.T) {
			m := loadTimeline(t, name)
			tracks, ok := m["tracks"].(map[string]any)
			if !ok {
				t.Fatal("no tracks map")
			}
			for _, track := range []string{
				"minecraft:visual/cloud_color",
				"minecraft:visual/sky_color",
				"minecraft:visual/fog_color",
				"minecraft:visual/sky_light_color",
			} {
				tr, ok := tracks[track].(map[string]any)
				if !ok {
					continue // track legitimately absent from this timeline
				}
				frames, ok := tr["keyframes"].([]any)
				if !ok || len(frames) == 0 {
					continue
				}
				for i, f := range frames {
					frame := f.(map[string]any)
					switch v := frame["value"].(type) {
					case string:
						if len(v) > 0 && v[0] != '#' {
							t.Errorf("%s.%s keyframe %d: %q is not a #rrggbb colour", track, "value", i, v)
						}
					default:
						t.Errorf("%s keyframe %d: value is %T (%v), want a hex string; "+
							"26.3 changed colour tracks from packed int to hex string",
							track, i, frame["value"], frame["value"])
					}
				}
			}
		})
	}
}

// Every timeline the dimension_type references must be present in the embedded
// set, or the client cannot resolve the HolderSet. The ids mirror
// internal/java/registry.dimensionType26_1.Timelines.
func TestAllReferencedTimelinesAreBundled(t *testing.T) {
	want := []string{
		"minecraft:day",
		"minecraft:moon",
		"minecraft:early_game",
		"minecraft:villager_schedule",
	}
	for _, id := range want {
		const prefix = "minecraft:"
		name := id[len(prefix):]
		if _, err := timelineFS.ReadFile("registrydata/timeline/" + name + ".json"); err != nil {
			t.Errorf("dimension_type references %s but it is not bundled: %v", id, err)
		}
	}

	entries, err := timelineFS.ReadDir("registrydata/timeline")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(want) {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("bundled %d timeline files %v, want exactly %d", len(entries), names, len(want))
	}
}

// timelineNBT must produce a non-trivial nameless-root network NBT blob.
func TestTimelineNBTEncodes(t *testing.T) {
	for _, name := range []string{"day", "early_game", "moon", "villager_schedule"} {
		b, err := timelineNBT(name)
		if err != nil {
			t.Fatalf("timelineNBT(%s): %v", name, err)
		}
		if len(b) < 32 {
			t.Errorf("timelineNBT(%s) = %d bytes, suspiciously small", name, len(b))
		}
		if b[0] != 0x0A {
			t.Errorf("timelineNBT(%s) root tag = 0x%02X, want 0x0A (TAG_Compound)", name, b[0])
		}
	}
}
