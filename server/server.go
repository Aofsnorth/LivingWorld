// Package server is LivingWorld's public, embeddable API. Import it to run a
// native dual-protocol (Java + Bedrock) Minecraft server from your own Go program,
// dragonfly-style:
//
//	func main() {
//	    srv := server.New(server.DefaultConfig())
//	    srv.Plugins().OnPlayerJoin(func(e *plugin.PlayerJoinEvent) {
//	        srv.Broadcast("Welcome, " + e.PlayerName + "!")
//	    })
//	    srv.Run() // blocks until Ctrl-C, then saves and shuts down
//	}
//
// All capabilities a plugin needs are also available directly on *Server, which
// implements plugin.Host.
package server

import (
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"livingworld/config"
	"livingworld/internal/bedrock"
	"livingworld/internal/command"
	"livingworld/internal/infrastructure/logging"
	"livingworld/internal/java"
	"livingworld/internal/player"
	"livingworld/internal/skinbridge"
	"livingworld/internal/world"
	worldgen "livingworld/internal/world/generator"
	terraingen "livingworld/internal/worldgen"
	"livingworld/plugin"
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

// Server is a running (or runnable) LivingWorld instance. The zero value is not
// usable; construct one with New.
type Server struct {
	cfg     *Config
	worlds  *world.Manager
	players *player.Manager
	skins   *skinbridge.Service
	java    *java.Server
	bedrock *bedrock.Server
	logger  logging.Logger

	ops       *OpsList
	whitelist *Whitelist
}

// New builds a server from cfg. Pass nil to use DefaultConfig. The server does
// not listen until Start (or Run) is called.
func New(cfg *Config) *Server {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	logger := logging.GetLogger("Server")

	worlds := world.NewManager()
	dw := worlds.GetDefaultWorld()
	switch cfg.World.Type {
	case "", "superflat":
		dw.SetGenerator(worldgen.NewSuperflat())
	case "overworld", "default", "normal":
		dw.SetGenerator(terraingen.NewGenerator(cfg.World.Seed))
		// Spawn on the generated surface (overworld terrain isn't flat at y=4).
		cfg.World.Spawn.Y = float64(dw.HighestSolidY(int(cfg.World.Spawn.X), int(cfg.World.Spawn.Z)))
	default:
		logger.Warn("Unknown world.type %q, falling back to superflat", cfg.World.Type)
		dw.SetGenerator(worldgen.NewSuperflat())
	}

	players := player.NewManager()
	players.StartPushLoop() // cross-edition player-to-player pushing
	// Let the world's mob-spawn director read live player positions (the world
	// package can't import player directly — import cycle).
	worlds.SetPlayerLocator(func() []world.Position {
		all := players.GetAllPlayers()
		pts := make([]world.Position, 0, len(all))
		for _, p := range all {
			pts = append(pts, p.Position)
		}
		return pts
	})
	worlds.StartTimeLoop(cfg.World.DayNightCycle)
	worlds.StartMobAI(cfg.World.Difficulty)
	worlds.StartWeatherCycle(cfg.World.WeatherCycle)
	worlds.StartDropPhysics()
	command.Bind(players, worlds)
	command.RegisterBuiltins(command.Default())
	skins := skinbridge.New()
	s := &Server{
		cfg:     cfg,
		worlds:  worlds,
		players: players,
		skins:   skins,
		java:    java.NewServer(cfg, players, worlds),
		bedrock: bedrock.NewServer(cfg, players, worlds, skins),
		logger:  logger,
	}
	// Ops are seeded from cfg (Config.Ops drives the login op-check); the
	// whitelist starts empty and disabled. Main replaces these with
	// file-backed instances when launched from the command line.
	s.ops = newOps(cfg.Ops...)
	s.whitelist = newWhitelist()
	// Make this server the capability surface handed to plugins.
	plugin.Manager().SetHost(s)
	// Wire /op and /deop to the server's operator list.
	command.BindOps(s)
	return s
}

// Start begins listening on both protocols and enables world persistence and
// autosave. It returns once both listeners are up.
func (s *Server) Start() error {
	s.skins.Start()

	if s.cfg.World.Persistence {
		if err := s.worlds.EnablePersistence(s.cfg.World.Directory); err != nil {
			s.logger.Warn("World persistence disabled (setup failed): %v", err)
		} else {
			s.worlds.StartAutosave(time.Duration(s.cfg.World.AutosaveSeconds) * time.Second)
			if err := s.players.EnablePersistence(filepath.Join(s.cfg.World.Directory, "playerdata")); err != nil {
				s.logger.Warn("Player persistence disabled (setup failed): %v", err)
			}
			s.logger.Info("World persistence enabled at %q (autosave every %ds)", s.cfg.World.Directory, s.cfg.World.AutosaveSeconds)
		}
	}

	// Load drop-in JavaScript plugins from ./plugins before accepting players so
	// their event handlers are registered up front.
	if n, err := plugin.Manager().LoadScripts("plugins"); err != nil {
		s.logger.Warn("Plugin scripts: %v", err)
	} else if n > 0 {
		s.logger.Info("Loaded %d script plugin(s) from ./plugins", n)
	}

	if err := s.java.Start(); err != nil {
		return err
	}
	if err := s.bedrock.Start(); err != nil {
		s.java.Stop()
		return err
	}
	plugin.Manager().Emit(&plugin.ServerStartEvent{BaseEvent: plugin.BaseEvent{Type_: plugin.EventServerStart}})
	s.logger.Info("LivingWorld started — Java %s, Bedrock %s", s.cfg.Address(), s.cfg.BedrockAddress())
	return nil
}

// Stop shuts down both listeners and saves all worlds. Safe to call more than once.
func (s *Server) Stop() {
	plugin.Manager().Emit(&plugin.ServerStopEvent{BaseEvent: plugin.BaseEvent{Type_: plugin.EventServerStop}})
	s.players.SaveAll() // persist player data before disconnecting everyone
	if s.bedrock != nil {
		s.bedrock.Stop()
	}
	if s.java != nil {
		s.java.Stop()
	}
	if err := s.worlds.Close(); err != nil {
		s.logger.Error("Error saving worlds on shutdown: %v", err)
	}
}

// Run starts the server and blocks until SIGINT/SIGTERM, then shuts down
// gracefully (saving worlds). It returns the Start error, if any.
func (s *Server) Run() error {
	if err := s.Start(); err != nil {
		return err
	}

	// Start the operator console; "stop" triggers the same graceful shutdown as
	// SIGINT/SIGTERM.
	stop := make(chan struct{})
	var once sync.Once
	go newConsole(s, os.Stdout, func() { once.Do(func() { close(stop) }) }).run(os.Stdin)
	s.logger.Info("Console ready — type 'help' for commands")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-quit:
	case <-stop:
	}
	s.logger.Info("Shutting down LivingWorld...")
	s.Stop()
	s.logger.Info("LivingWorld stopped")
	return nil
}

