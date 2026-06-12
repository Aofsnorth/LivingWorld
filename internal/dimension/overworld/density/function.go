// Package density is LivingWorld's density function interpreter — the
// arithmetic that turns climate fields into terrain shape. It mirrors
// vanilla's net.minecraft.world.level.levelgen.DensityFunction tree
// (a JSON-described arithmetic DAG) and is what the blueprint calls the
// "final_density router".
//
// For the Overworld the runtime tree is a single compile-once object built
// at WorldgenContext bootstrap; every per-chunk evaluation reuses the
// compiled tree. The interpreter supports the operators Mojang uses in
// the Overworld preset:
//
//   - Constant, LinearInterpolated, FlatCache
//   - Noise, ShiftedNoise (the temperature/humidity/etc. routed
//     noise forms)
//   - Abs, Square, Cube, HalfNegative, QuarterNegative, Invert
//   - Squeeze (the special "stretch into [-1, 1]" operator)
//   - Add, Mul, Min, Max, BlendAlpha, BlendOffset
//   - Clamp, RangeChoice, Spline
//
// Every operator is a small interface implementation; new operators slot
// in by satisfying the Function interface and registering in
// registry.go. The compile step (compile.go) is what turns a JSON-ish
// descriptor into a Function tree.
//
// The aquifer fields (barrier, fluid_level_floodedness, fluid_level_spread,
// lava) are themselves DensityFunction trees whose values per (x, y, z)
// drive the aquifer's flood/spread rule. vein_toggle / vein_ridged /
// vein_gap are tiny 1D fields that the ore veinifier reads.
package density

// Function is one node in a density function tree. It takes a context
// (block position + chunk) and returns a single float64 value. Tree
// evaluation is recursive but cheap: the tree is small (the Overworld
// final_density has ~30 nodes) and per-block calls are the inner loop.
type Function interface {
	// Eval returns the function value at (x, y, z). x and z are block
	// coordinates; y is world Y. The Context is the per-call state
	// (the world seed + the noise-router table). Most operators ignore
	// the context and use only (x, y, z).
	Eval(c Context, x, y, z int) float64
}

// Context carries the per-call state every Function may read. The
// minimum useful context is the noise router (six NormalNoise fields
// for the climate axes). For our purposes the struct is empty — we pass
// a pointer so the interface is future-proof.
type Context struct {
	// Seed is the world seed. Functions that need deterministic noise
	// (Noise, ShiftedNoise) seed their Perlin off this.
	Seed int64
	// Climate, if non-nil, lets Noise operators read the appropriate
	// axis without having to know the field's id. Optional.
	Climate ClimateReader
}

// ClimateReader is the climate interface Noise operators consume. It
// exists so density.Function doesn't have to import climate (cycle).
type ClimateReader interface {
	// Axis returns the climate value for the given axis index at (x, y, z).
	// Index 0=temperature, 1=humidity, 2=continentalness, 3=erosion,
	// 4=weirdness, 5=depth. Vanilla's Climate.Sample() routes this.
	Axis(axis int, x, y, z int) float64
}
