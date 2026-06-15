// Package noise is LivingWorld's vanilla-compatible random and noise
// infrastructure. It mirrors the contracts Mojang exposes from
// net.minecraft.world.level.levelgen, and is the foundation every other
// worldgen stage builds on:
//
//   - Xoroshiro / Legacy : seedable PRNG backends. WorldgenRandom picks one
//     based on the dimension's noise_settings.legacy_random_source flag.
//   - WorldgenRandom    : the public façade. It exposes the same seeding
//     methods vanilla does (setDecorationSeed, setFeatureSeed, setCarverSeed,
//     setLargeFeatureSeed, setLargeFeatureWithSalt) plus positional forking,
//     so feature / structure / carver code stays bit-compatible with Mojang.
//   - Perlin            : 2D/3D improved Perlin gradient noise, the field
//     sampler used for climate and ore placement.
//   - NormalNoise       : the unit-distribution noise Mojang actually feeds
//     into the density / climate routers (two Perlin octave stacks combined).
//
// Determinism contract: for a given world seed, every output of this package
// is a pure function of that seed and the queried coordinates. Two runs of
// the same seed always produce the same terrain.
//
// Faithfulness contract: the Xoroshiro backend implements Java's
// xoroshiro128++ (XoroshiroRandomSource) bit-for-bit — the PlusPlus output
// function, the upgradeSeedTo128bit (Stafford-13) seed expansion, and the
// nextLong/nextInt/nextDouble shapes — so a given world seed produces the same
// raw random stream Mojang would. See xoroshiro_test.go for the golden vectors.
package noise

import "math/bits"

// Seed-mixing constants, named as Mojang names them (RandomSupport).
const (
	goldenRatio64 = uint64(0x9E3779B97F4A7C15) // GOLDEN_RATIO_64
	silverRatio64 = uint64(0x6A09E667F3BCC909) // SILVER_RATIO_64
)

// SplitMix64 expands a 64-bit seed by one splitmix64 step (increment + the
// Stafford-13 finalizer). Kept for callers that want a quick 64-bit mix of an
// arbitrary value; the Xoroshiro seeding path uses mixStafford13 directly.
func SplitMix64(seed uint64) uint64 {
	return mixStafford13(seed + goldenRatio64)
}

// mixStafford13 is Mojang's RandomSupport.mixStafford13 — the splitmix64
// finalizer (variant "13") WITHOUT the leading increment. It is the mixer used
// by upgradeSeedTo128bit to derive the two xoroshiro state words.
func mixStafford13(z uint64) uint64 {
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return z ^ (z >> 31)
}

// upgradeSeedTo128bit expands a 64-bit seed into the two 128-bit xoroshiro
// state words exactly as Java's RandomSupport.upgradeSeedTo128bit does.
func upgradeSeedTo128bit(seed uint64) (lo, hi uint64) {
	l := seed ^ silverRatio64
	m := l + goldenRatio64
	return mixStafford13(l), mixStafford13(m)
}

// Xoroshiro is the modern vanilla PRNG (since 1.16), Mojang's
// XoroshiroRandomSource. State is a 128-bit pair advanced with the
// xoroshiro128++ scrambler (not **). lo == Java's seedLo (s0), hi == seedHi
// (s1). It has a 2^128 period and is not cryptographically secure.
type Xoroshiro struct {
	lo uint64 // s0 / seedLo
	hi uint64 // s1 / seedHi
}

// newXoroshiroState builds a generator from explicit 128-bit state, applying
// Mojang's all-zero guard (a (0,0) state would be a fixed point).
func newXoroshiroState(lo, hi uint64) *Xoroshiro {
	if lo == 0 && hi == 0 {
		lo, hi = goldenRatio64, silverRatio64
	}
	return &Xoroshiro{lo: lo, hi: hi}
}

// NewXoroshiro builds a Xoroshiro from a 64-bit seed via upgradeSeedTo128bit,
// matching new XoroshiroRandomSource(long).
func NewXoroshiro(seed uint64) *Xoroshiro {
	lo, hi := upgradeSeedTo128bit(seed)
	return newXoroshiroState(lo, hi)
}

