package registry

import (
	"strings"

	"github.com/Tnze/go-mc/level/biome"
	mcregistry "github.com/Tnze/go-mc/registry"
)

// intProviderUniform is the NBT form of the vanilla "minecraft:uniform"
// IntProvider codec. Since 26.3 monster_spawn_light_level is an IntProvider
// (DimensionType.MonsterSettings.monsterSpawnLightTest) rather than a bare
// int, so sending `7` made the whole dimension_type entry fail to parse.
//
// Source: 26.3-server.jar/data/minecraft/dimension_type/overworld.json
type intProviderUniform struct {
	Type         string `nbt:"type"`
	MinInclusive int32  `nbt:"min_inclusive"`
	MaxInclusive int32  `nbt:"max_inclusive"`
}

// 26.3 dimension type fields — completely different from 1.21.
// Fields like effects, ultrawarm, natural, bed_works, respawn_anchor_works,
// piglin_safe, has_raids are ALL GONE. They were moved into the "attributes"
// dispatched map keyed by ResourceLocation.
//
// Source: 26.3-server.jar/data/minecraft/dimension_type/overworld.json
type dimensionType26_1 struct {
	HasSkylight                 bool               `nbt:"has_skylight"`
	HasCeiling                  bool               `nbt:"has_ceiling"`
	HasEnderDragonFight         bool               `nbt:"has_ender_dragon_fight"`
	CoordinateScale             float64            `nbt:"coordinate_scale"`
	Height                      int32              `nbt:"height"`
	LogicalHeight               int32              `nbt:"logical_height"`
	MinY                        int32              `nbt:"min_y"`
	AmbientLight                float32            `nbt:"ambient_light"`
	InfiniteBurn                string             `nbt:"infiniburn"`
	MonsterSpawnLightLevel      intProviderUniform `nbt:"monster_spawn_light_level"`
	MonsterSpawnBlockLightLimit int32              `nbt:"monster_spawn_block_light_limit"`
	// NOTE: vanilla's NETWORK encoding omits skybox entirely (verified by
	// encoding vanilla's own overworld with DimensionType.NETWORK_CODEC via
	// tools/java/VanillaDump and diffing with tools/java/Diff). Sending it is
	// accepted by the codec but deviates from the wire format vanilla clients
	// are tested against, so it stays out.
	// 26.1 added these two. Without default_clock the dimension is not bound to
	// any world_clock, and without timelines the client has no sun_angle/moon_angle
	// curve — the sun freezes overhead even when set_time advances.
	//
	// timelines is a HolderSet<Timeline>. We encode it as an INLINE LIST of
	// element ids (an NBT list of strings) rather than a tag string
	// ("#minecraft:in_overworld"): a tag would require us to also serialize the
	// timeline tag registry, whereas a direct list only needs the referenced
	// timeline elements to exist (sent in config.go). Holder references never
	// fail during parse: MappedRegistry.createRegistrationLookup() returns
	// getOrCreateHolderOrThrow placeholders that bind when the timeline entries
	// register (verified from the 26.3 RegistryLoadTask bytecode). default_clock
	// is a Holder<WorldClock> serialized as a plain id string, exactly like
	// vanilla's own network encoding.
	// Source: 26.1.2.jar/data/minecraft/dimension_type/overworld.json, verified
	// against 26.3 DimensionType.NETWORK_CODEC via RegCheck + Diff.
	Timelines    []string       `nbt:"timelines"`
	DefaultClock string         `nbt:"default_clock"`
	Attributes   map[string]any `nbt:"attributes"`
}

