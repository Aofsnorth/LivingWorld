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

	// DefaultGamemode is the gamemode assigned to players who join with no
	// persisted state (first join) or whose saved gamemode is out of range
	// (corrupt save). 0=survival, 1=creative, 2=adventure, 3=spectator.
	// Players with a valid saved gamemode keep it — set DefaultGamemode
	// changes only affect NEW players and persist for the rest of the
	// session (it is not broadcast on every join).
	DefaultGamemode int `yaml:"defaultGamemode"`

	// Ops lists the canonical operator usernames (case-insensitive). The yaml
	// field is the bootstrap source — add your own name here on a fresh
	// install to grant yourself op on the first start. /op and /deop also
	// mutate a sidecar file (config/ops.txt, see Load), and the two are
	// merged at load time so neither one has to be hand-edited once running.
	Ops []string `yaml:"ops"`

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

	// SpawnMobs enables natural mob spawning. When false the spawn director
	// is skipped entirely — no new mobs appear, but existing ones keep their
	// AI and physics. Defaults to true.
	SpawnMobs bool `yaml:"spawnMobs"`

	// SpawnMode selects which edition's mob spawn/despawn model the natural
	// spawn director uses: "java" (chunk-scaled per-category mob caps,
	// surface-Y spawning, internal light = max(block,sky)) or "bedrock"
	// (single static 200 global cap, 3D shell-Y spawning around the player,
	// hostile light = block light only). Defaults to "java". Both editions'
	// clients are served from one world, so this is a server-wide switch.
	SpawnMode string `yaml:"spawnMode"`

	// DayNightCycle enables the advancing time-of-day (sun/moon movement).
	DayNightCycle bool `yaml:"dayNightCycle"`

	// WeatherCycle enables the automatic clear→rain→thunder weather director.
	// (Vanilla's doWeatherCycle gamerule.) With it off, weather stays at whatever
	// was last set (persisted) and only changes via the /weather command.
	WeatherCycle bool `yaml:"weatherCycle"`

	// WeatherDurations controls how long each phase of the weather director
	// runs. Each phase picks a random integer in [Min, Max] seconds, then runs
	// for that long before transitioning. Setting Min==Max pins the phase to a
	// fixed duration (useful for deterministic testing). Defaults match
	// vanilla-ish ranges (clear 5-15 min, rain 10-20 min, thunder 3-10 min).
	WeatherDurations WeatherDurationsConfig `yaml:"weatherDurations"`
}

// WeatherDurationsConfig is the per-phase duration range, in seconds, used by
// StartWeatherCycle. Zero values fall back to vanilla defaults.
type WeatherDurationsConfig struct {
	ClearMinSeconds   int `yaml:"clearMinSeconds"`
	ClearMaxSeconds   int `yaml:"clearMaxSeconds"`
	RainMinSeconds    int `yaml:"rainMinSeconds"`
	RainMaxSeconds    int `yaml:"rainMaxSeconds"`
	ThunderMinSeconds int `yaml:"thunderMinSeconds"`
	ThunderMaxSeconds int `yaml:"thunderMaxSeconds"`
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

	// BedrockHDSkins is retained for backward compatibility. The signed MineSkin
	// (64×64) property is now ALWAYS sent as the baseline so vanilla Java renders
	// Bedrock skins; the unsigned local HD URL is only used as a fallback when no
	// signed property is available yet (upload pending or no MineSkinAPIKey). This
	// flag no longer discards the signed property, so it has no effect on the
	// current selection logic.
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
		ServerName:     system.DefaultServerName,
		MOTD:           system.DefaultMOTD,
		PluginsDir:     system.DefaultPluginsDirectory,
		DefaultGamemode: gameplay.GamemodeSurvival,
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
			SpawnMobs:       true,
			SpawnMode:       "java",
			DayNightCycle:   true,
			WeatherCycle:    true,
			WeatherDurations: WeatherDurationsConfig{
				ClearMinSeconds:   300,  // 5 min
				ClearMaxSeconds:   900,  // 15 min
				RainMinSeconds:    600,  // 10 min
				RainMaxSeconds:    1200, // 20 min
				ThunderMinSeconds: 180,  // 3 min
				ThunderMaxSeconds: 600,  // 10 min
			},
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
