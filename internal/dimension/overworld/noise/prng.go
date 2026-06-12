package noise

// PRNG is the shared interface every random backend implements. It mirrors
// the contract Mojang's net.minecraft.world.level.levelgen.RandomSource
// exposes, narrowed to the methods worldgen actually calls.
type PRNG interface {
	// Seed returns the next int32, sign-corrected.
	NextInt() int32
	// Seed returns the next int32 in [0, bound). Panics if bound <= 0.
	NextIntBounded(bound int32) int32
	// NextLong returns a 64-bit value (sign-corrected for Xoroshiro).
	NextLong() int64
	// NextFloat returns [0, 1).
	NextFloat() float32
	// NextDouble returns [0, 1).
	NextDouble() float64
	// NextBoolean returns a coin flip.
	NextBoolean() bool
	// Fork returns an independent stream derived from this one.
	Fork() PRNG
}

// Xoroshirolike is a type constraint covering the two PRNG backends
// (Xoroshiro and Legacy) so worldgen_random.go can dispatch on the
// concrete type without using reflection.
type Xoroshirolike interface {
	*Xoroshiro | *Legacy
}
