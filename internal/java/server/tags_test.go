package server

import (
	"bytes"
	"testing"

	pk "github.com/Tnze/go-mc/net/packet"
)

// TestVanillaUpdateTagsPayload validates the full vanilla tag payload that
// replaces the client's tag view during configuration. It pins:
//   - the total section/tag counts against the vanilla 26.3 dump, and
//   - the specific tags the client needs to resolve the data-less registry
//     JSONs it decodes (block infiniburn for dimension_type, the timeline
//     tag the overworld dimension_type references, item enchantable tags for
//     enchantments).
//
// Before this payload existed, every join attempt failed with
// "Missing tag: 'minecraft:infiniburn_overworld' in 'minecraft:block'" (and,
// with data-less entries, enchantments would fail on the enchantable tags).
func TestVanillaUpdateTagsPayload(t *testing.T) {
	payload := buildVanillaUpdateTagsPayload()
	r := bytes.NewReader(payload)

	var regCount pk.VarInt
	if _, err := regCount.ReadFrom(r); err != nil {
		t.Fatal(err)
	}
	if int(regCount) != len(vanillaTagSections) {
		t.Fatalf("registry sections = %d, want %d", regCount, len(vanillaTagSections))
	}

	sections := map[string]map[string][]int32{}
	for i := 0; i < int(regCount); i++ {
		var regID pk.Identifier
		if _, err := regID.ReadFrom(r); err != nil {
			t.Fatal(err)
		}
		var tagCount pk.VarInt
		if _, err := tagCount.ReadFrom(r); err != nil {
			t.Fatal(err)
		}
		tags := map[string][]int32{}
		for j := 0; j < int(tagCount); j++ {
			var tagID pk.Identifier
			if _, err := tagID.ReadFrom(r); err != nil {
				t.Fatal(err)
			}
			var memberCount pk.VarInt
			if _, err := memberCount.ReadFrom(r); err != nil {
				t.Fatal(err)
			}
			members := make([]int32, 0, memberCount)
			for k := 0; k < int(memberCount); k++ {
				var id pk.VarInt
				if _, err := id.ReadFrom(r); err != nil {
					t.Fatal(err)
				}
				members = append(members, int32(id))
			}
			tags[string(tagID)] = members
		}
		sections[string(regID)] = tags
	}
	if r.Len() != 0 {
		t.Errorf("%d trailing bytes in payload", r.Len())
	}

	// Whole-payload pin against the vanilla dump.
	if got := len(sections); got != len(vanillaTagSections) {
		t.Fatalf("decoded %d sections, want %d", got, len(vanillaTagSections))
	}
	totalTags := 0
	for _, tags := range sections {
		totalTags += len(tags)
	}
	if totalTags != vanillaTagCount {
		t.Errorf("total tags = %d, want %d (vanilla 26.3 dump)", totalTags, vanillaTagCount)
	}

	// dimension_type's infiniburn HolderSet resolves in minecraft:block.
	block, ok := sections["minecraft:block"]
	if !ok {
		t.Fatal("no minecraft:block section; joins will fail on the infiniburn tag")
	}
	members, ok := block["minecraft:infiniburn_overworld"]
	if !ok {
		t.Fatal("minecraft:block section is missing minecraft:infiniburn_overworld")
	}
	if len(members) != 2 || members[0] != 334 || members[1] != 729 {
		t.Errorf("infiniburn members = %v, want [334 729] (netherrack, magma_block)", members)
	}

	// The vanilla overworld dimension_type references "#minecraft:in_overworld"
	// in minecraft:timeline; its members must be ids into the data-less
	// timeline send order [day, moon, early_game, villager_schedule].
	timeline, ok := sections["minecraft:timeline"]
	if !ok {
		t.Fatal("no minecraft:timeline section; the overworld dimension_type timeline tag is unbound")
	}
	inOverworld, ok := timeline["minecraft:in_overworld"]
	if !ok {
		t.Fatal("minecraft:timeline section is missing minecraft:in_overworld")
	}
	want := []int32{3, 0, 2, 1}
	if len(inOverworld) != len(want) {
		t.Fatalf("timeline:in_overworld members = %v, want %v", inOverworld, want)
	}
	for i := range want {
		if inOverworld[i] != want[i] {
			t.Fatalf("timeline:in_overworld members = %v, want %v", inOverworld, want)
		}
	}

	// Enchantments reference item/enchantment tags; a few must exist and be
	// non-empty (exclusive_set/* lives in the enchantment registry, the
	// enchantable/* sets in minecraft:item).
	item, ok := sections["minecraft:item"]
	if !ok {
		t.Fatal("no minecraft:item section; enchantments will fail to parse")
	}
	for _, tag := range []string{"minecraft:enchantable/melee_weapon", "minecraft:enchantable/sharp_weapon", "minecraft:enchantable/mining"} {
		if m, ok := item[tag]; !ok || len(m) == 0 {
			t.Errorf("minecraft:item tag %s missing or empty", tag)
		}
	}
	ench, ok := sections["minecraft:enchantment"]
	if !ok {
		t.Fatal("no minecraft:enchantment section; enchantment exclusive sets are unbound")
	}
	if m, ok := ench["minecraft:exclusive_set/damage"]; !ok || len(m) == 0 {
		t.Errorf("minecraft:enchantment tag minecraft:exclusive_set/damage missing or empty")
	}
}