// NewXoroshiroRaw builds a Xoroshiro from a pre-built (seedLo, seedHi) state
// pair, matching new XoroshiroRandomSource(long seedLo, long seedHi). The
// all-zero guard is applied.
func NewXoroshiroRaw(seedLo, seedHi uint64) *Xoroshiro {
	return newXoroshiroState(seedLo, seedHi)
}

// next advances the state and returns the 64-bit xoroshiro128++ output:
//
//	result = rotl(s0 + s1, 17) + s0
//	s1 ^= s0
//	s0 = rotl(s0, 49) ^ s1 ^ (s1 << 21)
//	s1 = rotl(s1, 28)
func (x *Xoroshiro) next() uint64 {
	s0, s1 := x.lo, x.hi
	result := bits.RotateLeft64(s0+s1, 17) + s0
	s1 ^= s0
	x.lo = bits.RotateLeft64(s0, 49) ^ s1 ^ (s1 << 21)
	x.hi = bits.RotateLeft64(s1, 28)
	return result
}

// SetSeed replaces the state with one derived from seed (XoroshiroRandomSource.
// setSeed): re-expanded through upgradeSeedTo128bit.
func (x *Xoroshiro) SetSeed(seed int64) {
	lo, hi := upgradeSeedTo128bit(uint64(seed))
	if lo == 0 && hi == 0 {
		lo, hi = goldenRatio64, silverRatio64
	}
	x.lo, x.hi = lo, hi
}

// NextLong returns the next 64-bit value as a signed Java long.
func (x *Xoroshiro) NextLong() int64 { return int64(x.next()) }

// NextInt returns the low 32 bits of the next output as a signed int — Java's
// XoroshiroRandomSource.nextInt() == (int) nextLong(). NOT sign-corrected.
func (x *Xoroshiro) NextInt() int32 { return int32(uint32(x.next())) }

// NextIntBounded returns a uniform int in [0, bound). It draws a non-negative
// 31-bit value (the top bits of next()) and applies java.util.Random's
// power-of-two fast path / rejection sampling, so the result is always in range
// and unbiased.
func (x *Xoroshiro) NextIntBounded(bound int32) int32 {
	if bound <= 0 {
		panic("noise: NextIntBounded requires bound > 0")
	}
	m := bound - 1
	if bound&m == 0 { // power of two
		return int32((int64(bound) * int64(x.next()>>33)) >> 31)
	}
	for {
		bits := int32(x.next() >> 33) // non-negative 31-bit value
		v := bits % bound
		if bits-v+m >= 0 {
			return v
		}
	}
}

// NextFloat returns a uniform float in [0, 1): the top 24 bits times 2^-24.
func (x *Xoroshiro) NextFloat() float32 {
	return float32(x.next()>>40) * float32(1.0/(1<<24))
}

// NextDouble returns a uniform double in [0, 1): the top 53 bits times 2^-53.
func (x *Xoroshiro) NextDouble() float64 {
	return float64(x.next()>>11) * (1.0 / (1 << 53))
}

// NextBoolean returns a uniform coin flip (low bit of the next output).
func (x *Xoroshiro) NextBoolean() bool { return x.next()&1 != 0 }

// Fork returns an independent Xoroshiro seeded from two raw draws of this
// stream, matching XoroshiroRandomSource.fork().
func (x *Xoroshiro) Fork() PRNG {
	lo := x.next()
	hi := x.next()
	return newXoroshiroState(lo, hi)
}

// ForkPositional returns a factory that derives independent, coordinate- or
// name-addressed streams from this generator's state, matching
// XoroshiroRandomSource.forkPositional(). It draws the two base words from this
// stream once; the returned factory is then a pure function of (x,y,z) / name.
func (x *Xoroshiro) ForkPositional() *XoroshiroPositional {
	return &XoroshiroPositional{seedLo: x.next(), seedHi: x.next()}
}