// buildOverworldAttributes builds the exact "attributes" dispatched map for
// the overworld as vanilla ENCODES it over the network. The network form
// carries only the audio/visual attributes; the gameplay attributes
// (bed_rule, straw_bed_rule, respawn_anchor_works,
// nether_portal_spawns_piglin) exist in the datapack JSON but are NOT part of
// vanilla's DimensionType.NETWORK_CODEC output — the client applies its local
// defaults for them. Verified by diffing our encoding against vanilla's own
// (tools/java/Diff): these four keys are "ONLY-A" (ours-only) entries.
func buildOverworldAttributes() map[string]any {
	return map[string]any{
		// Audio
		"minecraft:audio/ambient_sounds": map[string]any{
			"mood": map[string]any{
				"block_search_extent": int32(8),
				// vanilla encodes mood.offset as a double (NBT tag 6), not a
				// float — the Diff flagged 2.0f vs 2.0d.
				"offset":     float64(2.0),
				"sound":      "minecraft:ambient.cave",
				"tick_delay": int32(6000),
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
		// Visual
		"minecraft:visual/ambient_light_color": "#0a0a0a",
		"minecraft:visual/cloud_color":         "#ccffffff",
		"minecraft:visual/cloud_height":        float32(192.33),
		"minecraft:visual/fog_color":           "#c0d8ff",
		"minecraft:visual/sky_color":           "#78a7ff",
	}
}

type biomeEffects struct {
	FogColor           int32  `nbt:"fog_color"`
	SkyColor           int32  `nbt:"sky_color"`
	WaterColor         int32  `nbt:"water_color"`
	WaterFogColor      int32  `nbt:"water_fog_color"`
	GrassColor         *int32 `nbt:"grass_color,omitempty"`
	FoliageColor       *int32 `nbt:"foliage_color,omitempty"`
	GrassColorModifier string `nbt:"grass_color_modifier,omitempty"`
}

func int32Ptr(v int32) *int32 { return &v }

type plainsBiome struct {
	HasPrecipitation bool         `nbt:"has_precipitation"`
	Temperature      float32      `nbt:"temperature"`
	Downfall         float32      `nbt:"downfall"`
	Effects          biomeEffects `nbt:"effects"`
}

// buildOverworldDimension returns the overworld dimension type exactly as we
// send it. Extracted so tests can assert the data contract without going
// through the registry.
func buildOverworldDimension() dimensionType26_1 {
	return dimensionType26_1{
		HasSkylight:         true,
		HasCeiling:          false,
		HasEnderDragonFight: false,
		CoordinateScale:     1.0,
		Height:              384,
		LogicalHeight:       384,
		MinY:                -64,
		AmbientLight:        0.0,
		InfiniteBurn:        "#minecraft:infiniburn_overworld",
		// 26.3 made this a uniform IntProvider, not a bare int.
		MonsterSpawnLightLevel:      intProviderUniform{"minecraft:uniform", 0, 7},
		MonsterSpawnBlockLightLimit: 0,
		// Inline list of timeline element ids. The referenced elements are sent
		// in config.go; their Holder references bind through placeholder
		// references created at parse time (see the struct comment above).
		Timelines:    []string{"minecraft:day", "minecraft:moon", "minecraft:early_game", "minecraft:villager_schedule"},
		DefaultClock: "minecraft:overworld",
		Attributes:   buildOverworldAttributes(),
	}
}

func registerDimension(r *mcregistry.Registries, sizes map[string]int) {
	r.DimensionType.Put("minecraft:overworld", marshalNBT(buildOverworldDimension()))
	sizes["minecraft:dimension_type"] = 1
}

func registerBiomes(r *mcregistry.Registries, sizes map[string]int) {
	const biomeCount = 65
	for i := 0; i < biomeCount; i++ {
		id := biome.Type(i).String()
		name := strings.TrimPrefix(id, "minecraft:")
		bd := plainsBiome{
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
		switch name {
		case "desert", "badlands", "eroded_badlands", "wooded_badlands", "savanna", "savanna_plateau", "windswept_savanna":
			bd.HasPrecipitation = false
			bd.Temperature = 2.0
			bd.Downfall = 0.0
			bd.Effects.SkyColor = 7254527
			if strings.Contains(name, "badlands") {
				bd.Effects.GrassColor = int32Ptr(9470285)
				bd.Effects.FoliageColor = int32Ptr(10387789)
			}
		case "snowy_plains", "ice_spikes", "snowy_taiga", "snowy_beach", "snowy_slopes", "frozen_peaks", "jagged_peaks", "frozen_river", "frozen_ocean", "deep_frozen_ocean", "grove":
			bd.Temperature = 0.0
			bd.Downfall = 0.5
			bd.Effects.SkyColor = 8364543
		case "swamp":
			bd.Temperature = 0.8
			bd.Downfall = 0.9
			bd.Effects.WaterColor = 6388580
			bd.Effects.WaterFogColor = 2302743
			bd.Effects.FoliageColor = int32Ptr(6975545)
			bd.Effects.GrassColorModifier = "swamp"
		case "mangrove_swamp":
			bd.Temperature = 0.8
			bd.Downfall = 0.9
			bd.Effects.WaterColor = 3832426
			bd.Effects.WaterFogColor = 5077600
			bd.Effects.FoliageColor = int32Ptr(9285927)
			bd.Effects.GrassColorModifier = "swamp"
		case "dark_forest":
			bd.Temperature = 0.7
			bd.Downfall = 0.8
			bd.Effects.GrassColorModifier = "dark_forest"
		case "pale_garden":
			bd.Temperature = 0.7
			bd.Downfall = 0.8
			bd.Effects.GrassColor = int32Ptr(7832178)
			bd.Effects.FoliageColor = int32Ptr(8883574)
		case "cherry_grove":
			bd.Temperature = 0.5
			bd.Downfall = 0.8
			bd.Effects.WaterColor = 6141935
			bd.Effects.WaterFogColor = 6141935
			bd.Effects.GrassColor = int32Ptr(11983713)
			bd.Effects.FoliageColor = int32Ptr(11983713)
		case "jungle", "bamboo_jungle", "sparse_jungle":
			bd.Temperature = 0.95
			bd.Downfall = 0.9
		case "forest", "flower_forest", "birch_forest", "old_growth_birch_forest", "meadow":
			bd.Temperature = 0.7
			bd.Downfall = 0.8
		case "taiga", "old_growth_pine_taiga", "old_growth_spruce_taiga", "windswept_hills", "windswept_forest", "windswept_gravelly_hills", "stony_shore", "stony_peaks":
			bd.Temperature = 0.25
			bd.Downfall = 0.8
		}
		r.WorldGenBiome.Put(id, marshalNBT(bd))
	}
	sizes["minecraft:worldgen/biome"] = biomeCount
}
