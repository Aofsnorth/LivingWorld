package noise

// NormalNoise is the unit-distribution noise sampler Mojang uses for the
// density / climate routers. Vanilla's reference is
// net.minecraft.world.level.levelgen.synth.NormalNoise, which combines TWO
// PerlinNoise samplers — first and second — with a (-1, 1) clamping that
// guarantees the output stays in [-1, 1] even at extreme coordinates
// (where the raw Perlin field can drift outside the theoretical range
// because of double rounding).
//
// The two-sampler shape is what the blueprint calls out: every Overworld
// router entry that needs a noise field (temperature, humidity,
// continentalness, erosion, weirdness, depth) reads from one NormalNoise.
// Each parameter gets its own seed and frequency, so the router is built
// by composing six NormalNoise instances — never by sharing one.
type NormalNoise struct {
	first   *Perlin
	second  *Perlin
	factor  float64 // the (-1, 1) clamp boundary; vanilla uses sqrt(3) for 3D
	xzScale float64 // coordinate scale applied to X/Z before sampling
	yScale  float64 // coordinate scale applied to Y before sampling (3D only)
}

// NewNormalNoise builds a NormalNoise from the two perlin seeds Mojang's
// JsonOpsNormalNoise requires. factor is the clamp boundary; pass the
// dimension's noise_settings default (sqrt(3) for 3D fields, 1 for 2D).
func NewNormalNoise(firstSeed, secondSeed int64, xzScale, yScale, factor float64) *NormalNoise {
	return &NormalNoise{
		first:   NewPerlin(firstSeed),
		second:  NewPerlin(secondSeed),
		factor:  factor,
		xzScale: xzScale,
		yScale:  yScale,
	}
}

// Sample returns the normal-noise value at (x, y, z). Vanilla's reference
// implementation clamps the Perlin sum into [-factor, factor] before
// normalising, so the output range is exactly [-1, 1].
func (n *NormalNoise) Sample(x, y, z float64) float64 {
	sx, sy, sz := x*n.xzScale, y*n.yScale, z*n.xzScale
	a := n.first.Noise3D(sx, sy, sz)
	b := n.second.Noise3D(sx, sy, sz)
	sum := a + b
	if sum < -n.factor {
		sum = -n.factor
	} else if sum > n.factor {
		sum = n.factor
	}
	return sum / n.factor
}

// Sample2D is the convenience wrapper for noise sources that don't need a
// Y dimension (the climate samplers use 2D fields; the density router
// uses 3D).
func (n *NormalNoise) Sample2D(x, z float64) float64 { return n.Sample(x, 0, z) }
