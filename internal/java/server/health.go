package server

import (
	"livingworld/internal/world"

	"github.com/Tnze/go-mc/data/packetid"
	pk "github.com/Tnze/go-mc/net/packet"
)

// Vanilla survival defaults.
const (
	MaxHealth     float32 = 20
	MaxFood       int32   = 20
	MaxSaturation float32 = 5
)

// buildSetHealthPacket builds the clientbound SetHealth packet. food=20 is a
// full hunger bar; sending 10 (as the old code did) shows only half a bar.
func buildSetHealthPacket(health float32, food int32, saturation float32) pk.Packet {
	return pk.Marshal(
		packetid.ClientboundGameSetHealth,
		pk.Float(health),
		pk.VarInt(food),
		pk.Float(saturation),
	)
}

// sendHealth pushes the session's current health/food/saturation to the client.
func (s *PlayerSession) sendHealth() error {
	s.mu.Lock()
	h, f, sat := s.Health, s.Food, s.Saturation
	s.mu.Unlock()
	return s.SendPacket(buildSetHealthPacket(h, f, sat))
}

// HandleClientCommand processes ServerboundGameClientCommand (0 = PERFORM_RESPAWN).
func (s *PlayerSession) HandleClientCommand(p pk.Packet) {
	var actionID pk.VarInt
	if err := p.Scan(&actionID); err != nil {
		return
	}
	if actionID != 0 {
		return
	}

	spawn := s.Bridge.cfg.World.Spawn
	spawnX, spawnY, spawnZ := spawn.X, spawn.Y, spawn.Z
	if w := s.Bridge.wm.GetDefaultWorld(); w != nil {
		if topY := w.HighestSolidY(int(spawn.X), int(spawn.Z)); topY >= world.MinWorldHeight {
			spawnY = float64(topY + 1)
		}
	}

	s.mu.Lock()
	s.Health = MaxHealth
	s.Food = MaxFood
	s.Saturation = MaxSaturation
	s.FallDistance = 0
	s.OnGround = true
	s.X = spawnX
	s.Y = spawnY
	s.Z = spawnZ
	s.Yaw = spawn.Yaw
	s.Pitch = spawn.Pitch
	s.LoadedChunks = make(map[world.ChunkPos]bool)
	s.mu.Unlock()

	if s.Bridge.pm != nil {
		if pl := s.Bridge.pm.GetPlayer(s.UUIDVal); pl != nil {
			pl.Health = MaxHealth
			pl.Food = int(MaxFood)
			pl.Saturation = MaxSaturation
			pl.OnFire = false
			pl.AirSupply = 300
		}
		s.Bridge.pm.UpdatePosition(s.UUIDVal, spawnX, spawnY, spawnZ, spawn.Pitch, spawn.Yaw, true)
	}

	// Send ClientboundGameRespawn matching 26.3 CommonPlayerSpawnInfo + dataToKeep(0).
	_ = s.SendPacket(pk.Marshal(
		packetid.ClientboundGameRespawn,
		pk.VarInt(0),                         // SpawnInfo.dimension
		pk.Identifier("minecraft:overworld"), // SpawnInfo.name
		pk.Long(s.Bridge.cfg.World.Seed),     // SpawnInfo.hashedSeed
		pk.VarInt(int32(s.GameMode())),       // SpawnInfo.gamemode
		pk.VarInt(0),                         // SpawnInfo.previousGamemode (0 = empty)
		pk.Boolean(false),                    // SpawnInfo.isDebug
		pk.Boolean(false),                    // SpawnInfo.isFlat
		pk.Boolean(false),                    // SpawnInfo.death
		pk.VarInt(0),                         // SpawnInfo.portalCooldown
		pk.VarInt(63),                        // SpawnInfo.seaLevel
		pk.Byte(0),                           // dataToKeep
	))

	var abilFlags byte
	if s.GameMode() == 1 {
		abilFlags = 0x01 + 0x04 + 0x08
	} else if s.GameMode() == 3 {
		abilFlags = 0x01 + 0x02 + 0x04
	}
	_ = s.SendPacket(pk.Marshal(
		packetid.ClientboundGamePlayerAbilities,
		pk.Byte(abilFlags),
		pk.Float(0.05),
		pk.Float(0.1),
	))
	_ = s.SendPacket(buildPlayerPositionPacket(1, spawnX, spawnY, spawnZ, spawn.Yaw, spawn.Pitch))
	_ = s.SendPacket(pk.Marshal(
		packetid.ClientboundGameGameEvent,
		pk.UnsignedByte(13),
		pk.Float(0),
	))

	s.sendWorldState()
	_ = s.sendSpawnPosition()
	_ = s.sendHealth()
	s.syncInventory()
	s.sendWeather()
	s.mobViewer = newJavaMobTracker()
	s.updateChunks()
	s.spawnExistingMobs()
	s.spawnExistingDrops()
	s.spawnExistingForeignPlayers()
}
