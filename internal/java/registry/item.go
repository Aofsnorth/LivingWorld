package registry

import (
	"github.com/Tnze/go-mc/nbt"
	mcregistry "github.com/Tnze/go-mc/registry"
)

// textComponent is the NBT form of a Minecraft text Component, i.e. the
// object form of "translate"/"color" rather than a bare string.
//
// Source: 26.3-server.jar/data/minecraft/trim_material/amethyst.json
type textComponent struct {
	Translate string `nbt:"translate"`
	Color     string `nbt:"color"`
}

// trimMaterial is the 26.3 shape. It is NOT the 1.21 shape: the old
// asset_name / ingredient / item_model_index triple is gone, replaced by a
// palette_id, and description became a full text Component instead of a
// plain string.
//
// Sending the old shape made every 26.3 client abort the whole registry sync
// with "Failed to parse value {asset_name:..., item_model_index:0.0f, ...}
// for key minecraft:amethyst from server".
//
// Source: 26.3-server.jar/data/minecraft/trim_material/*.json
type trimMaterial struct {
	Description textComponent `nbt:"description"`
	PaletteID   string        `nbt:"palette_id"`
}

type trimPattern struct {
	AssetID      string `nbt:"asset_id"`
	TemplateItem string `nbt:"template_item"`
	Description  string `nbt:"description"`
	Decal        bool   `nbt:"decal"`
}

type jukeboxSong struct {
	SoundEvent       string  `nbt:"sound_event"`
	Description      string  `nbt:"description"`
	LengthInSeconds  float32 `nbt:"length_in_seconds"`
	ComparatorOutput int32   `nbt:"comparator_output"`
}

type instrument struct {
	SoundEvent  string  `nbt:"sound_event"`
	Range       float32 `nbt:"range"`
	UseDuration float32 `nbt:"use_duration"`
	Description string  `nbt:"description"`
}

// trimMaterialEntry pairs a registry key with its 26.3 value.
type trimMaterialEntry struct {
	key string
	val trimMaterial
}

// vanillaTrimMaterials is transcribed verbatim from
// 26.3-server.jar/data/minecraft/trim_material/*.json.
//
// The pre-26.3 shape (asset_name / ingredient / item_model_index, with a
// bare-string description) made every 26.3 client abort the whole registry
// sync, so the table lives at package scope and is asserted by tests.
var vanillaTrimMaterials = []trimMaterialEntry{
	{"minecraft:amethyst", trimMaterial{textComponent{"trim_material.minecraft.amethyst", "#9A5CC6"}, "minecraft:trim/amethyst"}},
	{"minecraft:copper", trimMaterial{textComponent{"trim_material.minecraft.copper", "#B4684D"}, "minecraft:trim/copper"}},
	{"minecraft:diamond", trimMaterial{textComponent{"trim_material.minecraft.diamond", "#6EECD2"}, "minecraft:trim/diamond"}},
	{"minecraft:emerald", trimMaterial{textComponent{"trim_material.minecraft.emerald", "#11A036"}, "minecraft:trim/emerald"}},
	{"minecraft:gold", trimMaterial{textComponent{"trim_material.minecraft.gold", "#DEB12D"}, "minecraft:trim/gold"}},
	{"minecraft:iron", trimMaterial{textComponent{"trim_material.minecraft.iron", "#ECECEC"}, "minecraft:trim/iron"}},
	{"minecraft:lapis", trimMaterial{textComponent{"trim_material.minecraft.lapis", "#416E97"}, "minecraft:trim/lapis"}},
	{"minecraft:netherite", trimMaterial{textComponent{"trim_material.minecraft.netherite", "#625859"}, "minecraft:trim/netherite"}},
	{"minecraft:quartz", trimMaterial{textComponent{"trim_material.minecraft.quartz", "#E3D4C4"}, "minecraft:trim/quartz"}},
	{"minecraft:redstone", trimMaterial{textComponent{"trim_material.minecraft.redstone", "#971607"}, "minecraft:trim/redstone"}},
	{"minecraft:resin", trimMaterial{textComponent{"trim_material.minecraft.resin", "#FC7812"}, "minecraft:trim/resin"}},
}

