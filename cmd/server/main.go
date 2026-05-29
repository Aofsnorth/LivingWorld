package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"livingworld/config"
	"livingworld/internal/bedrock"
	"livingworld/internal/java"
	"livingworld/internal/plugin"
	"livingworld/internal/player"
	"livingworld/internal/world"
	worldgen "livingworld/internal/world/generator"
)

var (
	Version   = "0.0.1"
	BuildDate = "2026-05-27"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Printf("LivingWorld Server v%s (build: %s)", Version, BuildDate)
	log.Println("Starting server...")

	configPath := flag.String("config", "config/config.yml", "path to YAML config")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	worldManager := world.NewManager()
	switch cfg.World.Type {
	case "", "superflat":
		worldManager.GetDefaultWorld().SetGenerator(worldgen.NewSuperflat())
	default:
		log.Printf("Unknown world.type %q, falling back to superflat", cfg.World.Type)
		worldManager.GetDefaultWorld().SetGenerator(worldgen.NewSuperflat())
	}
	playerManager := player.NewManager()

	server := NewServer(cfg, worldManager, playerManager)

	if err := server.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	plugin.Manager().Emit(&plugin.ServerStartEvent{BaseEvent: plugin.BaseEvent{Type_: plugin.EventServerStart}})

	log.Printf("Server started on Java %s, Bedrock %s", cfg.Address(), cfg.BedrockAddress())

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")
	plugin.Manager().Emit(&plugin.ServerStopEvent{BaseEvent: plugin.BaseEvent{Type_: plugin.EventServerStop}})
	server.Stop()
	log.Println("Server stopped")
}

type Server struct {
	cfg           *config.Config
	worldManager  *world.Manager
	playerManager *player.Manager
	javaServer    *java.Server
	bedrockServer *bedrock.Server
}

func NewServer(cfg *config.Config, wm *world.Manager, pm *player.Manager) *Server {
	return &Server{
		cfg:           cfg,
		worldManager:  wm,
		playerManager: pm,
		javaServer:    java.NewServer(cfg, pm, wm),
		bedrockServer: bedrock.NewServer(cfg, pm, wm),
	}
}

func (s *Server) Start() error {
	if err := s.javaServer.Start(); err != nil {
		return err
	}
	if err := s.bedrockServer.Start(); err != nil {
		s.javaServer.Stop()
		return err
	}
	return nil
}

func (s *Server) Stop() {
	if s.bedrockServer != nil {
		s.bedrockServer.Stop()
	}
	if s.javaServer != nil {
		s.javaServer.Stop()
	}
}