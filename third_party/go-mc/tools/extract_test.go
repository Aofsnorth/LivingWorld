package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractWorldgenData(t *testing.T) {
	jar := os.Getenv("VANILLA_JAR")
	if jar == "" {
		t.Skip("VANILLA_JAR not set")
	}
	dir := t.TempDir()
	if err := ExtractWorldgenData(jar, dir); err != nil {
		t.Fatalf("ExtractWorldgenData: %v", err)
	}
	for _, f := range []string{
		"density_function/overworld/base_3d_noise.json",
		"noise_settings/overworld.json",
		"noise/temperature.json",
		"structures/village/plains/houses/plains_small_house_1.nbt",
	} {
		_, err := os.Stat(filepath.Join(dir, f))
		if os.IsNotExist(err) {
			t.Fatalf("expected extracted file %s", f)
		}
		if err != nil {
			t.Fatalf("stat %s: %v", f, err)
		}
	}
}
