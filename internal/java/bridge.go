package java

import (
	"log"

	"livingworld/config"
	javaregistry "livingworld/internal/java/registry"
	"livingworld/internal/player"
	"livingworld/internal/world"

	"github.com/Tnze/go-mc/chat"
	gmnet "github.com/Tnze/go-mc/net"
	gmserver "github.com/Tnze/go-mc/server"
)

type javaBridge struct {
	cfg          *config.Config
	pm           *player.Manager
	wm           *world.Manager
	server       *gmserver.Server
	protocol     int
	sessions     *SessionManager
	playerEvents <-chan player.Event
}

func newJavaBridge(cfg *config.Config, pm *player.Manager, wm *world.Manager) *javaBridge {
	j := &javaBridge{
		cfg:      cfg,
		pm:       pm,
		wm:       wm,
		protocol: int(gmserver.ProtocolVersion),
		sessions: NewSessionManager(),
	}
	log.Printf("[Java] Using protocol version: %d (ProtocolVersion=%d)", j.protocol, gmserver.ProtocolVersion)
	j.startPlayerEventLoop()
	ping := gmserver.NewPingInfo("LivingWorld Java", j.protocol, chat.Message{Text: cfg.MOTD}, nil)
	playerList := gmserver.NewPlayerList(cfg.Java.MaxPlayers)
	registries, registrySizes := javaregistry.Build()
	j.server = &gmserver.Server{
		Logger:          log.Default(),
		ListPingHandler: &javaListPing{ping: ping, playerList: playerList},
		LoginHandler: &gmserver.MojangLoginHandler{
			OnlineMode:   cfg.Java.OnlineMode,
			Threshold:    -1,
			LoginChecker: playerList,
		},
		ConfigHandler: &javaConfig{registries: registries, registrySizes: registrySizes},
		GamePlay:      j,
	}
	return j
}

func (j *javaBridge) acceptConn(conn gmnet.Conn) {
	j.server.AcceptConn(&conn)
}
