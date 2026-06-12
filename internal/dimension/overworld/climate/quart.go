package climate

import "livingworld/internal/dimension/overworld/biome"

// InterpolateQuart bilinearly interpolates a 4×4 climate grid to a per-
// block point. lx, lz are block-local coordinates in [0, 15]. The
// returned ClimatePoint is what the surface rules stage and the per-block
// biome grid should use.
//
// Vanilla does this with quart Y too (the Y axis is also 4-block quart);
// we keep the 2D interpolation here because surface rules only read
// (T, H, C, E, W), and the Y signal is consumed by the surface rule's
// above_preliminary_surface / stone_depth gates.
func InterpolateQuart(g QuartGrid, lx, lz int) ClimatePoint {
	// Block cell index inside the 4×4 grid.
	fx := float64(lx) / 4.0
	fz := float64(lz) / 4.0
	x0 := int(fx)
	z0 := int(fz)
	x1 := x0 + 1
	if x1 > 3 {
		x1 = 3
	}
	z1 := z0 + 1
	if z1 > 3 {
		z1 = 3
	}
	tx := fx - float64(x0)
	tz := fz - float64(z0)
	p00 := g[z0][x0]
	p10 := g[z0][x1]
	p01 := g[z1][x0]
	p11 := g[z1][x1]
	blend := func(a, b, t float64) float64 { return a + (b-a)*t }
	return ClimatePoint{
		Temperature:    blend(blend(p00.Temperature, p10.Temperature, tx), blend(p01.Temperature, p11.Temperature, tx), tz),
		Humidity:       blend(blend(p00.Humidity, p10.Humidity, tx), blend(p01.Humidity, p11.Humidity, tx), tz),
		Continentalness: blend(blend(p00.Continentalness, p10.Continentalness, tx), blend(p01.Continentalness, p11.Continentalness, tx), tz),
		Erosion:        blend(blend(p00.Erosion, p10.Erosion, tx), blend(p01.Erosion, p11.Erosion, tx), tz),
		Weirdness:      blend(blend(p00.Weirdness, p10.Weirdness, tx), blend(p01.Weirdness, p11.Weirdness, tx), tz),
		Depth:          blend(blend(p00.Depth, p10.Depth, tx), blend(p01.Depth, p11.Depth, tx), tz),
	}
}

// PickBiomeAt is the convenience used by the chunk pipeline. It samples
// the grid at the given local block coordinate and dispatches to
// PickBiome.
func PickBiomeAt(g QuartGrid, lx, lz int) biome.Parameters {
	return PickBiome(InterpolateQuart(g, lx, lz))
}
