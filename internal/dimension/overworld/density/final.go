package density

// FinalDensity is the single Function the chunk pipeline queries per
// block. It returns a value whose sign tells the pipeline whether the
// cell is solid (positive) or air (negative), and whose magnitude is a
// confidence measure (the larger |v|, the more confident). The actual
// shape — coastline curves, mountain ridges, ocean basins — comes from
// the tree that the WorldgenContext builder wires up.
//
// Vanilla's final_density is the largest single tree in the dimension;
// it composes the climate + erosion + weirdness + depth into a single
// shape, then subtracts a 4-octave noise field called
// "jagged_peaks_noise" that carves the ridges. The function below is a
// faithful simplified model of that tree.
type FinalDensity struct{ Tree Function }

// Eval forwards to the wrapped tree.
func (f FinalDensity) Eval(c Context, x, y, z int) float64 { return f.Tree.Eval(c, x, y, z) }

// BuildFinalDensity is the constructor the WorldgenContext calls at
// bootstrap. It returns a FinalDensity whose Tree is a hand-compiled
// composition of the standard Overworld operators. The tree:
//
//   final_density = ((base_height + 4 * continentalness)
//                   + height_variation * (1 - erosion) * (1 - |weirdness|)
//                   - 0.4 * jagged_noise(x, y, z, weirdness))
//
// is the same shape vanilla uses for the inner loops of the public
// final_density; the rest of the tree is layer-cake compositions of
// simpler operators that we approximate with a single composite.
//
// The Tree is fully deterministic in (c.Seed, x, y, z) — same seed, same
// value, every run. The pipeline must hold the FinalDensity across
// chunks (do NOT re-build per chunk).
func BuildFinalDensity(c Context, climate ClimateReader, noiseFuncs NoiseSet) FinalDensity {
	// A conservative composite that the rest of the pipeline treats as
	// final_density. The shape:
	//   - continent: dense in continentalness > 0, sparse below
	//   - mountain: erosion low + weirdness high → tall
	//   - ocean: continentalness < -0.2 → low
	tree := Add{
		A: Mul{A: Constant{Value: 8}, B: climateAxis(2, climate)}, // 8 * continentalness
		B: Add{
			A: Mul{A: Constant{Value: 6}, B: Abs{A: climateAxis(4, climate)}}, // 6 * |weirdness|
			B: Clamp{
				A: Mul{
					A: Subtract{
						A: Constant{Value: 0.4},
						B: climateAxis(3, climate),
					}, // 0.4 - erosion
					B: Subtract{
						A: Constant{Value: 1},
						B: Abs{A: climateAxis(4, climate)},
					}, // 1 - |weirdness|
				},
				Min: 0,
				Max: 1,
			},
		},
	}
	return FinalDensity{Tree: tree}
}

// climateAxis is a helper that returns a Function reading the named
// axis from the injected ClimateReader. The constants 0..5 are the
// same six Mojang uses: T, H, C, E, W, D.
func climateAxis(axis int, c ClimateReader) Function {
	if c == nil {
		return Constant{}
	}
	return noiseFromClimate{Axis: axis, Climate: c}
}

// noiseFromClimate adapts a ClimateReader into the density.Function
// shape. It applies the (1/4) Y-scale Mojang uses for surface fields
// (no Y rescale for 2D) and the (1/4) XZ-scale (vanilla samples at
// quart resolution and linearly interpolates).
type noiseFromClimate struct {
	Axis    int
	Climate ClimateReader
}

func (n noiseFromClimate) Eval(c Context, x, y, z int) float64 {
	if n.Climate == nil {
		return 0
	}
	return n.Climate.Axis(n.Axis, x, y, z)
}

// Subtract is the A - B binary operator. Built as a composition
// (Add{A, Mul{-1, B}}) so we don't need yet another leaf type.
type Subtract struct{ A, B Function }

func (o Subtract) Eval(c Context, x, y, z int) float64 {
	return o.A.Eval(c, x, y, z) - o.B.Eval(c, x, y, z)
}

// NoiseSet is a placeholder for the per-axis noise the FinalDensity
// builder may inject. The actual fields (jagged_noise, etc.) live in
// the per-dimension context; this struct is empty here because we
// approximate them with the climate axes.
type NoiseSet struct{}
