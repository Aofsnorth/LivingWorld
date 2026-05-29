package bedrock

import (
	"math"
	"sync"
	"time"

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

	lastMovePublish     time.Time
	lastAuthInputAt     time.Time
	lastX, lastY, lastZ float64

	mu sync.Mutex
}

func newBedrockSession(id uuid.UUID, username string, runtimeID uint64, conn *minecraft.Conn) *bedrockSession {
	return &bedrockSession{id: id, username: username, runtimeID: runtimeID, conn: conn, identity: conn.IdentityData(), clientData: conn.ClientData()}
}

func (s *bedrockSession) write(pk packet.Packet) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.conn.WritePacket(pk)
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

func bedrockMetadata(name string) protocol.EntityMetadata {
	meta := protocol.NewEntityMetadata()
	meta.SetFlag(protocol.EntityDataKeyFlags, protocol.EntityDataFlagHasGravity)
	meta.SetFlag(protocol.EntityDataKeyFlags, protocol.EntityDataFlagHasCollision)
	meta.SetFlag(protocol.EntityDataKeyFlags, protocol.EntityDataFlagShowName)
	meta.SetFlag(protocol.EntityDataKeyFlags, protocol.EntityDataFlagAlwaysShowName)
	meta[protocol.EntityDataKeyName] = name
	meta[protocol.EntityDataKeyScale] = float32(1)
	meta[protocol.EntityDataKeyWidth] = float32(0.6)
	meta[protocol.EntityDataKeyHeight] = float32(1.8)
	return meta
}

func bedrockSurvivalAbilityData(runtimeID uint64) protocol.AbilityData {
	return protocol.AbilityData{
		EntityUniqueID:     int64(runtimeID),
		PlayerPermissions:  1,
		CommandPermissions: protocol.CommandPermissionLevelAny,
		Layers: []protocol.AbilityLayer{{
			Type: protocol.AbilityLayerTypeBase,
			Abilities: protocol.AbilityBuild |
				protocol.AbilityMine |
				protocol.AbilityDoorsAndSwitches |
				protocol.AbilityOpenContainers |
				protocol.AbilityAttackPlayers |
				protocol.AbilityAttackMobs,
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
	dx, dy, dz := x-s.lastX, y-s.lastY, z-s.lastZ
	horizontal := math.Sqrt(dx*dx + dz*dz)

	// Do not teleport on normal high-frequency packet throttling; that was
	// freezing the local Bedrock player. Only correct truly impossible movement.
	if dt < 0.05 {
		return false, false
	}
	maxHorizontal := 12.0*dt + 0.8
	maxVertical := 12.0*dt + 1.2
	if horizontal > maxHorizontal || math.Abs(dy) > maxVertical {
		return false, true
	}
	s.lastMovePublish = now
	s.lastX, s.lastY, s.lastZ = x, y, z
	return true, false
}
