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

func TestLoadStructure(t *testing.T) {
	data, ok := LoadStructure("village/plains/houses/plains_small_house_1")
	if !ok {
		t.Fatalf("expected small house structure to exist")
	}
	if len(data) == 0 {
		t.Fatalf("expected non-empty data")
	}
}
