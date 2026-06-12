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

// NextInt returns a uniform int32 (sign-corrected).
func (l *Legacy) NextInt() int32 {
	v := int32(l.nextBits(32))
	if v < 0 {
		return -v
	}
	return v
}

// NextIntBounded returns a uniform int in [0, bound). Matches the
// nextInt(int) contract: unbiased via the rejection-sampling shape.
func (l *Legacy) NextIntBounded(bound int32) int32 {
	if bound <= 0 {
		panic("noise: NextIntBounded requires bound > 0")
	}
	// Mojang: if (bound & -bound) == bound { return next(31) * bound >> 31 }
	if bound&(-bound) == bound {
		return int32(int64(l.nextBits(31)) * int64(bound) >> 31)
	}
	for {
		bits := l.nextBits(31)
		v := bits % uint64(bound)
		if bits-uint64(bound)*v < 0 {
			continue
		}
		return int32(v)
	}
}

// NextLong returns a uniform 64-bit value.
func (l *Legacy) NextLong() int64 {
	hi := l.nextBits(32)
	lo := l.nextBits(32)
	return int64(hi)<<32 | int64(lo)
}

// NextFloat returns a uniform float in [0, 1).
func (l *Legacy) NextFloat() float32 {
	return float32(l.nextBits(24)) / float32(1<<24)
}

// NextDouble returns a uniform double in [0, 1).
func (l *Legacy) NextDouble() float64 {
	return float64(l.nextBits(26)) / float64(1<<26) // / (1<<26)
}

// NextBoolean returns a uniform coin flip.
func (l *Legacy) NextBoolean() bool { return l.nextBits(1) != 0 }

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