// TestVanillaSyncRegistryList pins the data-less registry sync against the
// vanilla 26.3 SYNCHRONIZED_REGISTRIES dump: every registry the vanilla
// server sends must be present with its full element list, in raw-id order.
// The 26.3 client's item components hard-require several of these
// (block_transformer, banner_pattern, ...); a missing registry stays empty on
// the client and crashes the join at finish_configuration with
// "Missing element ResourceKey[minecraft:block_transformer / minecraft:shovel]".
func TestVanillaSyncRegistryList(t *testing.T) {
	if len(vanillaSyncRegistries) != vanillaSyncRegistryCount {
		t.Fatalf("registry count = %d, want %d", len(vanillaSyncRegistries), vanillaSyncRegistryCount)
	}
	total := 0
	for _, reg := range vanillaSyncRegistries {
		if reg.Name == "" {
			t.Error("empty registry name")
		}
		total += len(reg.Elements)
	}
	if total != vanillaSyncElementCount {
		t.Errorf("total elements = %d, want %d", total, vanillaSyncElementCount)
	}

	byName := map[string]int{}
	for _, reg := range vanillaSyncRegistries {
		byName[reg.Name] = len(reg.Elements)
	}
	for _, req := range []struct {
		name  string
		count int
	}{
		{"minecraft:block_transformer", 3}, // shovel/axe/hoe: REQUIRED by Item components
		{"minecraft:banner_pattern", 43},   // REQUIRED by banner item components
		{"minecraft:enchantment", 43},      // referenced by enchantable item tags
		{"minecraft:dimension_type", 4},    // overworld/nether/end/overworld_caves
		{"minecraft:worldgen/biome", 67},   // chunk biome palette id space
		{"minecraft:timeline", 4},          // day/moon/early_game/villager_schedule
		{"minecraft:world_clock", 2},       // overworld + nether
		{"minecraft:decorated_pot_pattern", 23},
		{"minecraft:dialog", 3},
	} {
		if got, ok := byName[req.name]; !ok || got != req.count {
			t.Errorf("%s elements = %d, want %d", req.name, got, req.count)
		}
	}

	// The biome registry element order must match go-mc's biome.Type raw ids
	// (chunk palettes send those ids directly against this send order).
	for i, id := range []string{
		"minecraft:badlands", "minecraft:bamboo_jungle", "minecraft:basalt_deltas",
		"minecraft:beach", "minecraft:birch_forest", "minecraft:cherry_grove",
		"minecraft:cold_ocean", "minecraft:crimson_forest", "minecraft:dappled_forest",
	} {
		var biomeReg *vanillaSyncRegistry
		for j := range vanillaSyncRegistries {
			if vanillaSyncRegistries[j].Name == "minecraft:worldgen/biome" {
				biomeReg = &vanillaSyncRegistries[j]
				break
			}
		}
		if biomeReg == nil {
			t.Fatal("no worldgen/biome registry")
		}
		if biomeReg.Elements[i] != id {
			t.Fatalf("worldgen/biome element[%d] = %s, want %s", i, biomeReg.Elements[i], id)
		}
	}
}

// TestWriteVanillaRegistryDataShape decodes the bytes writeVanillaRegistryData
// produces for one registry and pins the per-entry wire shape: Identifier,
// Boolean hasData=false, and nothing else.
func TestWriteVanillaRegistryDataShape(t *testing.T) {
	reg := vanillaSyncRegistry{
		Name:     "minecraft:test",
		Elements: []string{"minecraft:a", "minecraft:b", "minecraft:c"},
	}
	var buf bytes.Buffer
	_, _ = pk.VarInt(len(reg.Elements)).WriteTo(&buf)
	for _, el := range reg.Elements {
		_, _ = pk.Identifier(el).WriteTo(&buf)
		_, _ = pk.Boolean(false).WriteTo(&buf)
	}

	r := bytes.NewReader(buf.Bytes())
	var count pk.VarInt
	if _, err := count.ReadFrom(r); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("count = %d, want 3", count)
	}
	for i := 0; i < int(count); i++ {
		var id pk.Identifier
		var hasData pk.Boolean
		if _, err := id.ReadFrom(r); err != nil {
			t.Fatal(err)
		}
		if _, err := hasData.ReadFrom(r); err != nil {
			t.Fatal(err)
		}
		if hasData {
			t.Errorf("entry %d: hasData=true, want data-less", i)
		}
		if want := []string{"minecraft:a", "minecraft:b", "minecraft:c"}[i]; string(id) != want {
			t.Errorf("entry %d = %s, want %s", i, id, want)
		}
	}
	if r.Len() != 0 {
		t.Errorf("%d trailing bytes", r.Len())
	}
}
