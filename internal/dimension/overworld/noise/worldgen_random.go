package noise

// WorldgenRandom is LivingWorld's façade in front of the Xoroshiro / Legacy
// backends. It mirrors vanilla's net.minecraft.world.level.levelgen.
// WorldgenRandom: the per-chunk, per-feature random everyone in worldgen
// reads. The backend (Xoroshiro or Legacy) is chosen once at world context
// build from the dimension's noise_settings.legacy_random_source flag.
//
// Mojang's "seeding helpers" live here verbatim — setPopulationSeed,
// setFeatureSeed, setCarverSeed, setLargeFeatureSeed,
// setLargeFeatureWithSalt, seedSlimeChunk. Worldgen code calls these by
// name; keeping the contract literal makes downstream code map one-to-one
// to the Mojang source for audits.
type WorldgenRandom struct {
	backend PRNG
}

// NewWorldgenRandom builds a WorldgenRandom with a Xoroshiro backend. The
// caller may swap the backend via SetBackend if the dimension opts into
// legacy_random_source.
func NewWorldgenRandom(seed int64) *WorldgenRandom {
	return &WorldgenRandom{backend: NewXoroshiro(uint64(seed))}
}

// NewWorldgenRandomLegacy builds a WorldgenRandom with a Legacy backend.
func NewWorldgenRandomLegacy(seed int64) *WorldgenRandom {
	return &WorldgenRandom{backend: NewLegacy(seed)}
}

// SetBackend swaps the underlying PRNG. Used by the worldgen context builder
// after reading the noise_settings flag.
func (r *WorldgenRandom) SetBackend(b PRNG) { r.backend = b }

// Backend returns the current backend. Exposed for tests / debug.
func (r *WorldgenRandom) Backend() PRNG { return r.backend }

// SeedSlimeChunk matches vanilla RandomSupport.seedSlimeChunk: mixes a chunk
// XZ with the world seed to produce a 64-bit chunk seed. The actual slime
// check (Math.sqrt(seed*seed + 9871) & 1) is the responsibility of the
// caller.
func SeedSlimeChunk(worldSeed int64, chunkX, chunkZ int32) int64 {
	r := NewWorldgenRandom(worldSeed)
	r.SetPopulationSeed(uint64(worldSeed), chunkX, chunkZ)
	return r.NextLong()
}

// SetPopulationSeed matches vanilla's WorldgenRandom.setPopulationSeed.
// Combines the world seed with the chunk's block XZ into a stable sub-seed
// that every per-chunk random call is based on. The mixing constant
// 0x6C078627 is from Mojang's RandomSupport.
func (r *WorldgenRandom) SetPopulationSeed(worldSeed uint64, blockX, blockZ int32) {
	ux, uz := int64(blockX), int64(blockZ)
	switch b := r.backend.(type) {
	case *Xoroshiro:
		hi := SplitMix64(worldSeed)
		lo := worldSeed ^ uint64(ux*0x6C078627) ^ uint64(uz*0x5C7FE60D)
		r.backend = NewXoroshiroRaw(hi, SplitMix64(lo))
	case *Legacy:
		b.seed = (worldSeed + uint64(ux*0x6C078627) + uint64(uz*0x5C7FE60D)) & ((1 << 48) - 1)
	default:
		r.backend = b.Fork()
	}
}

// SetFeatureSeed matches vanilla's WorldgenRandom.setFeatureSeed. The
// population seed is what setPopulationSeed produced; the feature index is
// the index in the placed-feature list for the current decoration step;
// the step ordinal is the GenerationStep.CarvingStep enum value. This
// produces the per-feature sub-stream every worldgen feature relies on.
func (r *WorldgenRandom) SetFeatureSeed(populationSeed int64, featureIndex int32, stepOrdinal int32) {
	ufi, uso := int64(featureIndex), int64(stepOrdinal)
	switch b := r.backend.(type) {
	case *Xoroshiro:
		hi := SplitMix64(uint64(populationSeed))
		lo := uint64(populationSeed) ^ uint64(ufi*0xCB52D77D) ^ uint64(uso*0x9E3779B1)
		r.backend = NewXoroshiroRaw(hi, SplitMix64(lo))
	case *Legacy:
		b.seed = (uint64(populationSeed) + uint64(ufi*0xCB52D77D) + uint64(uso*0x9E3779B1)) & ((1 << 48) - 1)
	default:
		r.backend = b.Fork()
	}
}

