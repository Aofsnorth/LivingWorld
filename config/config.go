package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

type Config struct {
	ServerName string `yaml:"serverName"`
	MOTD       string `yaml:"motd"`
	PluginsDir string `yaml:"pluginsDir"`

	World WorldConfig `yaml:"world"`
	Java  JavaConfig  `yaml:"java"`
	Bedrock BedrockConfig `yaml:"bedrock"`
}

type WorldConfig struct {
	Type  string `yaml:"type"` // superflat
	Seed  int64  `yaml:"seed"`
	Spawn SpawnConfig `yaml:"spawn"`
}

type SpawnConfig struct {
	X float64 `yaml:"x"`
	Y float64 `yaml:"y"`
	Z float64 `yaml:"z"`
	Yaw   float32 `yaml:"yaw"`
	Pitch float32 `yaml:"pitch"`
}

type JavaConfig struct {
	Port               int    `yaml:"port"`
	OnlineMode         bool   `yaml:"onlineMode"`
	MaxPlayers         int    `yaml:"maxPlayers"`
	ViewDistance       int    `yaml:"viewDistance"`
	SimulationDistance int    `yaml:"simulationDistance"`
	Bind               string `yaml:"bind"`
}

type BedrockConfig struct {
	Port         int    `yaml:"port"`
	MaxPlayers   int    `yaml:"maxPlayers"`
	ViewDistance int    `yaml:"viewDistance"`
	Bind         string `yaml:"bind"`
	AuthDisabled bool   `yaml:"authDisabled"`
}

func Default() *Config {
	return &Config{
		ServerName: "LivingWorld Server",
		MOTD:       "A Minecraft Server",
		PluginsDir: "./plugins",
		World: WorldConfig{
			Type: "superflat",
			Seed: 12345,
			Spawn: SpawnConfig{X: 0, Y: 4, Z: 0, Yaw: 0, Pitch: 0},
		},
		Java: JavaConfig{
			Bind:               "0.0.0.0",
			Port:               25565,
			OnlineMode:         false,
			MaxPlayers:         100,
			ViewDistance:       10,
			SimulationDistance: 10,
		},
		Bedrock: BedrockConfig{
			Bind:         "0.0.0.0",
			Port:         19132,
			MaxPlayers:   100,
			ViewDistance: 8,
			AuthDisabled: true,
		},
	}
}

func (c *Config) Address() string {
	return fmt.Sprintf("%s:%d", c.Java.Bind, c.Java.Port)
}

func (c *Config) BedrockAddress() string {
	return fmt.Sprintf("%s:%d", c.Bedrock.Bind, c.Bedrock.Port)
}

// Load reads YAML config from path. If file is missing, it returns defaults.
// Environment variables override file/default values:
// - LIVINGWORLD_SERVER_NAME
// - LIVINGWORLD_JAVA_PORT
// - LIVINGWORLD_BEDROCK_PORT
// - LIVINGWORLD_PLUGINS_DIR
func Load(path string) (*Config, error) {
	cfg := Default()

	b, err := os.ReadFile(path)
	if err == nil {
		if err := yaml.Unmarshal(b, cfg); err != nil {
			return nil, fmt.Errorf("parse yaml %s: %w", path, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	if v := os.Getenv("LIVINGWORLD_SERVER_NAME"); v != "" {
		cfg.ServerName = v
	}
	if v := os.Getenv("LIVINGWORLD_PLUGINS_DIR"); v != "" {
		cfg.PluginsDir = v
	}
	if v := os.Getenv("LIVINGWORLD_JAVA_PORT"); v != "" {
		if p, e := strconv.Atoi(v); e == nil {
			cfg.Java.Port = p
		}
	}
	if v := os.Getenv("LIVINGWORLD_BEDROCK_PORT"); v != "" {
		if p, e := strconv.Atoi(v); e == nil {
			cfg.Bedrock.Port = p
		}
	}
	return cfg, nil
}
