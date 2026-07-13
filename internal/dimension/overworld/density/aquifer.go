package density

// AquiferFields holds the four noise fields Mojang routes through the
// aquifer. They are evaluated per (x, y, z) by the aquifer stage.
type AquiferFields struct {
	Barrier               Function
	FluidLevelFloodedness Function
	FluidLevelSpread      Function
	Lava                  Function
}

// BuildAquiferFields returns the standard Overworld aquifer tree using the
// noise parameters baked into the world context.
func BuildAquiferFields(c Context) AquiferFields {
	if c.Noises == nil {
		return AquiferFields{Barrier: Constant{}, FluidLevelFloodedness: Constant{}, FluidLevelSpread: Constant{Value: 1}, Lava: Constant{}}
	}
	return AquiferFields{
		Barrier:               Noise{Noise: "aquifer_barrier", XZScale: 1.0, YScale: 0.5},
		FluidLevelFloodedness: Noise{Noise: "aquifer_fluid_level_floodedness", XZScale: 1.0, YScale: 0.5},
		FluidLevelSpread:      Add{A: Noise{Noise: "aquifer_fluid_level_spread", XZScale: 1.0, YScale: 0.5}, B: Constant{Value: 1}},
		Lava:                  lavaGate{},
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
