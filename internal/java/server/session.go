package server

import (
	"livingworld/config"
	"livingworld/internal/java/protocol"
	"livingworld/internal/player"
	"livingworld/internal/world"
	"sync"
	"sync/atomic"

	gmnet "github.com/Tnze/go-mc/net"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/google/uuid"
)

var nextEntityID atomic.Int32

func init() {
	nextEntityID.Store(2)
}

type PlayerSession struct {
	EntityIDVal int32
	UUIDVal     uuid.UUID
	UsernameVal string
	Conn_       *gmnet.Conn
	Bridge      *javaBridge
	version     protocol.VersionHandler

	X, Y, Z    float64
	Yaw, Pitch float32
	OnGround   bool

	// FallDistance accumulates blocks fallen since last on the ground; used to
	// compute fall damage on landing (Java damage is server-authoritative).
	FallDistance float64

	Health     float32
	Food       int32
	Saturation float32
	GameModeVal   int32

	SelectedSlot int32
	LoadedChunks map[world.ChunkPos]bool
	lastSentPos  map[uuid.UUID]world.Position

	chunkQueue   chan struct{}
	mu sync.Mutex
	Ready bool
}

func NewPlayerSession(username string, id uuid.UUID, conn *gmnet.Conn, bridge *javaBridge) *PlayerSession {
	spawn := bridge.cfg.World.Spawn
	return &PlayerSession{
		EntityIDVal:     nextEntityID.Add(1),
		UUIDVal:         id,
		UsernameVal:     username,
		Conn_:           conn,
		Bridge:          bridge,
		X:               spawn.X,
		Y:               spawn.Y,
		Z:               spawn.Z,
		Health:          MaxHealth,
		Food:            MaxFood,
		Saturation:      MaxSaturation,
		GameModeVal:     0, // survival — required for hunger drain and fall damage
		LoadedChunks:    make(map[world.ChunkPos]bool),
		lastSentPos:     make(map[uuid.UUID]world.Position),
		chunkQueue:      make(chan struct{}, 1),
	}
}

// protocol.Session interface getters

func (s *PlayerSession) Conn() *gmnet.Conn {
	return s.Conn_
}

func (s *PlayerSession) UUID() uuid.UUID {
	return s.UUIDVal
}

func (s *PlayerSession) Username() string {
	return s.UsernameVal
}

func (s *PlayerSession) EntityID() int32 {
	return s.EntityIDVal
}

func (s *PlayerSession) Config() *config.Config {
	return s.Bridge.cfg
}

func (s *PlayerSession) PlayerManager() *player.Manager {
	return s.Bridge.pm
}

func (s *PlayerSession) WorldManager() *world.Manager {
	return s.Bridge.wm
}

func (s *PlayerSession) GameMode() int32 {
	return s.GameModeVal
}

func (s *PlayerSession) SendSpawnPosition() error {
	return s.sendSpawnPosition()
}

func (s *PlayerSession) SendHealth() error {
	return s.sendHealth()
}

func (s *PlayerSession) UpdateChunks() {
	s.updateChunks()
}

func (s *PlayerSession) SendWorldState() {
	s.sendWorldState()
}

func (s *PlayerSession) SendPacket(p pk.Packet) error {
	return s.Conn_.WritePacket(p)
}

// WriteRaw writes pre-framed packet data directly to the connection.
// The data must already include the length prefix (VarInt) followed by packetID + payload.
func (s *PlayerSession) WriteRaw(data []byte) error {
	_, err := s.Conn_.Writer.Write(data)
	return err
}

func (s *PlayerSession) ChunkX() int32 {
	return int32(s.X) >> 4
}

func (s *PlayerSession) ChunkZ() int32 {
	return int32(s.Z) >> 4
}

type SessionManager struct {
	sessions map[uuid.UUID]*PlayerSession
	mu       sync.RWMutex
}

func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[uuid.UUID]*PlayerSession),
	}
}

func (m *SessionManager) Add(s *PlayerSession) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[s.UUIDVal] = s
}

func (m *SessionManager) Remove(id uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, id)
}

func (m *SessionManager) Get(id uuid.UUID) *PlayerSession {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[id]
}

func (m *SessionManager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}

func (m *SessionManager) Broadcast(p pk.Packet) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, s := range m.sessions {
		_ = s.SendPacket(p)
	}
}

func (m *SessionManager) BroadcastExcept(exclude uuid.UUID, p pk.Packet) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, s := range m.sessions {
		if s.UUIDVal != exclude {
			_ = s.SendPacket(p)
		}
	}
}

func (m *SessionManager) ForEach(fn func(*PlayerSession)) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, s := range m.sessions {
		fn(s)
	}
}

func (s *PlayerSession) ChunkWorker() {
	for range s.chunkQueue {
		s.updateChunksWithBatch(true)
	}
}
