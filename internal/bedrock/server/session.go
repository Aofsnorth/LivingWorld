package server

import (
	"math"
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

	mu sync.Mutex
}

func newBedrockSession(id uuid.UUID, username string, runtimeID uint64, conn *minecraft.Conn, pm *player.Manager) *bedrockSession {
	return &bedrockSession{
		id:         id,
		username:   username,
		runtimeID:  runtimeID,
		conn:       conn,
		pm:         pm,
		identity:   conn.IdentityData(),
		clientData: conn.ClientData(),
		chunkCache: bedrockworld.NewChunkCache(),
		LoadedChunks: make(map[protocol.ChunkPos]bool),
		lastPubX:   -999999, // Trigger update immediately on first move
		lastPubZ:   -999999,
		lastChunkX: -999999,
		lastChunkZ: -999999,
		viewDistance: 0,
	}
}

func (s *bedrockSession) pmRef() *player.Manager { return s.pm }

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
// viewers see) — targeting the wrong id silently no-ops.
func (s *bedrockSession) Push(vx, vy, vz float64) {
	// Bedrock ground friction is extremely high compared to Java.
	// We amplify the horizontal knockback so the client actually gets moved by
	// the SetActorMotion packet without requiring an artificial vertical bump.
	vx *= 1.5
	vz *= 1.5

	s.write(&packet.SetActorMotion{
		EntityRuntimeID: bedrockLocalRuntime,
		Velocity:        mgl32.Vec3{float32(vx), float32(vy), float32(vz)},
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

func bedrockMetadata(name string, sneaking bool) protocol.EntityMetadata {
	meta := protocol.NewEntityMetadata()
	meta.SetFlag(protocol.EntityDataKeyFlags, protocol.EntityDataFlagHasGravity)
	meta.SetFlag(protocol.EntityDataKeyFlags, protocol.EntityDataFlagHasCollision)
	meta.SetFlag(protocol.EntityDataKeyFlags, protocol.EntityDataFlagShowName)
	meta.SetFlag(protocol.EntityDataKeyFlags, protocol.EntityDataFlagAlwaysShowName)
	meta.SetFlag(protocol.EntityDataKeyFlags, protocol.EntityDataFlagBreathing)

	meta[protocol.EntityDataKeyName] = name
	// The dedicated AlwaysShowNameTag byte (not just the flag) is what makes the
	// nametag render at any distance/angle; without it Bedrock fades it like a mob
	// nametag (only when close and looked at). Matches dragonfly's reference.
	meta[protocol.EntityDataKeyAlwaysShowNameTag] = uint8(1)
	meta[protocol.EntityDataKeyScale] = float32(1)
	meta[protocol.EntityDataKeyWidth] = float32(0.6)

	if sneaking {
		meta.SetFlag(protocol.EntityDataKeyFlags, protocol.EntityDataFlagSneaking)
		meta[protocol.EntityDataKeyPoseIndex] = int32(5)  // Crouching pose index
		meta[protocol.EntityDataKeyHeight] = float32(1.5) // Reduced height
	} else {
		meta[protocol.EntityDataKeyPoseIndex] = int32(0)  // Standing pose index
		meta[protocol.EntityDataKeyHeight] = float32(1.8) // Normal height
	}

	// Full air so remote player entities never render drowning bubble particles.
	meta[protocol.EntityDataKeyAirSupply] = int16(300)
	meta[protocol.EntityDataKeyAirSupplyMax] = int16(300)
	return meta
}

func bedrockSurvivalAbilityData(runtimeID uint64) protocol.AbilityData {
	return protocol.AbilityData{
		EntityUniqueID:     int64(runtimeID),
		PlayerPermissions:  0,
		CommandPermissions: 0,
		Layers: []protocol.AbilityLayer{{
			Type:      protocol.AbilityLayerTypeBase,
			Abilities: protocol.AbilityCount - 1,
			Values: protocol.AbilityBuild |
				protocol.AbilityMine |
				protocol.AbilityDoorsAndSwitches |
				protocol.AbilityOpenContainers |
				protocol.AbilityAttackPlayers |
				protocol.AbilityAttackMobs,
			FlySpeed:         protocol.AbilityBaseFlySpeed,
			VerticalFlySpeed: protocol.AbilityBaseVerticalFlySpeed,
			WalkSpeed:        protocol.AbilityBaseWalkSpeed,
		}},
	}
}

func bedrockMovementAttribute() protocol.Attribute {
	return protocol.Attribute{
		AttributeValue: protocol.AttributeValue{
			Name:  "minecraft:movement",
			Value: protocol.AbilityBaseWalkSpeed,
			Max:   math.MaxFloat32,
			Min:   0,
		},
		DefaultMin: 0,
		DefaultMax: math.MaxFloat32,
		Default:    protocol.AbilityBaseWalkSpeed,
	}
}

func bedrockLocalClientPosFromFeet(x, y, z float64) mgl32.Vec3 {
	return mgl32.Vec3{float32(x), float32(y + bedrockLocalEyeHeight), float32(z)}
}

func bedrockSharedFeetFromLocalClient(pos mgl32.Vec3) (x, y, z float64) {
	// The local Bedrock player is spawned/teleported with a camera-like Y
	// (feet + 1.62). Movement packets from this gophertunnel path remain in the
	// same visual coordinate space. Convert back to the shared feet coordinate
	// used by Java/world state; otherwise Java viewers see Bedrock players
	// floating about one eye-height above the grass.
	return float64(pos[0]), float64(pos[1]) - bedrockLocalEyeHeight, float64(pos[2])
}

func bedrockPosFromFeet(x, y, z float64) mgl32.Vec3 {
	return mgl32.Vec3{float32(x), float32(y), float32(z)}
}

func bedrockPosFromJavaFeet(x, y, z float64) mgl32.Vec3 {
	// Remote Java player entities in Bedrock need the same visual offset that
	// the local Bedrock client expects for player render positions. Without this
	// the Java player appears buried below the grass block.
	return mgl32.Vec3{float32(x), float32(y + bedrockLocalEyeHeight), float32(z)}
}

func (s *bedrockSession) movementUpdate(now time.Time, x, y, z float64) (publish bool, correct bool) {
	if s.lastMovePublish.IsZero() {
		s.lastMovePublish = now
		s.lastX, s.lastY, s.lastZ = x, y, z
		return true, false
	}
	dt := now.Sub(s.lastMovePublish).Seconds()
	if dt <= 0 {
		return false, false
	}
	s.lastMovePublish = now
	s.lastX, s.lastY, s.lastZ = x, y, z
	return true, false
}
