// Package density is LivingWorld's density function interpreter — the
// arithmetic that turns vanilla noise fields into terrain shape. It mirrors
// net.minecraft.world.level.levelgen.DensityFunction: a JSON-described
// arithmetic DAG that is evaluated once per block position.
//
// The runtime tree is built once per world at WorldgenContext bootstrap; every
// per-block evaluation reuses the compiled tree. The interpreter supports the
// operators Mojang uses in the Overworld preset.
//
// Every operator is a small interface implementation; new operators slot in by
// satisfying the Function interface and registering in parse.go.
package density

import "livingworld/internal/dimension/overworld/noise"

// Function is one node in a density function tree. It takes a context
// (block position + per-world state) and returns a single float64 value.
// Tree evaluation is recursive but cheap: the tree is small and the per-block
// call is the inner loop.
type Function interface {
	// Eval returns the function value at (x, y, z). x and z are block
	// coordinates; y is world Y. The Context is the per-call state
	// (the world seed + noise router + cache).
	Eval(c Context, x, y, z int) float64
}

// NoiseSet is the per-world map of vanilla noise ID to the NormalNoise
// sampler that implements it. Density operators read this map by name.
type NoiseSet map[string]*noise.NormalNoise

// Context carries the per-call state every Function may read.
// It is a value type so callers can copy it to reset caches per chunk.
type Context struct {
	// Seed is the world seed. Functions that need deterministic noise
	// (Noise, ShiftedNoise, OldBlendedNoise) seed their sources off this.
	Seed int64
	// Noises is the map of vanilla noise ID -> NormalNoise.
	Noises NoiseSet
	// Cache is the evaluation cache used by CacheOnce / Cache2D / FlatCache
	// and Interpolated. A fresh map should be used per chunk.
	Cache map[cacheKey]float64
}

// cacheKey is the key for the evaluation cache. Function implementations
// are compared by identity, so the key is a pointer or comparable value.
type cacheKey struct {
	f       Function
	x, y, z int
}

// cached returns a cached value for the given function and block position,
// computing it on first access.
func (c Context) cached(f Function, x, y, z int, compute func() float64) float64 {
	if c.Cache == nil {
		return compute()
	}
	key := cacheKey{f: f, x: x, y: y, z: z}
	if v, ok := c.Cache[key]; ok {
		return v
	}
	v := compute()
	c.Cache[key] = v
	return v
}

// NewContext builds a density Context with a fresh cache.
func NewContext(seed int64, noises NoiseSet) Context {
	return Context{
		Seed:   seed,
		Noises: noises,
		Cache:  make(map[cacheKey]float64),
	}
}
