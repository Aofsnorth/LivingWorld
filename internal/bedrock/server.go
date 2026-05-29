package bedrock

import (
	"fmt"
	"log"
	"sync"

	"livingworld/config"
	"livingworld/internal/player"
	"livingworld/internal/skinbridge"
	"livingworld/internal/world"

	"github.com/sandertv/gophertunnel/minecraft"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
)

type Server struct {
	addr      string
	port      int
	cfg       *config.Config
	pm        *player.Manager
	wm        *world.Manager
	skins     *skinbridge.Service
	converter *chunkConverter
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
		converter: newChunkConverter(),
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
	log.Printf("[Bedrock] Block palette pinned to MC %s (protocol %d) — clients MUST be this exact version or terrain renders transparent",
		protocol.CurrentVersion, protocol.CurrentProtocol)
	logBlockPaletteVersion()
	s.startPlayerEventLoop()
	s.startBlockEventLoop()

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
	s.wg.Wait()
}