func registerTrimData(r *mcregistry.Registries, sizes map[string]int) {
	for _, tm := range vanillaTrimMaterials {
		r.Registry("minecraft:trim_material").(*mcregistry.Registry[nbt.RawMessage]).Put(tm.key, marshalNBT(tm.val))
	}
	sizes["minecraft:trim_material"] = len(vanillaTrimMaterials)

	trimPatterns := []struct {
		key, asset, template, desc string
	}{
		{"minecraft:bolt", "minecraft:bolt", "minecraft:bolt_armor_trim_smithing_template", "trim_pattern.minecraft.bolt"},
		{"minecraft:coast", "minecraft:coast", "minecraft:coast_armor_trim_smithing_template", "trim_pattern.minecraft.coast"},
		{"minecraft:dune", "minecraft:dune", "minecraft:dune_armor_trim_smithing_template", "trim_pattern.minecraft.dune"},
		{"minecraft:eye", "minecraft:eye", "minecraft:eye_armor_trim_smithing_template", "trim_pattern.minecraft.eye"},
		{"minecraft:host", "minecraft:host", "minecraft:host_armor_trim_smithing_template", "trim_pattern.minecraft.host"},
		{"minecraft:raiser", "minecraft:raiser", "minecraft:raiser_armor_trim_smithing_template", "trim_pattern.minecraft.raiser"},
		{"minecraft:rib", "minecraft:rib", "minecraft:rib_armor_trim_smithing_template", "trim_pattern.minecraft.rib"},
		{"minecraft:sentry", "minecraft:sentry", "minecraft:sentry_armor_trim_smithing_template", "trim_pattern.minecraft.sentry"},
		{"minecraft:shaper", "minecraft:shaper", "minecraft:shaper_armor_trim_smithing_template", "trim_pattern.minecraft.shaper"},
		{"minecraft:silence", "minecraft:silence", "minecraft:silence_armor_trim_smithing_template", "trim_pattern.minecraft.silence"},
		{"minecraft:snout", "minecraft:snout", "minecraft:snout_armor_trim_smithing_template", "trim_pattern.minecraft.snout"},
		{"minecraft:spire", "minecraft:spire", "minecraft:spire_armor_trim_smithing_template", "trim_pattern.minecraft.spire"},
		{"minecraft:tide", "minecraft:tide", "minecraft:tide_armor_trim_smithing_template", "trim_pattern.minecraft.tide"},
		{"minecraft:vex", "minecraft:vex", "minecraft:vex_armor_trim_smithing_template", "trim_pattern.minecraft.vex"},
		{"minecraft:ward", "minecraft:ward", "minecraft:ward_armor_trim_smithing_template", "trim_pattern.minecraft.ward"},
		{"minecraft:wayfinder", "minecraft:wayfinder", "minecraft:wayfinder_armor_trim_smithing_template", "trim_pattern.minecraft.wayfinder"},
		{"minecraft:wild", "minecraft:wild", "minecraft:wild_armor_trim_smithing_template", "trim_pattern.minecraft.wild"},
	}
	for _, tp := range trimPatterns {
		r.Registry("minecraft:trim_pattern").(*mcregistry.Registry[nbt.RawMessage]).Put(tp.key, marshalNBT(trimPattern{
			AssetID: tp.asset, TemplateItem: tp.template, Description: tp.desc, Decal: false,
		}))
	}
	sizes["minecraft:trim_pattern"] = len(trimPatterns)
}

func registerEnchantments(_ *mcregistry.Registries, sizes map[string]int) {
	sizes["minecraft:enchantment"] = 0
}

func registerJukeboxSongs(r *mcregistry.Registries, sizes map[string]int) {
	songs := []string{
		"13", "cat", "blocks", "chirp", "far", "mall", "mellohi", "stal", "strad", "ward",
		"11", "wait", "pigstep", "otherside", "5", "relic", "precipice", "creator",
		"creator_music_box", "tears", "lava_chicken",
	}
	for _, song := range songs {
		r.JukeboxSong.Put("minecraft:"+song, marshalNBT(jukeboxSong{
			SoundEvent:       "minecraft:music_disc." + song,
			Description:      "{\"translate\":\"jukebox_song.minecraft." + song + "\"}",
			LengthInSeconds:  180.0,
			ComparatorOutput: 1,
		}))
	}
	sizes["minecraft:jukebox_song"] = len(songs)
}

func registerInstruments(r *mcregistry.Registries, sizes map[string]int) {
	instruments := []struct {
		key, sound string
	}{
		{"ponder_goat_horn", "item.goat_horn.sound.0"},
		{"sing_goat_horn", "item.goat_horn.sound.1"},
		{"seek_goat_horn", "item.goat_horn.sound.2"},
		{"feel_goat_horn", "item.goat_horn.sound.3"},
		{"admire_goat_horn", "item.goat_horn.sound.4"},
		{"call_goat_horn", "item.goat_horn.sound.5"},
		{"yearn_goat_horn", "item.goat_horn.sound.6"},
		{"dream_goat_horn", "item.goat_horn.sound.7"},
	}
	for _, inst := range instruments {
		r.Registry("minecraft:instrument").(*mcregistry.Registry[nbt.RawMessage]).Put("minecraft:"+inst.key, marshalNBT(instrument{
			SoundEvent: "minecraft:" + inst.sound, Range: 256.0, UseDuration: 7.0,
			Description: "{\"translate\":\"instrument.minecraft." + inst.key + "\"}",
		}))
	}
	sizes["minecraft:instrument"] = len(instruments)
}
