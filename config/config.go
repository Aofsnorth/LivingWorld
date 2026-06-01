package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"livingworld/internal/shared/constants/gameplay"
	"livingworld/internal/shared/constants/network"
	"livingworld/internal/shared/constants/system"

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
	case gameplay.DifficultyPeaceful:
		return gameplay.DifficultyBytePeaceful
	case gameplay.DifficultyEasy:
		return gameplay.DifficultyByteEasy
	case gameplay.DifficultyHard:
		return gameplay.DifficultyByteHard
	default: // "normal" or unset
		return gameplay.DifficultyByteNormal
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

	// BedrockHDSkins, when true, serves a Bedrock player's FULL-resolution
	// (128×128) skin to Java via the local skin bridge instead of the MineSkin
	// 64×64 signed property — fixing the downscaled "burik" look. This only
	// renders on clients that accept unsigned skin URLs from arbitrary hosts
	// (most authlib-injector launchers do; strict/vanilla clients show the
	// default skin instead, so it is OFF by default — flip it on and test).
	BedrockHDSkins bool `yaml:"bedrockHDSkins"`

	// DebugChunks gates verbose chunk-streaming logs (boundary crossings, chunk
	// cache center, forget/send lists). Runtime-only: set via the --debug-chunks
	// CLI flag, never read from or written to YAML. Default OFF.
	DebugChunks bool `yaml:"-"`
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
		ServerName: system.DefaultServerName,
		MOTD:       system.DefaultMOTD,
		PluginsDir: system.DefaultPluginsDirectory,
		World: WorldConfig{
			Type: system.WorldTypeDefault,
			Seed: system.DefaultWorldSeed,
			Spawn: SpawnConfig{
				X:     system.DefaultSpawnX,
				Y:     system.DefaultSpawnY,
				Z:     system.DefaultSpawnZ,
				Yaw:   system.DefaultSpawnYaw,
				Pitch: system.DefaultSpawnPitch,
			},
			Persistence:     true,
			Directory:       system.DefaultWorldsDirectory,
			AutosaveSeconds: int(system.DefaultAutosaveInterval.Seconds()),
			Difficulty:      gameplay.DifficultyNormal,
			DayNightCycle:   true,
		},
		Java: JavaConfig{
			Bind:               network.DefaultBindAddress,
			Port:               network.DefaultJavaPort,
			OnlineMode:         false,
			MaxPlayers:         system.DefaultMaxPlayers,
			ViewDistance:       gameplay.DefaultJavaViewDistance,
			SimulationDistance: gameplay.DefaultSimulationDistance,
			SkinSource:         system.SkinSourceAuto,
		},
		Bedrock: BedrockConfig{
			Bind:         network.DefaultBindAddress,
			Port:         network.DefaultBedrockPort,
			MaxPlayers:   system.DefaultMaxPlayers,
			ViewDistance: gameplay.DefaultBedrockViewDistance,
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

	opsFile := strings.Replace(path, "config.yml", system.DefaultOpsFile, 1)
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