// Plugins returns the plugin manager for registering plugins and event handlers.
func (s *Server) Plugins() *plugin.PluginManager { return plugin.Manager() }

// Worlds returns the underlying world manager for advanced use.
func (s *Server) Worlds() *world.Manager { return s.worlds }

// PlayerManager returns the underlying player manager for advanced use.
func (s *Server) PlayerManager() *player.Manager { return s.players }

// Ops returns the operator list (admins) for runtime management.
func (s *Server) Ops() *OpsList { return s.ops }

// SetOp grants or revokes operator status by name: it updates the ops list,
// keeps the login op-check (Config.Ops) in sync, and applies to the connected
// player immediately. Returns whether membership actually changed.
func (s *Server) SetOp(name string, op bool) bool {
	var changed bool
	if op {
		changed, _ = s.ops.Add(name)
	} else {
		changed, _ = s.ops.Remove(name)
	}
	s.cfg.Ops = s.ops.List()
	if pl := s.players.GetPlayerByName(name); pl != nil {
		pl.Op = op
	}
	return changed
}

// ListOps returns the current operator names.
func (s *Server) ListOps() []string { return s.ops.List() }

// Whitelist returns the join whitelist. Enforcement at login is performed by
// the protocol layer (it should reject players for which Allowed is false);
// toggle it with Whitelist().SetEnabled.
func (s *Server) Whitelist() *Whitelist { return s.whitelist }

// --- plugin.Host implementation ---

// Broadcast sends a chat message to every connected player.
func (s *Server) Broadcast(msg string) { s.players.Broadcast(msg) }

// Message sends a chat message to a single player by name.
func (s *Server) Message(playerName, msg string) {
	if p := s.players.GetPlayerByName(playerName); p != nil {
		s.players.Message(p.UUID, msg)
	}
}

// Players returns the names of currently connected players.
func (s *Server) Players() []string {
	all := s.players.GetAllPlayers()
	names := make([]string, 0, len(all))
	for _, p := range all {
		names = append(names, p.Username)
	}
	return names
}

// PlayerCount returns the number of connected players.
func (s *Server) PlayerCount() int { return s.players.PlayerCount() }

// GetBlock returns the block state ID at a world position.
func (s *Server) GetBlock(x, y, z int) int32 {
	return s.worlds.GetDefaultWorld().GetBlock(x, y, z).ID()
}

// SetBlock sets the block state ID at a world position and notifies clients on
// both protocols.
func (s *Server) SetBlock(x, y, z int, stateID int32) {
	s.worlds.SetBlockAndPublish(world.BlockUpdateSourceServer, x, y, z, world.BlockByID(stateID))
}

// StateID resolves a block state ID from a namespaced name ("minecraft:stone").
func (s *Server) StateID(name string) int32 { return world.StateID(name) }

// Log writes a line to the server log.
func (s *Server) Log(format string, args ...any) { s.logger.Info(format, args...) }