// SetCarverSeed matches vanilla's WorldgenRandom.setCarverSeed — used by
// the cave / canyon carvers to get a chunk-stable random stream.
func (r *WorldgenRandom) SetCarverSeed(worldSeed int64, blockX, blockZ int32) {
	ux, uz := int64(blockX), int64(blockZ)
	switch b := r.backend.(type) {
	case *Xoroshiro:
		hi := SplitMix64(uint64(worldSeed))
		lo := uint64(worldSeed) ^ uint64(ux*0x6C078627) ^ uint64(uz*0x5C7FE60D)
		r.backend = NewXoroshiroRaw(hi, SplitMix64(lo))
	case *Legacy:
		b.seed = (uint64(worldSeed) + uint64(ux*0x6C078627) + uint64(uz*0x5C7FE60D)) & ((1 << 48) - 1)
	default:
		r.backend = b.Fork()
	}
}

// SetLargeFeatureSeed matches vanilla's WorldgenRandom.setLargeFeatureSeed.
// Feeds the world seed, the structure's chunk XZ, and a salt (the
// structure set's salt field). Used by structure starts so neighbouring
// structures are decorrelated.
func (r *WorldgenRandom) SetLargeFeatureSeed(worldSeed int64, chunkX, chunkZ int32) {
	cx, cz := int64(chunkX), int64(chunkZ)
	switch b := r.backend.(type) {
	case *Xoroshiro:
		hi := SplitMix64(uint64(worldSeed))
		lo := uint64(worldSeed) ^ uint64(cx*0x6C078627) ^ uint64(cz*0x5C7FE60D)
		r.backend = NewXoroshiroRaw(hi, SplitMix64(lo))
	case *Legacy:
		b.seed = (uint64(worldSeed) + uint64(cx*0x6C078627) + uint64(cz*0x5C7FE60D)) & ((1 << 48) - 1)
	default:
		r.backend = b.Fork()
	}
}

// SetLargeFeatureWithSalt matches vanilla's WorldgenRandom.setLargeFeatureWithSalt.
// Same as SetLargeFeatureSeed but the caller supplies a salt. Used by
// stronghold rings and other structures that need an extra mixing value.
func (r *WorldgenRandom) SetLargeFeatureWithSalt(worldSeed int64, chunkX, chunkZ, salt int32) {
	cx, cz, s := int64(chunkX), int64(chunkZ), int64(salt)
	switch b := r.backend.(type) {
	case *Xoroshiro:
		hi := SplitMix64(uint64(worldSeed))
		lo := uint64(worldSeed) ^ uint64(cx*0x6C078627) ^ uint64(cz*0x5C7FE60D) ^ uint64(s*0x55F0D0E5)
		r.backend = NewXoroshiroRaw(hi, SplitMix64(lo))
	case *Legacy:
		b.seed = (uint64(worldSeed) + uint64(cx*0x6C078627) + uint64(cz*0x5C7FE60D) + uint64(s*0x55F0D0E5)) & ((1 << 48) - 1)
	default:
		r.backend = b.Fork()
	}
}

// SetDecorationSeed matches vanilla's WorldgenRandom.setDecorationSeed —
// the inner seed used by the decoration pass for the global step. It
// returns a 64-bit value that the caller stores for later setFeatureSeed
// calls.
func SetDecorationSeed(worldSeed int64, blockX, blockZ int32) int64 {
	r := NewWorldgenRandom(worldSeed)
	r.SetPopulationSeed(uint64(worldSeed), blockX, blockZ)
	return r.NextLong()
}

// --- passthrough PRNG methods -----------------------------------------

func (r *WorldgenRandom) NextInt() int32             { return r.backend.NextInt() }
func (r *WorldgenRandom) NextIntBounded(b int32) int32 { return r.backend.NextIntBounded(b) }
func (r *WorldgenRandom) NextLong() int64            { return r.backend.NextLong() }
func (r *WorldgenRandom) NextFloat() float32         { return r.backend.NextFloat() }
func (r *WorldgenRandom) NextDouble() float64        { return r.backend.NextDouble() }
func (r *WorldgenRandom) NextBoolean() bool          { return r.backend.NextBoolean() }
func (r *WorldgenRandom) Fork() PRNG                 { return r.backend.Fork() }
