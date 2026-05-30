package registry

import (
	mcregistry "github.com/Tnze/go-mc/registry"
)

// 26.1 dimension type fields — completely different from 1.21.
// Fields like effects, ultrawarm, natural, bed_works, respawn_anchor_works,
// piglin_safe, has_raids are ALL GONE. They were moved into the "attributes"
// dispatched map keyed by ResourceLocation.
//
// Source: 26.1.2.jar/data/minecraft/dimension_type/overworld.json
type dimensionType26_1 struct {
	HasSkylight                 bool    `nbt:"has_skylight"`
	HasCeiling                  bool    `nbt:"has_ceiling"`
	HasEnderDragonFight         bool    `nbt:"has_ender_dragon_fight"`
	CoordinateScale             float64 `nbt:"coordinate_scale"`
	Height                      int32   `nbt:"height"`
	LogicalHeight               int32   `nbt:"logical_height"`
	MinY                        int32   `nbt:"min_y"`
	AmbientLight                float32 `nbt:"ambient_light"`
	InfiniteBurn                string  `nbt:"infiniburn"`
	MonsterSpawnLightLevel      int32   `nbt:"monster_spawn_light_level"`
	MonsterSpawnBlockLightLimit int32   `nbt:"monster_spawn_block_light_limit"`
	Skybox                      string  `nbt:"skybox,omitempty"`
	// 26.1 added these two. Without default_clock the dimension is not bound to
	// any world_clock, and without timelines the client has no sun_angle/moon_angle
	// curve — the sun freezes overhead even when set_time advances.
	//
	// timelines is a HolderSet<Timeline>. We encode it as an INLINE LIST of
	// element ids (an NBT list of strings) rather than a tag string
	// ("#minecraft:in_overworld"): a tag would require us to also serialize the
	// timeline tag registry, whereas a direct list only needs the referenced
	// timeline elements to exist (we send them data-less in config.go, so the
	// client fills them from its built-in "core" pack). default_clock is a
	// Holder<WorldClock> serialized as a plain id string.
	// Source: 26.1.2.jar/data/minecraft/dimension_type/overworld.json
	Timelines    []string       `nbt:"timelines"`
	DefaultClock string         `nbt:"default_clock"`
	Attributes   map[string]any `nbt:"attributes"`
}

// buildOverworldAttributes builds the exact "attributes" dispatched map for
// the overworld, matching 26.1.2.jar's overworld.json.
func buildOverworldAttributes() map[string]any {
	return map[string]any{
		// Audio
		"minecraft:audio/ambient_sounds": map[string]any{
			"mood": map[string]any{
				"block_search_extent": int32(8),
				"offset":              float32(2.0),
				"sound":               "minecraft:ambient.cave",
				"tick_delay":          int32(6000),
			},
		},
		"minecraft:audio/background_music": map[string]any{
			"creative": map[string]any{
				"max_delay": int32(24000),
				"min_delay": int32(12000),
				"sound":     "minecraft:music.creative",
			},
			"default": map[string]any{
				"max_delay": int32(24000),
				"min_delay": int32(12000),
				"sound":     "minecraft:music.game",
			},
		},
		// Gameplay
		"minecraft:gameplay/bed_rule": map[string]any{
			"can_set_spawn": "always",
			"can_sleep":     "when_dark",
			"error_message": map[string]any{
				"translate": "block.minecraft.bed.no_sleep",
			},
		},
		"minecraft:gameplay/nether_portal_spawns_piglin": true,
		"minecraft:gameplay/respawn_anchor_works":        false,
		// Visual
		"minecraft:visual/ambient_light_color": "#0a0a0a",
		"minecraft:visual/cloud_color":         "#ccffffff",
		"minecraft:visual/cloud_height":        float32(192.33),
		"minecraft:visual/fog_color":           "#c0d8ff",
		"minecraft:visual/sky_color":           "#78a7ff",
	}
}

type biomeEffects struct {
	FogColor      int32 `nbt:"fog_color"`
	SkyColor      int32 `nbt:"sky_color"`
	WaterColor    int32 `nbt:"water_color"`
	WaterFogColor int32 `nbt:"water_fog_color"`
}

type plainsBiome struct {
	HasPrecipitation bool         `nbt:"has_precipitation"`
	Temperature      float32      `nbt:"temperature"`
	Downfall         float32      `nbt:"downfall"`
	Effects          biomeEffects `nbt:"effects"`
}

func registerDimension(r *mcregistry.Registries, sizes map[string]int) {
	dim := dimensionType26_1{
		HasSkylight:                 true,
		HasCeiling:                  false,
		HasEnderDragonFight:         false,
		CoordinateScale:             1.0,
		Height:                      384,
		LogicalHeight:               384,
		MinY:                        -64,
		AmbientLight:                0.0,
		InfiniteBurn:                "#minecraft:infiniburn_overworld",
		MonsterSpawnLightLevel:      7, // plain int; nether=7, end=15
		MonsterSpawnBlockLightLimit: 0,
		Skybox:                      "overworld",
		// Inline list of timeline element ids (matches client's #in_overworld tag
		// expansion: universal→villager_schedule, plus day/moon/early_game). All
		// four are sent data-less in config.go so the client uses its built-in data.
		Timelines:    []string{"minecraft:day", "minecraft:moon", "minecraft:early_game", "minecraft:villager_schedule"},
		DefaultClock: "minecraft:overworld",
		Attributes:   buildOverworldAttributes(),
	}
	r.DimensionType.Put("minecraft:overworld", marshalNBT(dim))
	sizes["minecraft:dimension_type"] = 1
}

func registerBiomes(r *mcregistry.Registries, sizes map[string]int) {
	plainsData := plainsBiome{
		HasPrecipitation: true,
		Temperature:      0.8,
		Downfall:         0.4,
		Effects: biomeEffects{
			FogColor:      12638463,
			SkyColor:      7907327,
			WaterColor:    4159204,
			WaterFogColor: 329011,
		},
	}
	r.WorldGenBiome.Put("minecraft:plains", marshalNBT(plainsData))
	sizes["minecraft:worldgen/biome"] = 1
}
