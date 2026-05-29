package java

import (
	"livingworld/config"
	"livingworld/internal/java/server"
	"livingworld/internal/player"
	"livingworld/internal/world"
)

type Server = server.Server

func NewServer(cfg *config.Config, pm *player.Manager, wm *world.Manager) *Server {
	return server.NewServer(cfg, pm, wm)
}
