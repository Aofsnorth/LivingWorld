package vanilladata

// clone returns a defensive copy of b. The generated maps in this package are
// immutable source data; callers must not be able to corrupt them by mutating
// the returned slice.
func clone(b []byte) []byte {
	if b == nil {
		return nil
	}
	c := make([]byte, len(b))
	copy(c, b)
	return c
}

func LoadDensityFunction(name string) ([]byte, bool) {
	v, ok := DensityFunctions[name]
	if !ok {
		return nil, false
	}
	return clone(v), true
}

func LoadNoiseSettings(name string) ([]byte, bool) {
	v, ok := NoiseSettings[name]
	if !ok {
		return nil, false
	}
	return clone(v), true
}

func LoadNoise(name string) ([]byte, bool) {
	v, ok := Noises[name]
	if !ok {
		return nil, false
	}
	return clone(v), true
}

func LoadStructure(path string) ([]byte, bool) {
	v, ok := Structures[path]
	if !ok {
		return nil, false
	}
	return clone(v), true
}
