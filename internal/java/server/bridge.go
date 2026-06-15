package server

import (
	"livingworld/internal/java/protocol"
	"log"
	"time"

	"livingworld/config"
	"livingworld/internal/command"
	javaregistry "livingworld/internal/java/registry"
	"livingworld/internal/player"
	"livingworld/internal/world"

	"github.com/Tnze/go-mc/chat"
	gmcommand "github.com/Tnze/go-mc/server/command"
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
	j.startLightEventLoop()
	j.startEffectEventLoop()
	j.startTimeLoop()
	j.startDropLoop()
	j.startMobSync()
	// UX (M0.7): mob sound fan-out. The world tick publishes a
	// []mobs.SoundEmit per tick and this listener broadcasts a
	// ClientboundGameSoundEntity packet to every session per emit.
	j.wm.OnMobSound(j.publishMobSounds)
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
		ConfigHandler: &javaConfig{registries: registries, registrySizes: registrySizes, commands: buildCommandTree()},
		GamePlay:      j,
	}
	return j
}

func (j *javaBridge) acceptConn(conn gmnet.Conn) {
	j.server.AcceptConn(&conn)
}

// buildCommandTree serializes the protocol-free command.Registry into a go-mc
// command.Graph so the client gets a complete ClientboundCommands packet
// during config phase. Each command is exposed as a literal node with an
// argument child where the command takes required arguments — the client then
// shows "<name> <arg>" in tab-completion rather than just "<name>". Built
// once at bridge construction and shared across every connection.
func buildCommandTree() *gmcommand.Graph {
	g := gmcommand.NewGraph()
	for _, c := range command.Default().Commands() {
		addCommandNode(g, c)
		for _, alias := range c.Aliases {
			aliasCopy := *c
			aliasCopy.Name = alias
			addCommandNode(g, &aliasCopy)
		}
	}
	return g
}

// addCommandNode adds a single command to the graph. Commands with a typed
// first-argument get one Argument child; commands with no recognized argument
// shape stay as a bare Literal so the client still autocompletes the name.
// Unhandle marks the node as a leaf — execution still goes through the
// project's Dispatch, not brigadier, so we don't need a go-mc Run handler.
func addCommandNode(g *gmcommand.Graph, c *command.Command) {
	switch c.Name {
	case "gamemode", "gm":
		g.Literal(c.Name).AppendArgument(g.Argument("mode", gmcommand.StringParser(0)).Unhandle()).Unhandle()
	case "tp", "teleport":
		g.Literal(c.Name).AppendArgument(g.Argument("target", gmcommand.StringParser(0)).Unhandle()).Unhandle()
	case "give":
		g.Literal(c.Name).AppendArgument(g.Argument("item", gmcommand.StringParser(0)).Unhandle()).Unhandle()
	case "time", "weather", "summon":
		g.Literal(c.Name).AppendArgument(g.Argument("value", gmcommand.StringParser(0)).Unhandle()).Unhandle()
	default:
		g.Literal(c.Name).Unhandle()
	}
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
