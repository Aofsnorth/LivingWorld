package bedrock

import (
	"sync"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/google/uuid"
	"github.com/sandertv/gophertunnel/minecraft"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/login"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

// bedrockSession is the protocol-side representation of one connected Bedrock
// client. Keep protocol-specific details here so handler.go stays focused on
// connection flow and packet dispatch.
type bedrockSession struct {
	id        uuid.UUID
	username  string
	runtimeID uint64

	conn       *minecraft.Conn
	identity   login.IdentityData
	clientData login.ClientData

	mu sync.Mutex
}

func newBedrockSession(id uuid.UUID, username string, runtimeID uint64, conn *minecraft.Conn) *bedrockSession {
	return &bedrockSession{
		id:         id,
		username:   username,
		runtimeID:  runtimeID,
		conn:       conn,
		identity:   conn.IdentityData(),
		clientData: conn.ClientData(),
	}
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
		PlayerPermissions:  1, // member
		CommandPermissions: protocol.CommandPermissionLevelAny,
		Layers: []protocol.AbilityLayer{{
			Type:             protocol.AbilityLayerTypeBase,
			Abilities:        protocol.AbilityBuild | protocol.AbilityMine | protocol.AbilityDoorsAndSwitches | protocol.AbilityOpenContainers | protocol.AbilityAttackPlayers | protocol.AbilityAttackMobs | protocol.AbilityWalkSpeed,
			Values:           protocol.AbilityBuild | protocol.AbilityMine | protocol.AbilityDoorsAndSwitches | protocol.AbilityOpenContainers | protocol.AbilityAttackPlayers | protocol.AbilityAttackMobs | protocol.AbilityWalkSpeed,
			FlySpeed:         protocol.AbilityBaseFlySpeed,
			VerticalFlySpeed: protocol.AbilityBaseVerticalFlySpeed,
			WalkSpeed:        protocol.AbilityBaseWalkSpeed,
		}},
	}
}

func bedrockPosFromFeet(x, y, z float64) mgl32.Vec3 {
	return mgl32.Vec3{float32(x), float32(y), float32(z)}
}
