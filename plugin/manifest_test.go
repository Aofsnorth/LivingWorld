package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadManifest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plugin.yml")
	if err := os.WriteFile(path, []byte("name: greeter\nversion: 1.0.0\napi-version: \"26\"\ndepends: [core]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if m.Name != "greeter" || m.Version != "1.0.0" || m.APIVersion != "26" {
		t.Fatalf("unexpected manifest: %+v", m)
	}
	if len(m.Depends) != 1 || m.Depends[0] != "core" {
		t.Fatalf("depends = %v", m.Depends)
	}

	bad := filepath.Join(dir, "bad.yml")
	if err := os.WriteFile(bad, []byte("version: 1.0.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadManifest(bad); err == nil {
		t.Fatal("expected error for manifest missing name")
	}
}

func TestResolveOrder(t *testing.T) {
	// b depends on a, c depends on b → a, b, c
	manifests := []*Manifest{
		{Name: "c", Version: "1", Depends: []string{"b"}},
		{Name: "a", Version: "1"},
		{Name: "b", Version: "1", Depends: []string{"a"}},
	}
	order, err := ResolveOrder(manifests)
	if err != nil {
		t.Fatalf("ResolveOrder: %v", err)
	}
	pos := map[string]int{}
	for i, m := range order {
		pos[m.Name] = i
	}
	if !(pos["a"] < pos["b"] && pos["b"] < pos["c"]) {
		t.Fatalf("bad order: %v", names(order))
	}

	if _, err := ResolveOrder([]*Manifest{{Name: "x", Depends: []string{"missing"}}}); err == nil {
		t.Fatal("expected missing-dependency error")
	}
	cycle := []*Manifest{
		{Name: "p", Depends: []string{"q"}},
		{Name: "q", Depends: []string{"p"}},
	}
	if _, err := ResolveOrder(cycle); err == nil {
		t.Fatal("expected cycle error")
	}
}

func names(ms []*Manifest) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.Name
	}
	return out
}
