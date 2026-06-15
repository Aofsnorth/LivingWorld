package feature

import "livingworld/internal/dimension/overworld/noise"

// perlinLocal is the feature package's handle to the canonical Perlin field
// sampler. It used to be a separate (and subtly divergent) copy of the noise
// algorithm; it now delegates to noise.Perlin so there is a single faithful
// implementation. The thin wrapper is kept so step.go's call sites
// (newPerlinLocal / noise3) stay unchanged.
type perlinLocal struct{ p *noise.Perlin }

func newPerlinLocal(seed int64) *perlinLocal { return &perlinLocal{p: noise.NewPerlin(seed)} }

func (p *perlinLocal) noise3(x, y, z float64) float64 { return p.p.Noise3D(x, y, z) }
