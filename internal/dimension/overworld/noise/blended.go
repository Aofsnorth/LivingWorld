package noise

import "math"

// BlendedNoise is the legacy 3D terrain noise used by minecraft:old_blended_noise.
// It wraps three PerlinNoise octave stacks (minLimit, maxLimit, main) and
// blends them according to the value produced by the main octave stack.
type BlendedNoise struct {
	minLimitNoise, maxLimitNoise, mainNoise *PerlinNoise
	xzMultiplier, yMultiplier               float64
	xzFactor, yFactor                       float64
	smearScaleMultiplier                    float64
	MaxValue                                float64
}

// NewBlendedNoise builds a BlendedNoise for either legacy or modern terrain.
// The caller supplies the seed-derived random source and the five parameters
// from the JSON definition.
func NewBlendedNoise(r PRNG, xzScale, yScale, xzFactor, yFactor, smearScaleMultiplier float64) *BlendedNoise {
	minAmplitudes := make([]float64, 16)
	maxAmplitudes := make([]float64, 16)
	mainAmplitudes := make([]float64, 8)
	for i := range minAmplitudes {
		minAmplitudes[i] = 1.0
		maxAmplitudes[i] = 1.0
	}
	for i := range mainAmplitudes {
		mainAmplitudes[i] = 1.0
	}

	bn := &BlendedNoise{
		minLimitNoise:        NewPerlinNoiseLegacy(r, -15, minAmplitudes),
		maxLimitNoise:        NewPerlinNoiseLegacy(r, -15, maxAmplitudes),
		mainNoise:            NewPerlinNoiseLegacy(r, -7, mainAmplitudes),
		xzMultiplier:         684.412 * xzScale,
		yMultiplier:          684.412 * yScale,
		xzFactor:             xzFactor,
		yFactor:              yFactor,
		smearScaleMultiplier: smearScaleMultiplier,
	}
	bn.MaxValue = bn.minLimitNoise.EdgeValue(bn.yMultiplier + 2)
	return bn
}

// Sample returns the blended noise value at the block coordinate (x, y, z).
func (bn *BlendedNoise) Sample(x, y, z float64) float64 {
	scaledX := x * bn.xzMultiplier
	scaledY := y * bn.yMultiplier
	scaledZ := z * bn.xzMultiplier

	factoredX := scaledX / bn.xzFactor
	factoredY := scaledY / bn.yFactor
	factoredZ := scaledZ / bn.xzFactor

	smear := bn.yMultiplier * bn.smearScaleMultiplier
	factoredSmear := smear / bn.yFactor

	value := 0.0
	factor := 1.0
	for i := 0; i < 8; i++ {
		noise := bn.mainNoise.GetOctaveNoise(i)
		if noise != nil {
			value += noise.sample(
				perlinWrap(factoredX*factor),
				perlinWrap(factoredY*factor),
				perlinWrap(factoredZ*factor),
				factoredSmear*factor,
				factoredY*factor,
			) / factor
		}
		factor /= 2
	}

	value = (value/10 + 1) / 2

	factor = 1.0
	minVal := 0.0
	maxVal := 0.0
	for i := 0; i < 16; i++ {
		xx := perlinWrap(scaledX * factor)
		yy := perlinWrap(scaledY * factor)
		zz := perlinWrap(scaledZ * factor)
		smearsmear := smear * factor
		yLimit := scaledY * factor

		if value < 1 {
			noise := bn.minLimitNoise.GetOctaveNoise(i)
			if noise != nil {
				minVal += noise.sample(xx, yy, zz, smearsmear, yLimit) / factor
			}
		}
		if value > 0 {
			noise := bn.maxLimitNoise.GetOctaveNoise(i)
			if noise != nil {
				maxVal += noise.sample(xx, yy, zz, smearsmear, yLimit) / factor
			}
		}
		factor /= 2
	}

	return clampedLerp(minVal/512, maxVal/512, value) / 128
}

func clampedLerp(a, b, t float64) float64 {
	if t < 0 {
		return a
	}
	if t > 1 {
		return b
	}
	return a + (b-a)*t
}

func clamp(v, min, max float64) float64 {
	return math.Min(math.Max(v, min), max)
}
