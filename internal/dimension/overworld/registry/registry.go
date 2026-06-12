// Package registry is the Overworld's pre-compiled worldgen registry. It
// is the result of reading the datapack
// data/minecraft/worldgen/{noise,density_function,noise_settings,...}
// block for the Overworld and baking it into a Go struct tree. Once
// built, it is a constant — the chunk pipeline consults it on every
// chunk without re-reading JSON.
//
// For our v1 the registry is a hand-rolled value object (not a JSON
// loader) because the datapack is too large to be parsed line-by-line
// for every world context. The same constants can later be loaded from
// JSON by replacing this file's var with a deserialise call.
package registry

import "livingworld/internal/dimension/overworld/biome"

// NoiseSettings is the Overworld baseline the blueprint calls out:
// min_y = -64, height = 384, sea_level = 63, ore_veins_enabled = true,
// legacy_random_source = false. The "size_horizontal" / "size_vertical"
// are the cell sizes Mojang uses for the noise router (1 and 2
// respectively).
type NoiseSettings struct {
	MinY               int
	Height             int
	SeaLevel           int
	OreVeinsEnabled    bool
	LegacyRandomSource bool
	SizeHorizontal     int
	SizeVertical       int
}

// OverworldNoiseSettings returns the canonical 26.1.2 baseline.
func OverworldNoiseSettings() NoiseSettings {
	return NoiseSettings{
		MinY:               -64,
		Height:             384,
		SeaLevel:           63,
		OreVeinsEnabled:    true,
		LegacyRandomSource: false,
		SizeHorizontal:     1,
		SizeVertical:       2,
	}
}

// Registry is the immutable bag the WorldgenContext carries.
type Registry struct {
	Noise        NoiseSettings
	SurfaceRules any // surface.Program; kept as any to avoid an import cycle
	OreConfigs   any // []ore.Config
	Structures   any // []structure.Set
	Carvers      any // []carver.Carver
	BiomeTable   []biome.Parameters
}

// NewOverworld returns the canonical registry. It is safe to share
// across all worlds (every field is read-only).
func NewOverworld() *Registry {
	return &Registry{
		Noise:      OverworldNoiseSettings(),
		BiomeTable: biome.All(),
	}
}
