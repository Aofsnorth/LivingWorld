package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	ServerName string `yaml:"serverName"`
	MOTD       string `yaml:"motd"`
	PluginsDir string `yaml:"pluginsDir"`

	// Ops lists the usernames (case-insensitive) allowed to run operator/cheat
	// commands like /gamemode and /time. Loaded from ops.txt.
	Ops []string `yaml:"-"`

	World   WorldConfig   `yaml:"world"`
	Java    JavaConfig    `yaml:"java"`
	Bedrock BedrockConfig `yaml:"bedrock"`
}

// IsOp reports whether username is in the configured ops list (case-insensitive).
func (c *Config) IsOp(username string) bool {
	for _, op := range c.Ops {
		if strings.EqualFold(op, username) {
			return true
		}
	}
	return false
}

type WorldConfig struct {
	Type  string      `yaml:"type"` // superflat
	Seed  int64       `yaml:"seed"`
	Spawn SpawnConfig `yaml:"spawn"`

	// Persistence controls saving the world to disk. Directory is the base path
	// under which each world gets its own subfolder; AutosaveSeconds is the
	// periodic save interval (0 disables autosave but a final save still runs on
	// shutdown).
	Persistence     bool   `yaml:"persistence"`
	Directory       string `yaml:"directory"`
	AutosaveSeconds int    `yaml:"autosaveSeconds"`

	// Difficulty: "peaceful" | "easy" | "normal" | "hard". Drives mob spawning
	// and damage once the entity system exists.
	Difficulty string `yaml:"difficulty"`

	// DayNightCycle enables the advancing time-of-day (sun/moon movement).
	DayNightCycle bool `yaml:"dayNightCycle"`
}

// DifficultyByte maps the configured difficulty name to the Minecraft 0-3 value
// (peaceful=0, easy=1, normal=2, hard=3). Unknown values default to normal.
func (w WorldConfig) DifficultyByte() byte {
	switch w.Difficulty {
	case "peaceful":
		return 0
	case "easy":
		return 1
	case "hard":
		return 3
	default: // "normal" or unset
		return 2
	}
}

type SpawnConfig struct {
	X     float64 `yaml:"x"`
	Y     float64 `yaml:"y"`
	Z     float64 `yaml:"z"`
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

	// SkinSource decides where to fetch a player's skin in offline mode (when the
	// client doesn't send one):
	//   "auto"  (default) try every source: Ely.by then Mojang
	//   "mojang" premium accounts only
	//   "ely"    Ely.by / LegacyLauncher / TLauncher cracked skins only
	//   "none"   don't fetch (let the client's own launcher skin show)
	SkinSource string `yaml:"skinSource"`

	// MineSkinAPIKey is used to upload Bedrock skins to Mojang so Java clients
	// can see them. Required for cross-edition skins to work.
	MineSkinAPIKey string `yaml:"mineSkinAPIKey"`
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
			Type:            "superflat",
			Seed:            12345,
			Spawn:           SpawnConfig{X: 0, Y: 4, Z: 0, Yaw: 0, Pitch: 0},
			Persistence:     true,
			Directory:       "worlds",
			AutosaveSeconds: 300,
			Difficulty:      "normal",
			DayNightCycle:   true,
		},
		Java: JavaConfig{
			Bind:               "0.0.0.0",
			Port:               25565,
			OnlineMode:         false,
			MaxPlayers:         100,
			ViewDistance:       10,
			SimulationDistance: 10,
			SkinSource:         "auto",
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

	opsFile := strings.Replace(path, "config.yml", "ops.txt", 1)
	if opsB, err := os.ReadFile(opsFile); err == nil {
		lines := strings.Split(string(opsB), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				cfg.Ops = append(cfg.Ops, line)
			}
		}
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
