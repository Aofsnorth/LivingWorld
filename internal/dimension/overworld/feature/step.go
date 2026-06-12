// Package feature is the decoration-step stage. Vanilla's chunk pipeline
// runs features in 10 ordered GenerationStep.Decoration phases:
//
//   raw_generation
//   lakes
//   local_modifications
//   underground_structures
//   surface_structures
//   strongholds
//   underground_ores
//   underground_decoration
//   fluid_springs
//   vegetal_decoration
//   top_layer_modification
//
// Per step, the pipeline builds a global ordered list of placed features
// for the chunk's biomes and ticks them top-to-bottom. The decoration
// index is then mixed with the population seed (setFeatureSeed) to
// derive the per-feature sub-stream.
//
// For our v1 we model the steps, but only a few of them carry real
// feature work: underground_ores (the ore pass), vegetal_decoration
// (no-op for now), and top_layer_modification (snow / surface ice).
// The other steps are wired but empty; future placeable features
// (trees, flowers, structures inside the chunk) can hang off the same
// Step enum.
package feature

// Step is the per-decoration-step enum. The integer values match
// vanilla's GenerationStep.CarvingStep ordinals so SetFeatureSeed's
// (populationSeed, featureIndex, stepOrdinal) call lines up with the
// Mojang source.
type Step int

const (
	StepRawGeneration Step = iota
	StepLakes
	StepLocalModifications
	StepUndergroundStructures
	StepSurfaceStructures
	StepStrongholds
	StepUndergroundOres
	StepUndergroundDecoration
	StepFluidSprings
	StepVegetalDecoration
	StepTopLayerModification
)

// AllSteps is the full ordered list of decoration steps. The pipeline
// iterates over this slice top-to-bottom for every chunk.
var AllSteps = []Step{
	StepRawGeneration,
	StepLakes,
	StepLocalModifications,
	StepUndergroundStructures,
	StepSurfaceStructures,
	StepStrongholds,
	StepUndergroundOres,
	StepUndergroundDecoration,
	StepFluidSprings,
	StepVegetalDecoration,
	StepTopLayerModification,
}

// Placer is one feature in a chunk's step-ordered list. The pipeline
// calls Place on each in turn; the placer returns the modified block
// at the given (lx, ly, lz).
type Placer interface {
	Place(lx, ly, lz int) (blockName string, placed bool)
}

// PerStepPlacers is the list of placers for a given step on a chunk.
// The pipeline walks AllSteps; for each step it calls every placer in
// the per-step slice in order.
type PerStepPlacers struct {
	Step    Step
	Placers []Placer
}

// OrePlacer is a Placer that applies the ore-blob pass. It defers the
// real per-cell decision to ore.ApplyBlob.
type OrePlacer struct {
	Seed     int64
	ChunkX   int
	ChunkZ   int
	Configs  any // ore.Config slice; kept as any to avoid import cycle
	BlockAt  func(lx, ly, lz int) string
	SetBlock func(lx, ly, lz int, name string)
}

// Place applies the ore pass to a single cell. The pipeline calls
// Place for every solid stone cell in the chunk; the placer returns
// the new block name (or the current one if no ore applies).
func (p OrePlacer) Place(lx, ly, lz int) (string, bool) {
	cur := p.BlockAt(lx, ly, lz)
	cfgs, _ := p.Configs.([]OreConfig)
	newName := ApplyBlob(p.Seed, p.ChunkX, p.ChunkZ, lx, ly, lz, cur, cfgs)
	if newName != cur {
		p.SetBlock(lx, ly, lz, newName)
		return newName, true
	}
	return cur, false
}

// OreConfig is a local re-export of ore.Config so this package doesn't
// import the ore package directly (cycle). The ore package uses the
// same field set.
type OreConfig struct {
	BlockName      string
	MinY, MaxY     int
	VeinTries      int
	VeinSize       int
	ThresholdFloat float64
	MountainOnly   bool
}

// ApplyBlob is the per-cell decision; pulled out so the pipeline can
// call it without going through the Placer interface.
func ApplyBlob(seed int64, cx, cz, x, y, z int, currentBlock string, configs []OreConfig) string {
	if currentBlock != "minecraft:stone" {
		return currentBlock
	}
	ns := perlin3D{p: newPerlinLocal(seed ^ 0x6F65_7265)}
	for _, ore := range configs {
		if y < ore.MinY || y > ore.MaxY {
			continue
		}
		v := ns.sample(float64(x)+0.5, float64(y)+0.5, float64(z)+0.5)
		if v < 0 {
			v = -v
		}
		if v < ore.ThresholdFloat {
			continue
		}
		return ore.BlockName
	}
	return currentBlock
}

// perlin3D is a tiny 3D Perlin sampler used only by the ore pass. It
// is implemented locally so the feature package stays independent of
// the noise package (the ore package owns the canonical Perlin).
type perlin3D struct{ p *perlinLocal }

func (p perlin3D) sample(x, y, z float64) float64 { return p.p.noise3(x, y, z) }

func perlin3D_(seed int64) perlin3D { return perlin3D{p: newPerlinLocal(seed)} }
