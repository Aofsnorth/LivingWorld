package density

// FinalDensity is the single Function the chunk pipeline queries per block.
// It returns a value whose sign tells the pipeline whether the cell is solid
// (positive) or air (negative). The shape is the full vanilla final_density
// tree, parsed from the noise_settings JSON.
type FinalDensity struct{ Tree Function }

// Eval forwards to the wrapped tree.
func (f FinalDensity) Eval(c Context, x, y, z int) float64 { return f.Tree.Eval(c, x, y, z) }
