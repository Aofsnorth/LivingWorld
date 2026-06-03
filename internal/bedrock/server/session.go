package server

import (
	"sync"
	"time"

	bedrockworld "livingworld/internal/bedrock/world"
	"livingworld/internal/player"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/google/uuid"
	"github.com/sandertv/gophertunnel/minecraft"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/login"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

const (
	// Bedrock local spawn/teleport positions are empirically camera/eye based in
	// this gophertunnel path. Shared player/world state remains feet based.
	bedrockLocalEyeHeight = 1.62
)

type bedrockSession struct {
	id        uuid.UUID
	username  string
	runtimeID uint64

	conn       *minecraft.Conn
	identity   login.IdentityData
	clientData login.ClientData
	pm         *player.Manager

	lastMovePublish     time.Time
	lastAuthInputAt     time.Time
	lastX, lastY, lastZ float64

	// chunkCache is the thread-safe Bedrock chunk cache for this connection session.
	chunkCache *bedrockworld.ChunkCache

	// LoadedChunks tracks which chunks have already been sent to the client.
	LoadedChunks map[protocol.ChunkPos]bool
	chunkMu      sync.Mutex

	// lastPubX/lastPubZ track the last position where NetworkChunkPublisherUpdate was sent.
	lastPubX int32
	lastPubZ int32

	// lastChunkX/lastChunkZ track the player's last chunk coordinates for loading chunks dynamically.
	lastChunkX int32
	lastChunkZ int32

	// viewDistance tracks the player's active view distance Negotiated with the client.
	viewDistance int32

	// invOpened tracks whether the player's own inventory ContainerOpen has been
	// sent. Re-sending ContainerOpen while already open crashes the client.
	invOpened bool

	health float32 // server-tracked health for cross-edition melee damage

	// viewers tracks which foreign players are spawned on this client, for the
	// Area-Of-Interest spawn/despawn diff (#9).
	viewers *viewerTracker

	// mobViewer (M0.7) tracks which mobs are currently spawned on this
	// client's AOI. OnMove checks the per-mob distance and only sends
	// MoveActorAbsolute for mobs within ~80 blocks. Cross-boundary
	// entries are sent as AddActor; exits as RemoveActor.
	mobViewer *mobTracker

	mu sync.Mutex
}

func newBedrockSession(id uuid.UUID, username string, runtimeID uint64, conn *minecraft.Conn, pm *player.Manager) *bedrockSession {
	return &bedrockSession{
		id:           id,
		username:     username,
		runtimeID:    runtimeID,
		conn:         conn,
		pm:           pm,
		identity:     conn.IdentityData(),
		clientData:   conn.ClientData(),
		chunkCache:   bedrockworld.NewChunkCache(),
		LoadedChunks: make(map[protocol.ChunkPos]bool),
		lastPubX:     -999999, // Trigger update immediately on first move
		lastPubZ:     -999999,
		lastChunkX:   -999999,
		lastChunkZ:   -999999,
		viewDistance: 0,
		health:       20,
		viewers:      newViewerTracker(),
		mobViewer:    newMobTracker(),
	}
}

func (s *bedrockSession) pmRef() *player.Manager { return s.pm }

// chunkCenter returns the viewer's last known chunk coordinates under the session
// lock. lastChunkX/Z are written by the move goroutine and read by the AOI
// reconcile on the player-event-loop goroutine, so both sides take s.mu.
func (s *bedrockSession) chunkCenter() (cx, cz int32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastChunkX, s.lastChunkZ
}

// setChunkCenter records the viewer's current chunk under the session lock.
func (s *bedrockSession) setChunkCenter(cx, cz int32) {
	s.mu.Lock()
	s.lastChunkX, s.lastChunkZ = cx, cz
	s.mu.Unlock()
}

func (s *bedrockSession) write(pk packet.Packet) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.conn.WritePacket(pk)
}

// SendMessage implements player.Controller: chat to this Bedrock client.
func (s *bedrockSession) SendMessage(msg string) {
	s.write(&packet.Text{TextType: packet.TextTypeRaw, Message: msg})
}

// Kick implements player.Controller: disconnect this Bedrock client.
func (s *bedrockSession) Kick(reason string) {
	_ = s.conn.Close()
}

// Push implements player.Controller: apply a velocity impulse (blocks/tick) to
// the local player. Bedrock velocity is mgl32.Vec3 in blocks/tick. The local
// player knows itself as bedrockLocalRuntime (1), NOT bs.runtimeID (the id other
// viewers see) â€” targeting the wrong id silently no-ops.
func (s *bedrockSession) Push(vx, vy, vz float64) {
	// Bedrock here uses client-authoritative movement (StartGame leaves
	// MovementType=Client), so the client only partially applies a server
	// SetActorMotion to its own player â€” a Bedrock player shoved by a Java player
	// feels "heavy" / barely moves while the Java player (whose client applies
	// SetEntityMotion in full) moves freely, i.e. the push is lopsided. We amplify
	// the horizontal impulse to compensate. The push loop's force is bounded by
	// pushStrength(0.08), so even at full overlap this stays at ~0.36 b/t (melee-
	// knockback scale) and never launches. NOTE: true cross-edition parity needs
	// server-authoritative movement (specs Â§6); this is the bounded interim tuning.
	vx *= 4.5
	vz *= 4.5

	s.write(&packet.SetActorMotion{
		EntityRuntimeID: bedrockLocalRuntime,
		Velocity:        mgl32.Vec3{float32(vx), float32(vy), float32(vz)},
	})
}

// Hurt implements player.Controller: apply melee damage to this Bedrock player —
// drop its health bar and play the hurt flash + sound on its own client.
func (s *bedrockSession) Hurt(amount float32) {
	s.mu.Lock()
	s.health -= amount
	if s.health < 0 {
		s.health = 0
	}
	hp := s.health
	s.mu.Unlock() // write() also takes s.mu; never hold it across a write
	s.write(&packet.ActorEvent{EntityRuntimeID: bedrockLocalRuntime, EventType: packet.ActorEventHurt})
	s.write(&packet.UpdateAttributes{
		EntityRuntimeID: bedrockLocalRuntime,
		Attributes:      []protocol.Attribute{bedrockHealthAttribute(hp)},
	})
}

func (s *Server) addSession(bs *bedrockSession) {
	s.sessionsMu.Lock()
	s.sessions[bs.id.String()] = bs
	s.sessionsMu.Unlock()
}

func (s *Server) removeSession(id uuid.UUID) {
	s.sessionsMu.Lock()
	delete(s.sessions, id.String())
	s.sessionsMu.Unlock()
}

func (s *Server) getSession(id uuid.UUID) (*bedrockSession, bool) {
	s.sessionsMu.RLock()
	bs, ok := s.sessions[id.String()]
	s.sessionsMu.RUnlock()
	return bs, ok
}

func (s *Server) forEachSession(fn func(*bedrockSession)) {
	s.sessionsMu.RLock()
	list := make([]*bedrockSession, 0, len(s.sessions))
	for _, bs := range s.sessions {
		list = append(list, bs)
	}
	s.sessionsMu.RUnlock()
	for _, bs := range list {
		fn(bs)
	}
}
