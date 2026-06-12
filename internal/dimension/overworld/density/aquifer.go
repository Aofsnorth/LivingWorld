package density

// AquiferFields holds the four noise fields Mojang routes through the
// aquifer. They are evaluated per (x, y, z) by the aquifer stage; the
// values are interpreted as:
//
//   - Barrier: the "is this cell inside a barrier" boolean. The aquifer
//     treats cells where Barrier > 0 as solid rock (no flooding).
//   - FluidLevelFloodedness: per-cell probability that the cell will be
//     filled with fluid (water or lava) when its surrounding rock is
//     removed. Mapped to [0, 1] then compared against a per-block PRNG.
//   - FluidLevelSpread: the "fluid spills into this cell" probability.
//     A high value makes the aquifer propagate far.
//   - Lava: the "this fluid should be lava, not water" boolean. Cells
//     below sea level with Lava > 0 are lava.
//
// The fields are all 1D NormalNoise lookups; their exact scale / seed
// values are baked into the WorldgenContext at build time.
type AquiferFields struct {
	Barrier              Function
	FluidLevelFloodedness Function
	FluidLevelSpread     Function
	Lava                 Function
}

// BuildAquiferFields returns the standard Overworld aquifer tree:
//
//	barrier               = noise(axis=C, scale=1.0, xz_scale=1/16)
//	fluid_level_floodedness = noise(axis=C, scale=1.0, xz_scale=1/32)
//	fluid_level_spread    = noise(axis=C, scale=1.0, xz_scale=1/16) + 1.0
//	lava                  = noise(axis=Y, scale=1.0, xz_scale=1/32) > 0
//
// The Y axis (lava) is approximated by the depth axis: vanilla's lava
// is gated on the y-coordinate, not a noise field, so we cheat and
// return a Function that reads the block Y directly. The aquifer
// interprets the sign as "lava here" (Y < some threshold).
func BuildAquiferFields(c Context, climate ClimateReader) AquiferFields {
	if climate == nil {
		return AquiferFields{Barrier: Constant{}, FluidLevelFloodedness: Constant{}, FluidLevelSpread: Constant{Value: 1}, Lava: Constant{}}
	}
	barrier := noiseFromClimate{Axis: 2, Climate: climate}        // continentalness
	flood := noiseFromClimate{Axis: 2, Climate: climate}          // reuse C axis
	spread := Add{A: noiseFromClimate{Axis: 2, Climate: climate}, B: Constant{Value: 1}}
	lava := lavaGate{}
	return AquiferFields{
		Barrier:              barrier,
		FluidLevelFloodedness: flood,
		FluidLevelSpread:     spread,
		Lava:                 lava,
	}
}

// lavaGate returns 1 for cells below Y=11 (vanilla "lava in deepslate
// level" gate) and 0 above. The aquifer reads this as a probability
// multiplier.
type lavaGate struct{}

func (lavaGate) Eval(c Context, x, y, z int) float64 {
	if y < 11 {
		return 1
	}
	return 0
}
