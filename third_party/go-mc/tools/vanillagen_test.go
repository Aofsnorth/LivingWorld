package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateVanilladata(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "density_function", "overworld"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "density_function", "overworld", "base_3d_noise.json"), []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := GenerateVanilladata(src, dst); err != nil {
		t.Fatalf("GenerateVanilladata: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "data.go")); os.IsNotExist(err) {
		t.Fatal("expected data.go")
	}
}
