package server

import (
	"livingworld/internal/java/protocol"
	"log"
	"time"

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
	gmserver.IsSupportedProtocol = func(proto int32) bool {
		_, ok := protocol.GetVersionHandler(int(proto))
		return ok
	}
	log.Printf("[Java] Using default protocol version: %d (ProtocolVersion=%d). Supported: %v", j.protocol, gmserver.ProtocolVersion, protocol.GetSupportedProtocols())
	j.startPlayerEventLoop()
	j.startBlockEventLoop()
	j.startEffectEventLoop()
	j.startTimeLoop()
	j.startDropLoop()
	j.startMobSync()
	j.wm.OnWeatherChange(j.broadcastWeather)
	ping := gmserver.NewPingInfo("LivingWorld Java", j.protocol, chat.Message{Text: cfg.MOTD}, nil)
	playerList := gmserver.NewPlayerList(cfg.Java.MaxPlayers)
	registries, registrySizes := javaregistry.Build()
	j.server = &gmserver.Server{
		Logger:          log.Default(),
		ListPingHandler: &javaListPing{ping: ping, playerList: playerList, sessions: j.sessions},
		LoginHandler: &gmserver.MojangLoginHandler{
			OnlineMode:   cfg.Java.OnlineMode,
			Threshold:    256, // vanilla default: compress packets >=256B (chunks ~52KB raw -> a few KB)
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

// startTimeLoop periodically broadcasts the authoritative world time. With the
// daylight cycle on, the client advances the sun itself between sends (rate=1.0),
// so a few-second interval is enough to correct drift without spamming.
func (j *javaBridge) startTimeLoop() {
	advancing := j.cfg.World.DayNightCycle
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			w := j.wm.GetDefaultWorld()
			j.sessions.Broadcast(buildSetTimePacket(w.GetTime(), w.GetDayTime(), advancing))
		}
	}()
}
