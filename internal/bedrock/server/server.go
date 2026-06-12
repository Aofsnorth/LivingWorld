package server

import (
	"fmt"
	"log"
	"sync"

	"livingworld/config"
	bedrockworld "livingworld/internal/bedrock/world"
	"livingworld/internal/player"
	"livingworld/internal/skinbridge"
	"livingworld/internal/world"

	"github.com/sandertv/gophertunnel/minecraft"
)

type Server struct {
	addr      string
	port      int
	cfg       *config.Config
	pm        *player.Manager
	wm        *world.Manager
	skins     *skinbridge.Service
	converter *bedrockworld.ChunkConverter
	listener  *minecraft.Listener
	wg        sync.WaitGroup
	running   bool
	mu        sync.Mutex

	sessionsMu   sync.RWMutex
	sessions     map[string]*bedrockSession
	playerEvents <-chan player.Event
}

func NewServer(cfg *config.Config, pm *player.Manager, wm *world.Manager, skins *skinbridge.Service) *Server {
	return &Server{
		port:      cfg.Bedrock.Port,
		addr:      cfg.Bedrock.Bind,
		cfg:       cfg,
		pm:        pm,
		wm:        wm,
		converter: bedrockworld.NewChunkConverter(),
		sessions:  make(map[string]*bedrockSession),
		skins:     skins,
	}
}

func (s *Server) Start() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = true
	s.mu.Unlock()

	// DISABLED 2026-06-02 — see internal/bedrock/server/raknet_pong.go.
	// registerSurvivalNetwork() replaced gophertunnel's "raknet" network to rewrite
	// the pong gamemode to "Survival". A controlled experiment proved the listener
	// wrapper breaks the RakNet connection handshake with go-raknet Jan-2026 (the
	// version dragonfly v0.10.13 pulls in): the unconnected ping/pong still works
	// (server is discoverable) but EVERY client times out connecting. Stock
	// gophertunnel WITH this override times out; WITHOUT it a real client joins in
	// ~15ms. The "Survival" server-list label is purely cosmetic (the in-game
	// gamemode comes from GameData.PlayerGameMode), so dropping it to keep joins
	// working is the right trade. Re-enable only with a non-wrapping pong fix.
	// registerSurvivalNetwork()

	cfg := minecraft.ListenConfig{
		MaximumPlayers:         s.cfg.Bedrock.MaxPlayers,
		AuthenticationDisabled: s.cfg.Bedrock.AuthDisabled,
		StatusProvider:         minecraft.NewStatusProvider(s.cfg.ServerName, s.cfg.MOTD),
	}

	addr := fmt.Sprintf("%s:%d", s.addr, s.port)
	listener, err := cfg.Listen("raknet", addr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	s.listener = listener

	log.Printf("[Bedrock] Server listening on 0.0.0.0:%d", s.port)
	bedrockworld.LogBlockPaletteVersion()
	s.startBlockEventLoop()
	s.startEffectEventLoop()
	s.startPlayerEventLoop()
	s.startTimeLoop()
	s.startMobSync()
	s.startWeatherSync()
	s.startDropLoop()
	s.startCrackProgressLoop()
	s.registerPickupHandler()
	// UX (M0.7): mob sound fan-out. The world tick publishes a
	// []mobs.SoundEmit per tick and this listener translates each
	// into the per-edition packet (LevelSoundEvent for combat /
	// hurt / death, PlaySound with namespaced id for ambients).
	s.wm.OnMobSound(s.publishMobSounds)

	s.wg.Add(1)
	go s.acceptLoop()

	return nil
}

func (s *Server) acceptLoop() {
	defer s.wg.Done()

	for s.running {
		conn, err := s.listener.Accept()
		if err != nil {
			if !s.running {
				break
			}
			log.Printf("[Bedrock] Accept error: %v", err)
			continue
		}

		go s.handleConn(conn)
	}
}

func (s *Server) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	s.mu.Unlock()

	if s.listener != nil {
		s.listener.Close()
	}
	s.pm.Unsubscribe("bedrock-server")
	s.wm.UnsubscribeBlockUpdates("bedrock-server")
	s.wm.UnsubscribeWorldEffects("bedrock-effects")
	s.wg.Wait()
}
