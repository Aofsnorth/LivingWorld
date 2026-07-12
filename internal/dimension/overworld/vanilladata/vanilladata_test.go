package vanilladata

import (
	"testing"
)

func TestLoadDensityFunction(t *testing.T) {
	data, ok := LoadDensityFunction("overworld/base_3d_noise")
	if !ok {
		t.Fatalf("expected base_3d_noise to exist")
	}
	if len(data) == 0 {
		t.Fatalf("expected non-empty data")
	}
}

func TestLoadNoiseSettings(t *testing.T) {
	data, ok := LoadNoiseSettings("overworld")
	if !ok {
		t.Fatalf("expected overworld noise settings to exist")
	}
	if len(data) == 0 {
		t.Fatalf("expected non-empty data")
	}
}

func TestLoadNoise(t *testing.T) {
	data, ok := LoadNoise("temperature")
	if !ok {
		t.Fatalf("expected temperature noise to exist")
	}
	if len(data) == 0 {
		t.Fatalf("expected non-empty data")
	}
}

func TestLoadStructure(t *testing.T) {
	data, ok := LoadStructure("village/plains/houses/plains_small_house_1")
	if !ok {
		t.Fatalf("expected small house structure to exist")
	}
	if len(data) == 0 {
		t.Fatalf("expected non-empty data")
	}
}

func TestLoadersReturnDefensiveCopies(t *testing.T) {
	cases := []struct {
		name string
		load func() ([]byte, bool)
	}{
		{"density", func() ([]byte, bool) { return LoadDensityFunction("overworld/base_3d_noise") }},
		{"noise settings", func() ([]byte, bool) { return LoadNoiseSettings("overworld") }},
		{"noise", func() ([]byte, bool) { return LoadNoise("temperature") }},
		{"structure", func() ([]byte, bool) { return LoadStructure("village/plains/houses/plains_small_house_1") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, ok := tc.load()
			if !ok {
				t.Fatalf("expected data to exist")
			}
			b, _ := tc.load()
			if len(a) == 0 {
				t.Fatalf("expected non-empty data")
			}
			a[0] = ^a[0]
			if a[0] == b[0] {
				t.Fatalf("%s returned the same backing array", tc.name)
			}
		})
	}
}

func TestLoadMissingKey(t *testing.T) {
	if _, ok := LoadDensityFunction("overworld/does_not_exist"); ok {
		t.Fatalf("expected missing density function to return false")
	}
	if _, ok := LoadNoiseSettings("does_not_exist"); ok {
		t.Fatalf("expected missing noise settings to return false")
	}
	if _, ok := LoadNoise("does_not_exist"); ok {
		t.Fatalf("expected missing noise to return false")
	}
	if _, ok := LoadStructure("does/not/exist"); ok {
		t.Fatalf("expected missing structure to return false")
	}
}
