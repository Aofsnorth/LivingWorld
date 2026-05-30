package plugin

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Manifest is a parsed plugin.yml: identity plus load requirements.
type Manifest struct {
	Name        string   `yaml:"name"`
	Version     string   `yaml:"version"`
	APIVersion  string   `yaml:"api-version"`
	Entrypoint  string   `yaml:"entrypoint"`
	Depends     []string `yaml:"depends"`      // hard deps: must be present, load first
	SoftDepends []string `yaml:"soft-depends"` // optional: load first only if present
	Permissions []string `yaml:"permissions"`
}

// LoadManifest reads and validates a plugin.yml file.
func LoadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if m.Name == "" || m.Version == "" {
		return nil, fmt.Errorf("%s: name and version are required", path)
	}
	return &m, nil
}

// ResolveOrder topologically sorts manifests so every plugin loads after its
// dependencies. It errors on a missing hard dependency or a dependency cycle.
func ResolveOrder(manifests []*Manifest) ([]*Manifest, error) {
	byName := make(map[string]*Manifest, len(manifests))
	for _, m := range manifests {
		byName[m.Name] = m
	}
	const (
		visiting = 1
		done     = 2
	)
	state := make(map[string]int, len(manifests))
	var order []*Manifest
	var visit func(m *Manifest) error
	visit = func(m *Manifest) error {
		switch state[m.Name] {
		case visiting:
			return fmt.Errorf("dependency cycle at %q", m.Name)
		case done:
			return nil
		}
		state[m.Name] = visiting
		for _, dep := range m.Depends {
			d, ok := byName[dep]
			if !ok {
				return fmt.Errorf("%q: missing dependency %q", m.Name, dep)
			}
			if err := visit(d); err != nil {
				return err
			}
		}
		for _, dep := range m.SoftDepends {
			if d, ok := byName[dep]; ok {
				if err := visit(d); err != nil {
					return err
				}
			}
		}
		state[m.Name] = done
		order = append(order, m)
		return nil
	}
	for _, m := range manifests {
		if err := visit(m); err != nil {
			return nil, err
		}
	}
	return order, nil
}
