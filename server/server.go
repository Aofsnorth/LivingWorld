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
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"livingworld/config"
	"livingworld/internal/bedrock"
	"livingworld/internal/java"
	"livingworld/internal/player"
	"livingworld/internal/skinbridge"
	"livingworld/internal/world"
	worldgen "livingworld/internal/world/generator"
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
}

// New builds a server from cfg. Pass nil to use DefaultConfig. The server does
// not listen until Start (or Run) is called.
func New(cfg *Config) *Server {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	worlds := world.NewManager()
	switch cfg.World.Type {
	case "", "superflat":
		worlds.GetDefaultWorld().SetGenerator(worldgen.NewSuperflat())
	default:
		log.Printf("Unknown world.type %q, falling back to superflat", cfg.World.Type)
		worlds.GetDefaultWorld().SetGenerator(worldgen.NewSuperflat())
	}

	players := player.NewManager()
	skins := skinbridge.New()

	s := &Server{
		cfg:     cfg,
		worlds:  worlds,
		players: players,
		skins:   skins,
		java:    java.NewServer(cfg, players, worlds),
		bedrock: bedrock.NewServer(cfg, players, worlds, skins),
	}
	// Make this server the capability surface handed to plugins.
	plugin.Manager().SetHost(s)
	return s
}

// Start begins listening on both protocols and enables world persistence and
// autosave. It returns once both listeners are up.
func (s *Server) Start() error {
	s.skins.Start()

	if s.cfg.World.Persistence {
		if err := s.worlds.EnablePersistence(s.cfg.World.Directory); err != nil {
			log.Printf("World persistence disabled (setup failed): %v", err)
		} else {
			s.worlds.StartAutosave(time.Duration(s.cfg.World.AutosaveSeconds) * time.Second)
			log.Printf("World persistence enabled at %q (autosave every %ds)", s.cfg.World.Directory, s.cfg.World.AutosaveSeconds)
		}
	}

	if err := s.java.Start(); err != nil {
		return err
	}
	if err := s.bedrock.Start(); err != nil {
		s.java.Stop()
		return err
	}
	plugin.Manager().Emit(&plugin.ServerStartEvent{BaseEvent: plugin.BaseEvent{Type_: plugin.EventServerStart}})
	log.Printf("LivingWorld started — Java %s, Bedrock %s", s.cfg.Address(), s.cfg.BedrockAddress())
	return nil
}

// Stop shuts down both listeners and saves all worlds. Safe to call more than once.
func (s *Server) Stop() {
	plugin.Manager().Emit(&plugin.ServerStopEvent{BaseEvent: plugin.BaseEvent{Type_: plugin.EventServerStop}})
	if s.bedrock != nil {
		s.bedrock.Stop()
	}
	if s.java != nil {
		s.java.Stop()
	}
	if err := s.worlds.Close(); err != nil {
		log.Printf("Error saving worlds on shutdown: %v", err)
	}
}

// Run starts the server and blocks until SIGINT/SIGTERM, then shuts down
// gracefully (saving worlds). It returns the Start error, if any.
func (s *Server) Run() error {
	if err := s.Start(); err != nil {
		return err
	}
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down LivingWorld...")
	s.Stop()
	log.Println("LivingWorld stopped")
	return nil
}

// Plugins returns the plugin manager for registering plugins and event handlers.
func (s *Server) Plugins() *plugin.PluginManager { return plugin.Manager() }

// Worlds returns the underlying world manager for advanced use.
func (s *Server) Worlds() *world.Manager { return s.worlds }

// PlayerManager returns the underlying player manager for advanced use.
func (s *Server) PlayerManager() *player.Manager { return s.players }

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
func (s *Server) Log(format string, args ...any) { log.Printf(format, args...) }
