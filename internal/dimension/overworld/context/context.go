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
	"livingworld/internal/dimension/overworld/biome"
	"livingworld/internal/dimension/overworld/carver"
	"livingworld/internal/dimension/overworld/climate"
	"livingworld/internal/dimension/overworld/density"
	"livingworld/internal/dimension/overworld/feature"
	"livingworld/internal/dimension/overworld/noise"
	"livingworld/internal/dimension/overworld/ore"
	"livingworld/internal/dimension/overworld/registry"
	"livingworld/internal/dimension/overworld/structure"
	"livingworld/internal/dimension/overworld/surface"
)

// Context is the Overworld's per-world worldgen context. It is
// constructed once via New and reused for every chunk.
type Context struct {
	Seed      int64
	Reg       *registry.Registry
	Sampler   *climate.Sampler
	Final     density.FinalDensity
	Aquifer   *aquiferHelper
	Surface   *surface.Program
	Carvers   []carver.Carver
	Ores      []ore.Config
	Struct    *structure.Planner
	NoiseCtx  density.Context
}

// aquiferHelper bridges the aquifer package's constructor with the
// density package's types. It exists so the worldgen context can hold
// the aquifer alongside the FinalDensity without exposing the
// per-package types here.
type aquiferHelper struct {
	Apply func(density.Context, int, int, int, string, biome.Parameters) string
}

// New builds a WorldgenContext for the given world seed. It is
// expensive (one-shot at world bootstrap) but everything downstream
// is read-only.
func New(seed int64) *Context {
	reg := registry.NewOverworld()
	seeds := climate.AxisSeeds{
		TemperatureFirst: 0x7E2A_F1A0 ^ seed,
		TemperatureSecond: 0x8A92_F1A0 ^ seed,
		HumidityFirst: 0x7E2A_F1A0 ^ seed*31,
		HumiditySecond: 0x8A92_F1A0 ^ seed*31,
		ContinentalnessFirst: 0x1_F1A0 ^ seed*37,
		ContinentalnessSecond: 0x8A92_F1A0 ^ seed*37,
		ErosionFirst: 0x7E2A_F1A0 ^ seed*41,
		ErosionSecond: 0x8A92_F1A0 ^ seed*41,
		WeirdnessFirst: 0x7E2A_F1A0 ^ seed*43,
		WeirdnessSecond: 0x8A92_F1A0 ^ seed*43,
		DepthFirst: 0x7E2A_F1A0 ^ seed*47,
		DepthSecond: 0x8A92_F1A0 ^ seed*47,
	}
	sampler := climate.NewSampler(seeds, 1.0/4.0)
	nc := density.Context{Seed: seed, Climate: samplerAxisAdapterFor(sampler)}
	final := density.BuildFinalDensity(nc, nc.Climate, density.NoiseSet{})
	aq := &aquiferHelper{Apply: nil}
	_ = aq // populated by dimension facade after wiring aquifer package
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
		NoiseCtx: nc,
	}
}

// samplerAxisAdapter lets a climate.Sampler satisfy the
// density.ClimateReader interface without an import cycle.
type samplerAxisAdapter struct{ S *climate.Sampler }

func (a samplerAxisAdapter) Axis(axis int, x, y, z int) float64 {
	p := a.S.Sample(x, y, z)
	switch axis {
	case 0:
		return p.Temperature
	case 1:
		return p.Humidity
	case 2:
		return p.Continentalness
	case 3:
		return p.Erosion
	case 4:
		return p.Weirdness
	case 5:
		return p.Depth
	}
	return 0
}

func samplerAxisAdapterFor(s *climate.Sampler) samplerAxisAdapter {
	return samplerAxisAdapter{S: s}
}

func samplerAxisAdapterOrig(s *climate.Sampler) density.ClimateReader { return samplerAxisAdapterFor(s) }
func init() {
	_ = samplerAxisAdapterOrig
	_ = feature.AllSteps
	_ = noise.NewXoroshiro
}
