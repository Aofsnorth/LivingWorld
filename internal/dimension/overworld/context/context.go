// Package context is the Overworld's immutable per-world worldgen
// context. It mirrors vanilla's WorldgenContext object: a read-only bag
// the chunk pipeline queries to build a chunk, built ONCE per world
// from the dimension's noise_settings + the world seed.
//
// The context is shared across all chunks and all goroutines; every
// field is either immutable (the noise samplers) or a thread-safe
// lookup (the per-axis noise).
package context

import (
	"encoding/json"

	"livingworld/internal/dimension/overworld/biome"
	"livingworld/internal/dimension/overworld/carver"
	"livingworld/internal/dimension/overworld/climate"
	"livingworld/internal/dimension/overworld/density"
	"livingworld/internal/dimension/overworld/noise"
	"livingworld/internal/dimension/overworld/ore"
	"livingworld/internal/dimension/overworld/registry"
	"livingworld/internal/dimension/overworld/structure"
	"livingworld/internal/dimension/overworld/surface"
	"livingworld/internal/dimension/overworld/vanilladata"
)

// Context is the Overworld's per-world worldgen context. It is
// constructed once via New and reused for every chunk.
type Context struct {
	Seed     int64
	Reg      *registry.Registry
	Sampler  *climate.Sampler
	Final    density.FinalDensity
	Aquifer  *aquiferHelper
	Surface  *surface.Program
	Carvers  []carver.Carver
	Ores     []ore.Config
	Struct   *structure.Planner
	NoiseCtx density.Context
}

// aquiferHelper bridges the aquifer package's constructor with the
// density package's types. It exists so the worldgen context can hold
// the aquifer alongside the FinalDensity without exposing the
// per-package types here.
type aquiferHelper struct {
	Apply func(density.Context, int, int, int, string, biome.Parameters) string
}

// VanillaDensity builds just the vanilla density stack — the noise samplers
// plus the parsed overworld final_density tree — for the given seed. It is
// the terrain-shape source the chunk pipeline queries; New wraps the same
// stack in the full worldgen context.
func VanillaDensity(seed int64) (density.Context, density.FinalDensity) {
	noises := buildNoiseSet(seed)
	noiseCtx := density.NewContext(seed, noises)

	settingsRaw, ok := vanilladata.LoadNoiseSettings("overworld")
	if !ok {
		panic("overworld noise_settings not found")
	}
	tree, err := density.ParseFinalDensity(noiseCtx, settingsRaw)
	if err != nil {
		panic(err)
	}
	return noiseCtx, density.FinalDensity{Tree: tree}
}

// VanillaStack builds the noise context, the final_density tree, and the six
// climate router functions for one world seed. The chunk pipeline uses the
// climate functions to select biomes the way NoiseRouter.createClimateSampler
// + MultiNoiseBiomeSource do in vanilla.
func VanillaStack(seed int64) (density.Context, density.FinalDensity, density.ClimateFunctions) {
	noises := buildNoiseSet(seed)
	noiseCtx := density.NewContext(seed, noises)

	settingsRaw, ok := vanilladata.LoadNoiseSettings("overworld")
	if !ok {
		panic("overworld noise_settings not found")
	}
	tree, err := density.ParseFinalDensity(noiseCtx, settingsRaw)
	if err != nil {
		panic(err)
	}
	climate, err := density.ParseClimate(noiseCtx, settingsRaw)
	if err != nil {
		panic(err)
	}
	return noiseCtx, density.FinalDensity{Tree: tree}, climate
}

// New builds a WorldgenContext for the given world seed. It is
// expensive (one-shot at world bootstrap) but everything downstream
// is read-only.
func New(seed int64) *Context {
	reg := registry.NewOverworld()
	seeds := climate.AxisSeeds{
		TemperatureFirst:      0x7E2A_F1A0 ^ seed,
		TemperatureSecond:     0x8A92_F1A0 ^ seed,
		HumidityFirst:         0x7E2A_F1A0 ^ seed*31,
		HumiditySecond:        0x8A92_F1A0 ^ seed*31,
		ContinentalnessFirst:  0x1_F1A0 ^ seed*37,
		ContinentalnessSecond: 0x8A92_F1A0 ^ seed*37,
		ErosionFirst:          0x7E2A_F1A0 ^ seed*41,
		ErosionSecond:         0x8A92_F1A0 ^ seed*41,
		WeirdnessFirst:        0x7E2A_F1A0 ^ seed*43,
		WeirdnessSecond:       0x8A92_F1A0 ^ seed*43,
		DepthFirst:            0x7E2A_F1A0 ^ seed*47,
		DepthSecond:           0x8A92_F1A0 ^ seed*47,
	}
	sampler := climate.NewSampler(seeds, 1.0/4.0)

	noiseCtx, final := VanillaDensity(seed)

	aq := &aquiferHelper{Apply: nil}
	_ = aq
	surf := surface.NewOverworldProgram()
	cars := []carver.Carver{carver.NewOverworldCave(), carver.NewOverworldCanyon()}
	ores := ore.AllOverworldOres()
	planner := structure.NewPlanner(seed)
	return &Context{
		Seed:     seed,
		Reg:      reg,
		Sampler:  sampler,
		Final:    final,
		Aquifer:  nil,
		Surface:  surf,
		Carvers:  cars,
		Ores:     ores,
		Struct:   planner,
		NoiseCtx: noiseCtx,
	}
}

// buildNoiseSet converts vanilla's 26.3 noise parameter records into
// samplers. The 26.3 schema is {base_amplitude, base_octave, octave_count}
// (NormalNoise$Parameters): octave k runs at frequency 2^(base_octave+k)
// with amplitude base_amplitude * 0.5^k (buildOctaves). An older revision of
// this function read {firstOctave, amplitudes} — a schema no vanilla data
// file uses — so every noise silently became a constant and the density
// tree's cave/climate terms collapsed to flat values.
func buildNoiseSet(seed int64) density.NoiseSet {
	type params struct {
		BaseAmplitude     float64   `json:"base_amplitude"`
		BaseOctave        int       `json:"base_octave"`
		OctaveCount       int       `json:"octave_count"`
		AmplitudeModifier []float64 `json:"amplitude_modifiers"`
	}

	pos := noise.NewXoroshiro(uint64(seed)).ForkPositional()
	noises := make(density.NoiseSet, len(vanilladata.Noises))
	for name, raw := range vanilladata.Noises {
		var p params
		if err := json.Unmarshal(raw, &p); err != nil {
			panic(err)
		}
		// Codec defaults: base_amplitude 1.0, octave_count 1.
		if p.BaseAmplitude == 0 {
			p.BaseAmplitude = 1.0
		}
		if p.OctaveCount <= 0 {
			p.OctaveCount = 1
		}
		r := pos.FromHashOf("minecraft:" + name)
		noises[name] = noise.NewNormalNoise26(r, p.BaseOctave, p.BaseAmplitude, p.OctaveCount, p.AmplitudeModifier)
	}
	return noises
}
