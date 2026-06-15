package noise

import (
	"crypto/md5"
	"encoding/binary"
)

// WorldgenRandom is LivingWorld's façade in front of the Xoroshiro / Legacy
// backends. It mirrors vanilla's net.minecraft.world.level.levelgen.
// WorldgenRandom: the per-chunk, per-feature random everyone in worldgen reads.
// The backend (Xoroshiro or Legacy) is chosen once at world-context build from
// the dimension's noise_settings.legacy_random_source flag.
//
// The seeding helpers below implement Mojang's actual formulas verbatim (they
// re-seed the backend via SetSeed and mix with raw nextLong draws), so a given
// world seed reproduces Mojang's per-chunk/per-feature streams.
type WorldgenRandom struct {
	backend PRNG
}

// NewWorldgenRandom builds a WorldgenRandom with a Xoroshiro backend.
func NewWorldgenRandom(seed int64) *WorldgenRandom {
	return &WorldgenRandom{backend: NewXoroshiro(uint64(seed))}
}

// NewWorldgenRandomLegacy builds a WorldgenRandom with a Legacy backend.
func NewWorldgenRandomLegacy(seed int64) *WorldgenRandom {
	return &WorldgenRandom{backend: NewLegacy(seed)}
}

// SetBackend swaps the underlying PRNG (used by the context builder after
// reading the noise_settings flag).
func (r *WorldgenRandom) SetBackend(b PRNG) { r.backend = b }

// Backend returns the current backend. Exposed for tests / debug.
func (r *WorldgenRandom) Backend() PRNG { return r.backend }

// SetSeed re-seeds the backend (WorldgenRandom.setSeed).
func (r *WorldgenRandom) SetSeed(seed int64) { r.backend.SetSeed(seed) }

// SetPopulationSeed implements vanilla WorldgenRandom.setDecorationSeed: it
// re-seeds the backend to a per-chunk decoration seed derived from the world
// seed and the chunk's block XZ, and returns that seed (callers pass it to
// SetFeatureSeed). The "| 1" forces the multipliers odd, exactly as Mojang does.
func (r *WorldgenRandom) SetPopulationSeed(worldSeed uint64, blockX, blockZ int32) int64 {
	r.backend.SetSeed(int64(worldSeed))
	l := r.backend.NextLong() | 1
	m := r.backend.NextLong() | 1
	n := int64(blockX)*l + int64(blockZ)*m ^ int64(worldSeed)
	r.backend.SetSeed(n)
	return n
}

// SetFeatureSeed implements vanilla WorldgenRandom.setFeatureSeed: the
// decoration seed plus the feature's index and 10000×step ordinal.
func (r *WorldgenRandom) SetFeatureSeed(decorationSeed int64, featureIndex int32, stepOrdinal int32) {
	r.backend.SetSeed(decorationSeed + int64(featureIndex) + int64(10000*stepOrdinal))
}

// SetCarverSeed implements vanilla WorldgenRandom.setCarverSeed.
func (r *WorldgenRandom) SetCarverSeed(worldSeed int64, chunkX, chunkZ int32) {
	r.backend.SetSeed(worldSeed)
	l := r.backend.NextLong()
	m := r.backend.NextLong()
	r.backend.SetSeed(int64(chunkX)*l ^ int64(chunkZ)*m ^ worldSeed)
}

// SetLargeFeatureSeed implements vanilla WorldgenRandom.setLargeFeatureSeed.
func (r *WorldgenRandom) SetLargeFeatureSeed(worldSeed int64, chunkX, chunkZ int32) {
	r.backend.SetSeed(worldSeed)
	l := r.backend.NextLong()
	m := r.backend.NextLong()
	r.backend.SetSeed(int64(chunkX)*l ^ int64(chunkZ)*m ^ worldSeed)
}

// SetLargeFeatureWithSalt implements vanilla WorldgenRandom.setLargeFeatureWithSalt.
func (r *WorldgenRandom) SetLargeFeatureWithSalt(worldSeed int64, regionX, regionZ, salt int32) {
	r.backend.SetSeed(int64(regionX)*341873128712 + int64(regionZ)*132897987541 + worldSeed + int64(salt))
}

