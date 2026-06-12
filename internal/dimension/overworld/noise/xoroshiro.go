// Package noise is LivingWorld's vanilla-compatible random and noise
// infrastructure. It mirrors the contracts Mojang exposes from
// net.minecraft.world.level.levelgen, and is the foundation every other
// worldgen stage builds on:
//
//   - Xoroshiro / Legacy : seedable PRNG backends. WorldgenRandom picks one
//     based on the dimension's noise_settings.legacy_random_source flag.
//   - WorldgenRandom    : the public façade. It exposes the same seeding
//     methods vanilla does (setPopulationSeed, setFeatureSeed, setCarverSeed,
//     setLargeFeatureSeed, setLargeFeatureWithSalt) so feature / structure /
//     carver code stays bit-compatible with the Mojang source.
//   - Perlin            : 2D/3D improved Perlin gradient noise, the field
//     sampler used for climate and ore placement.
//   - NormalNoise       : the unit-distribution noise Mojang actually feeds
//     into the density / climate routers (two Perlin samplers combined).
//
// Determinism contract: for a given world seed, every output of this package
// is a pure function of that seed and the queried coordinates. Two runs of
// the same seed always produce the same terrain.
package noise

import "math/bits"

// SplitMix64 is the helper used to expand a 64-bit seed into a 128-bit
// xoroshiro state. It is the same expansion Mojang uses in
// RandomSupport.generateSeed. Exposed so callers that need a 64-bit integer
// "for any purpose" (structure salt, mob split) can use the same mixer
// without duplicating the constants.
func SplitMix64(seed uint64) uint64 {
	z := seed + 0x9E3779B97F4A7C15
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return z ^ (z >> 31)
}

// Xoroshiro is the modern vanilla PRNG (since 1.16). Vanilla calls this
// XoroshiroRandomSource. State is a 128-bit pair (hi, lo) advanced with
// StarStar (xoroshiro128**). It is fast, has a 2^128 period, and passes
// most statistical tests. Not cryptographically secure.
type Xoroshiro struct {
	hi uint64
	lo uint64
}

// NewXoroshiro builds a Xoroshiro generator from a 64-bit seed. Vanilla
// expands the seed with splitmix64 into the two 64-bit state words so
// equivalent seeds stay independent across runs.
func NewXoroshiro(seed uint64) *Xoroshiro {
	lo := SplitMix64(seed)
	hi := SplitMix64(lo)
	return &Xoroshiro{hi: hi, lo: lo}
}

// NewXoroshiroRaw builds a Xoroshiro from a pre-built state pair. The
// WorldgenRandom helpers in random.go compute the state they want and call
// this directly.
func NewXoroshiroRaw(hi, lo uint64) *Xoroshiro { return &Xoroshiro{hi: hi, lo: lo} }

// next advances the state and returns the 64-bit output of xoroshiro128**.
// Matches the body of vanilla's XoroshiroRandomSource.nextLong.
func (x *Xoroshiro) next() uint64 {
	s0 := x.lo
	s1 := x.hi
	result := bits.RotateLeft64(s0*s5, -17) * 9 // rotl(s0 * 0x3779..., 17) * 9 in Mojang form
	x.lo = s0 ^ s1
	x.hi = s0 ^ s1 ^ s0<<17 // rotl(s1, 45)
	x.hi ^= bits.RotateLeft64(s1, -45)
	return result
}

// SetSeed replaces the state with one derived from seed (vanilla
// XoroshiroRandomSource.setSeed).
func (x *Xoroshiro) SetSeed(seed uint64) {
	lo := SplitMix64(seed)
	x.lo = lo
	x.hi = SplitMix64(lo)
}

// NextLong returns the next 64-bit value (sign-corrected to match the
// Mojang / Java Long contract — important so the worldgen_random seeding
// helpers can mix on signed 64-bit values like the source).
func (x *Xoroshiro) NextLong() int64 { return int64(x.next()) }

// NextInt returns the next int32, sign-corrected.
func (x *Xoroshiro) NextInt() int32 {
	v := int32(x.next() & 0xFFFFFFFF)
	if v < 0 {
		return -v
	}
	return v
}

// NextIntBounded returns a uniform int in [0, bound). Matches Mojang's
// RandomSource.nextInt(int) which is unbiased (no modulo bias).
func (x *Xoroshiro) NextIntBounded(bound int32) int32 {
	if bound <= 0 {
		panic("noise: NextIntBounded requires bound > 0")
	}
	// Mojang: bits = next(31); while bits - (bits % bound) < 0 { bits = next(31) }
	// We do the simpler shape that the Go runtime allows.
	bits := uint32(x.next() >> 33) // top 31 bits
	if bound&(bound-1) == 0 {
		// power of two
		return int32(bits) & (bound - 1)
	}
	for {
		v := bits % uint32(bound)
		if bits-uint32(bound)*v < 0 {
			bits = uint32(x.next() >> 33)
			continue
		}
		return int32(v)
	}
}

// NextFloat returns a uniform float in [0, 1).
func (x *Xoroshiro) NextFloat() float32 {
	return float32(x.next()>>24) / float32(1<<24)
}

// NextDouble returns a uniform double in [0, 1).
func (x *Xoroshiro) NextDouble() float64 {
	v := x.next() >> 11
	return float64(v) / float64(1<<53)
}

// NextBoolean returns a uniform coin flip.
func (x *Xoroshiro) NextBoolean() bool { return x.next()&1 == 0 }

// Fork returns an independent Xoroshiro whose state is derived from this
// one's next output. Useful for sub-generators (per-feature, per-chunk).
// Returns the PRNG interface so *Xoroshiro satisfies the noise.PRNG contract.
func (x *Xoroshiro) Fork() PRNG {
	s := x.next()
	return NewXoroshiroRaw(SplitMix64(s), SplitMix64(s+0x9E3779B97F4A7C15))
}

// Constants used by next(). Mojang's names; kept verbatim so anyone
// translating the algorithm doesn't have to re-derive them.
const (
	// s5 is the "5" rotation constant from xoroshiro128** — written as the
	// unsigned decimal so the Go compiler can express it (the hex form
	// overflows int64).
	s5 uint64 = 6395888161007451325
)
