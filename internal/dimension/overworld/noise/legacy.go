package noise

// Legacy is a port of java.util.Random — the PRNG that powered worldgen
// before Xoroshiro (and still backs the legacy_random_source code path).
// Mojang wraps this in LegacyRandomSource and selects it when the dimension
// declares legacy_random_source = true. State is a single 48-bit number
// advanced with the Linear Congruential Generator formula from java.util.Random.
type Legacy struct {
	seed uint64 // low 48 bits are the LCG state
}

// NewLegacy builds a Legacy generator from a 64-bit seed. Mirrors
// java.util.Random(long) which writes (seed ^ 0x5DEECE66D) & ((1<<48)-1).
func NewLegacy(seed int64) *Legacy {
	return &Legacy{seed: (uint64(seed) ^ 0x5DEECE66D) & ((1 << 48) - 1)}
}

// advance performs one LCG step and returns the new 48-bit state.
func (l *Legacy) advance() uint64 {
	const a = 0x5DEECE66D
	const c = 0xB
	const m = uint64(1)<<48 - 1
	l.seed = (l.seed*a + c) & m
	return l.seed
}

// nextBits returns the top `bits` bits of the 48-bit state (vanilla's
// java.util.Random.next(bits)).
func (l *Legacy) nextBits(bits uint) uint64 {
	return l.advance() >> (48 - bits)
}

// NextInt returns a uniform int32 — Java's java.util.Random.next(32), the raw
// signed 32-bit value (NOT sign-corrected).
func (l *Legacy) NextInt() int32 {
	return int32(uint32(l.nextBits(32)))
}

// NextIntBounded returns a uniform int in [0, bound), matching
// java.util.Random.nextInt(int): power-of-two fast path plus rejection sampling.
func (l *Legacy) NextIntBounded(bound int32) int32 {
	if bound <= 0 {
		panic("noise: NextIntBounded requires bound > 0")
	}
	if bound&(bound-1) == 0 { // power of two
		return int32(int64(bound) * int64(l.nextBits(31)) >> 31)
	}
	m := bound - 1
	for {
		bits := int32(l.nextBits(31))
		v := bits % bound
		if bits-v+m >= 0 {
			return v
		}
	}
}

// NextLong returns a uniform 64-bit value (java.util.Random.nextLong).
func (l *Legacy) NextLong() int64 {
	hi := int32(uint32(l.nextBits(32)))
	lo := int32(uint32(l.nextBits(32)))
	return int64(hi)<<32 + int64(lo)
}

// NextFloat returns a uniform float in [0, 1).
func (l *Legacy) NextFloat() float32 {
	return float32(l.nextBits(24)) / float32(1<<24)
}

// NextDouble returns a uniform double in [0, 1), matching
// java.util.Random.nextDouble: (next(26)<<27 + next(27)) * 2^-53.
func (l *Legacy) NextDouble() float64 {
	hi := int64(l.nextBits(26)) << 27
	lo := int64(l.nextBits(27))
	return float64(hi+lo) * (1.0 / (1 << 53))
}

// NextBoolean returns a uniform coin flip.
func (l *Legacy) NextBoolean() bool { return l.nextBits(1) != 0 }

// SetSeed re-seeds the LCG (java.util.Random.setSeed).
func (l *Legacy) SetSeed(seed int64) {
	l.seed = (uint64(seed) ^ 0x5DEECE66D) & ((1 << 48) - 1)
}

// Seed returns the current 48-bit state. Useful for debugging.
func (l *Legacy) Seed() uint64 { return l.seed }

// Fork returns an independent Legacy whose state is derived from this
// one's next 32-bit value. Mirrors WorldgenRandom.fork() for the legacy
// code path.
func (l *Legacy) Fork() PRNG {
	n := &Legacy{seed: l.seed}
	n.advance()
	return n
}
