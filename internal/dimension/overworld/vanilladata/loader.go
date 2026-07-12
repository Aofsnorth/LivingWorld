package vanilladata

func LoadDensityFunction(name string) ([]byte, bool) {
	v, ok := DensityFunctions[name]
	if !ok {
		return nil, false
	}
	return []byte(v), true
}

func LoadNoiseSettings(name string) ([]byte, bool) {
	v, ok := NoiseSettings[name]
	if !ok {
		return nil, false
	}
	return []byte(v), true
}

func LoadNoise(name string) ([]byte, bool) {
	v, ok := Noises[name]
	if !ok {
		return nil, false
	}
	return []byte(v), true
}

func LoadStructure(path string) ([]byte, bool) {
	v, ok := Structures[path]
	if !ok {
		return nil, false
	}
	return v, true
}
