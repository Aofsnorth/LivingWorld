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
	"context"
	"os"
	"path/filepath"
	"time"

	"livingworld/internal/bedrock"
	"livingworld/internal/infrastructure/logging"
	"livingworld/internal/java"
	"livingworld/internal/player"
	"livingworld/internal/skinbridge"
	"livingworld/internal/world"
	"livingworld/plugin"
)

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

// Run starts the server and blocks until SIGINT/SIGTERM (or the console
// "stop" command), then shuts down gracefully via the application harness.
// The harness owns signal handling, the operator console, ordered component
// shutdown, and health reporting; see NewHarness for the extensibility seam.
// It returns the Start error, if any.
func (s *Server) Run() error {
	h := s.NewHarness()
	_ = h.Register(&consoleComponent{srv: s, out: os.Stdout, stop: func() { _ = h.Stop() }})
	return h.Run(context.Background())
}

// Plugins returns the plugin manager for registering plugins and event handlers.
func (s *Server) Plugins() *plugin.PluginManager { return plugin.Manager() }

// Worlds returns the underlying world manager for advanced use.
func (s *Server) Worlds() *world.Manager { return s.worlds }

// PlayerManager returns the underlying player manager for advanced use.
func (s *Server) PlayerManager() *player.Manager { return s.players }

// Log writes a line to the server log.
func (s *Server) Log(format string, args ...any) { s.logger.Info(format, args...) }
