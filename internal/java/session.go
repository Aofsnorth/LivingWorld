package java

import (
	"sync"
	"sync/atomic"

	"livingworld/internal/world"

	gmnet "github.com/Tnze/go-mc/net"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/google/uuid"
)

var nextEntityID atomic.Int32

func init() {
	nextEntityID.Store(2)
}

type PlayerSession struct {
	EntityID int32
	UUID     uuid.UUID
	Username string
	Conn     *gmnet.Conn
	Bridge   *javaBridge

	X, Y, Z    float64
	Yaw, Pitch float32
	OnGround   bool

	// FallDistance accumulates blocks fallen since last on the ground; used to
	// compute fall damage on landing (Java damage is server-authoritative).
	FallDistance float64

	Health     float32
	Food       int32
	Saturation float32
	GameMode   int32

	SelectedSlot int32
	LoadedChunks map[world.ChunkPos]bool

	mu sync.Mutex
}

func NewPlayerSession(username string, id uuid.UUID, conn *gmnet.Conn, bridge *javaBridge) *PlayerSession {
	spawn := bridge.cfg.World.Spawn
	return &PlayerSession{
		EntityID:     nextEntityID.Add(1),
		UUID:         id,
		Username:     username,
		Conn:         conn,
		Bridge:       bridge,
		X:            spawn.X,
		Y:            spawn.Y,
		Z:            spawn.Z,
		Health:       MaxHealth,
		Food:         MaxFood,
		Saturation:   MaxSaturation,
		GameMode:     0, // survival — required for hunger drain and fall damage
		LoadedChunks: make(map[world.ChunkPos]bool),
	}
}

func (s *PlayerSession) SendPacket(p pk.Packet) error {
	return s.Conn.WritePacket(p)
}

// WriteRaw writes pre-framed packet data directly to the connection.
// The data must already include the length prefix (VarInt) followed by packetID + payload.
func (s *PlayerSession) WriteRaw(data []byte) error {
	_, err := s.Conn.Writer.Write(data)
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
	m.sessions[s.UUID] = s
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
		if s.UUID != exclude {
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