// SeedSlimeChunk returns the seed vanilla feeds to a java.util.Random to decide
// whether (chunkX,chunkZ) is a slime chunk (the check is nextInt(10) == 0 on a
// Legacy source built from this seed).
func SeedSlimeChunk(worldSeed int64, chunkX, chunkZ int32) int64 {
	return worldSeed +
		int64(chunkX*chunkX*0x4C1906) +
		int64(chunkX*0x5AC0DB) +
		int64(chunkZ*chunkZ)*0x4307A7 +
		int64(chunkZ*0x5F24F) ^ 0x3AD8025F
}

// SetDecorationSeed is the package-level convenience used where a fresh
// generator is built per call (returns the decoration seed; see SetPopulationSeed).
func SetDecorationSeed(worldSeed int64, blockX, blockZ int32) int64 {
	return NewWorldgenRandom(worldSeed).SetPopulationSeed(uint64(worldSeed), blockX, blockZ)
}

// --- passthrough PRNG methods -----------------------------------------

func (r *WorldgenRandom) NextInt() int32               { return r.backend.NextInt() }
func (r *WorldgenRandom) NextIntBounded(b int32) int32 { return r.backend.NextIntBounded(b) }
func (r *WorldgenRandom) NextLong() int64              { return r.backend.NextLong() }
func (r *WorldgenRandom) NextFloat() float32           { return r.backend.NextFloat() }
func (r *WorldgenRandom) NextDouble() float64          { return r.backend.NextDouble() }
func (r *WorldgenRandom) NextBoolean() bool            { return r.backend.NextBoolean() }
func (r *WorldgenRandom) Fork() PRNG                   { return r.backend.Fork() }

// --- positional forking (vanilla XoroshiroPositionalRandomFactory) ----

// XoroshiroPositional derives independent Xoroshiro streams addressed by world
// coordinates or by a name hash, matching Mojang's
// XoroshiroPositionalRandomFactory. It is the seeding mechanism behind named
// noises ("minecraft:temperature", …) and per-position feature placement.
type XoroshiroPositional struct {
	seedLo, seedHi uint64
}

// At returns the stream for block position (x,y,z): XoroshiroRandomSource(
// Mth.getSeed(x,y,z) ^ seedLo, seedHi).
func (f *XoroshiroPositional) At(x, y, z int) *Xoroshiro {
	return newXoroshiroState(uint64(mthGetSeed(x, y, z))^f.seedLo, f.seedHi)
}

// FromHashOf returns the stream for a name: XoroshiroRandomSource(md5[0:8] ^
// seedLo, md5[8:16] ^ seedHi), with the MD5 bytes read big-endian (Java's
// Longs.fromBytes). This is how Mojang seeds each named worldgen noise.
func (f *XoroshiroPositional) FromHashOf(name string) *Xoroshiro {
	lo, hi := md5Seed(name)
	return newXoroshiroState(lo^f.seedLo, hi^f.seedHi)
}

// mthGetSeed mirrors net.minecraft.util.Mth.getSeed. Note the first multiply is
// a 32-bit int multiply (overflowing) BEFORE the widen to long, exactly as Java
// evaluates it — the int32 cast preserves that wraparound.
func mthGetSeed(x, y, z int) int64 {
	l := int64(int32(x)*3129871) ^ int64(z)*116129781 ^ int64(y)
	l = l*l*42317861 + l*11
	return l >> 16
}

// md5Seed returns the two 64-bit words of MD5(name), big-endian, matching
// Java's Hashing.md5().hashString(name) + Longs.fromBytes split.
func md5Seed(name string) (lo, hi uint64) {
	sum := md5.Sum([]byte(name))
	return binary.BigEndian.Uint64(sum[0:8]), binary.BigEndian.Uint64(sum[8:16])
}

// SeedFromHashOf returns a standalone Xoroshiro seeded purely from a name hash
// (no positional base), i.e. XoroshiroRandomSource(md5[0:8], md5[8:16]).
func SeedFromHashOf(name string) *Xoroshiro {
	lo, hi := md5Seed(name)
	return newXoroshiroState(lo, hi)
}
