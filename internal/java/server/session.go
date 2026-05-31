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

	Health      float32
	Food        int32
	Saturation  float32
	GameModeVal int32

	SelectedSlot int32
	LoadedChunks map[world.ChunkPos]bool
	lastSentPos  map[uuid.UUID]world.Position

	chunkQueue chan struct{}
	// sendQueue serializes foreign-avatar relays (spawn/move/skin/...) on a
	// per-session goroutine so a slow client backs up only its own queue, never
	// the shared player-event loop. Without this, the single event-loop goroutine
	// blocked on each client's network write in turn, so one busy/slow client
	// stalled relays to everyone (players looked frozen) and could even drop
	// join events under the movement-packet flood (foreign players invisible).
	sendQueue chan func()
	mu        sync.Mutex
	// writeMu serializes all writes to Conn_. SendPacket/WriteRaw are called
	// concurrently from many goroutines (chunk worker, player-event loop, drop
	// loop, block-update loop, keep-alive, the read loop). go-mc's
	// Conn.WritePacket has no internal lock, so concurrent writes interleave
	// their bytes and corrupt the packet stream — which showed up as chunks
	// rendering wrong / not at all while moving (the moment with the most
	// concurrent traffic: chunk streaming + entity sync at once).
	writeMu sync.Mutex
	Ready   bool
}

func NewPlayerSession(username string, id uuid.UUID, conn *gmnet.Conn, bridge *javaBridge) *PlayerSession {
	spawn := bridge.cfg.World.Spawn
	return &PlayerSession{
		EntityIDVal:  nextEntityID.Add(1),
		UUIDVal:      id,
		UsernameVal:  username,
		Conn_:        conn,
		Bridge:       bridge,
		X:            spawn.X,
		Y:            spawn.Y,
		Z:            spawn.Z,
		Health:       MaxHealth,
		Food:         MaxFood,
		Saturation:   MaxSaturation,
		GameModeVal:  0, // survival — required for hunger drain and fall damage
		LoadedChunks: make(map[world.ChunkPos]bool),
		lastSentPos:  make(map[uuid.UUID]world.Position),
		chunkQueue:   make(chan struct{}, 1),
		sendQueue:    make(chan func(), 1024),
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
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.Conn_.WritePacket(p)
}

// WriteRaw writes pre-framed packet data directly to the connection.
// The data must already include the length prefix (VarInt) followed by packetID + payload.
func (s *PlayerSession) WriteRaw(data []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err := s.Conn_.Writer.Write(data)
	return err
}

func (s *PlayerSession) ChunkX() int32 {
	return world.ChunkCoord(s.X)
}

func (s *PlayerSession) ChunkZ() int32 {
	return world.ChunkCoord(s.Z)
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

// sendLoop runs queued relay work (foreign-avatar spawn/move/...) in order on a
// dedicated goroutine, so blocking network writes never stall the shared
// player-event loop.
func (s *PlayerSession) sendLoop() {
	for f := range s.sendQueue {
		f()
	}
}

// enqueue schedules relay work for this session. If the queue is full the client
// is hopelessly behind, so the update is dropped for this session only (it will
// be corrected by the next position/teleport update) rather than blocking others.
func (s *PlayerSession) enqueue(f func()) {
	select {
	case s.sendQueue <- f:
	default:
	}
}
