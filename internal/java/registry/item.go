package registry

import (
	"github.com/Tnze/go-mc/nbt"
	mcregistry "github.com/Tnze/go-mc/registry"
)

type trimMaterial struct {
	AssetName      string  `nbt:"asset_name"`
	Ingredient     string  `nbt:"ingredient"`
	ItemModelIndex float32 `nbt:"item_model_index"`
	Description    string  `nbt:"description"`
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

func registerTrimData(r *mcregistry.Registries, sizes map[string]int) {
	trimMaterials := []struct {
		key, asset, item, desc string
		idx                    float32
	}{
		{"minecraft:amethyst", "amethyst", "minecraft:amethyst_shard", "trim_material.minecraft.amethyst", 0.0},
		{"minecraft:copper", "copper", "minecraft:copper_ingot", "trim_material.minecraft.copper", 0.0},
		{"minecraft:diamond", "diamond", "minecraft:diamond", "trim_material.minecraft.diamond", 0.0},
		{"minecraft:emerald", "emerald", "minecraft:emerald", "trim_material.minecraft.emerald", 0.0},
		{"minecraft:gold", "gold", "minecraft:gold_ingot", "trim_material.minecraft.gold", 0.0},
		{"minecraft:iron", "iron", "minecraft:iron_ingot", "trim_material.minecraft.iron", 0.0},
		{"minecraft:lapis", "lapis", "minecraft:lapis_lazuli", "trim_material.minecraft.lapis", 0.0},
		{"minecraft:netherite", "netherite", "minecraft:netherite_ingot", "trim_material.minecraft.netherite", 0.0},
		{"minecraft:quartz", "quartz", "minecraft:quartz", "trim_material.minecraft.quartz", 0.0},
		{"minecraft:redstone", "redstone", "minecraft:redstone", "trim_material.minecraft.redstone", 0.0},
		{"minecraft:resin", "resin", "minecraft:resin_brick", "trim_material.minecraft.resin", 0.0},
	}
	for _, tm := range trimMaterials {
		r.Registry("minecraft:trim_material").(*mcregistry.Registry[nbt.RawMessage]).Put(tm.key, marshalNBT(trimMaterial{
			AssetName: tm.asset, Ingredient: tm.item, ItemModelIndex: tm.idx, Description: tm.desc,
		}))
	}
	sizes["minecraft:trim_material"] = len(trimMaterials)

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
