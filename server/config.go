package server

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml"
	"gopkg.in/yaml.v3"

	"livingworld/config"
)

// Config is the server configuration. It is an alias of the YAML config type, so
// it can be built in code or loaded from a file via LoadConfig.
type Config = config.Config

// DefaultConfig returns a ready-to-run configuration (superflat world, offline
// mode, persistence enabled).
func DefaultConfig() *Config { return config.Default() }

// LoadConfig reads configuration from a YAML file, falling back to defaults for
// any missing fields.
func LoadConfig(path string) (*Config, error) { return config.Load(path) }

// LoadConfigFile loads server configuration from a YAML (.yml/.yaml) or TOML
// (.toml) file, layered over DefaultConfig. The format is chosen by the file
// extension. A missing file is not an error: the defaults are returned.
//
// TOML is decoded to a generic map and re-encoded as YAML so the single set of
// `yaml` struct tags on config.Config drives both formats (keys are identical,
// e.g. serverName, world.type, java.port).
//
// Ops and the whitelist live in their own files; see LoadOps and LoadWhitelist.
func LoadConfigFile(path string) (*Config, error) {
	cfg := config.Default()

	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	doc := b
	if strings.EqualFold(filepath.Ext(path), ".toml") {
		tree, err := toml.LoadBytes(b)
		if err != nil {
			return nil, fmt.Errorf("parse toml %s: %w", path, err)
		}
		if doc, err = yaml.Marshal(tree.ToMap()); err != nil {
			return nil, fmt.Errorf("convert toml %s: %w", path, err)
		}
	}

	if err := yaml.Unmarshal(doc, cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	return cfg, nil
}
