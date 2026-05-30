// Package noise provides deterministic, seedable value sources for worldgen.
//
// Everything here is a pure function of the seed: the same seed always yields
// the same sequence/field, so Java and Bedrock clients see identical terrain
// (DESIGN §10, R3.3). It is NOT cryptographically secure and must not be used
// for anything security-sensitive.
package noise

// RNG is a deterministic splitmix64 pseudo-random generator.
type RNG struct{ state uint64 }

// New returns an RNG seeded from seed.
func New(seed int64) *RNG { return &RNG{state: uint64(seed)} }

// Derive returns an independent stream seeded from seed and salt, so
// sub-generators (per biome, per feature, per structure) stay deterministic yet
// decorrelated.
func Derive(seed int64, salt uint64) *RNG {
	return &RNG{state: uint64(seed) ^ (salt * 0x9E3779B97F4A7C15)}
}

// Uint64 returns the next 64-bit value (splitmix64).
func (r *RNG) Uint64() uint64 {
	r.state += 0x9E3779B97F4A7C15
	z := r.state
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return z ^ (z >> 31)
}

// Float64 returns the next value in [0, 1).
func (r *RNG) Float64() float64 { return float64(r.Uint64()>>11) / (1 << 53) }

// IntN returns the next value in [0, n). It panics if n <= 0.
func (r *RNG) IntN(n int) int {
	if n <= 0 {
		panic("noise: IntN requires n > 0")
	}
	return int(r.Uint64() % uint64(n))
}
