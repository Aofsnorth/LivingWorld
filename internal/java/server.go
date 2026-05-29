package java

import (
	"fmt"
	"log"
	"sync"

	"livingworld/config"
	"livingworld/internal/player"
	"livingworld/internal/world"

	gmnet "github.com/Tnze/go-mc/net"
)

type Server struct {
	addr     string
	port     int
	listener *gmnet.Listener
	cfg      *config.Config
	pm       *player.Manager
	wm       *world.Manager
	wg       sync.WaitGroup
	running  bool
	mu       sync.Mutex
	bridge   *javaBridge
}

func NewServer(cfg *config.Config, pm *player.Manager, wm *world.Manager) *Server {
	return &Server{
		port: cfg.Java.Port,
		addr: cfg.Java.Bind,
		cfg:  cfg,
		pm:   pm,
		wm:   wm,
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

	addr := fmt.Sprintf("%s:%d", s.addr, s.port)
	listener, err := gmnet.ListenMC(addr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	s.listener = listener
	s.bridge = newJavaBridge(s.cfg, s.pm, s.wm)

	log.Printf("[Java] Server listening on %s", addr)

	s.wg.Add(1)
	go s.acceptLoop()

	return nil
}

func (s *Server) acceptLoop() {
	defer s.wg.Done()

	for {
		s.mu.Lock()
		listener := s.listener
		running := s.running
		s.mu.Unlock()
		if !running || listener == nil {
			break
		}

		conn, err := listener.Accept()
		if err != nil {
			s.mu.Lock()
			stillRunning := s.running
			s.mu.Unlock()
			if !stillRunning {
				break
			}
			log.Printf("[Java] Accept error: %v", err)
			continue
		}

		go s.bridge.acceptConn(conn)
	}
}

func (s *Server) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	listener := s.listener
	s.listener = nil
	s.mu.Unlock()
	if listener != nil {
		_ = listener.Close()
	}
	s.wg.Wait()
}
