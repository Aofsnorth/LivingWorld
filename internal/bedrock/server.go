package bedrock

import (
	"livingworld/config"
	"livingworld/internal/bedrock/server"
	"livingworld/internal/player"
	"livingworld/internal/skinbridge"
	"livingworld/internal/world"
)

type Server = server.Server

func NewServer(cfg *config.Config, pm *player.Manager, wm *world.Manager, skins *skinbridge.Service) *Server {
	return server.NewServer(cfg, pm, wm, skins)
}
