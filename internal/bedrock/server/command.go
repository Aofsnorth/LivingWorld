package server

import (
	"fmt"
	"strings"

	"livingworld/internal/command"
	"livingworld/internal/player"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/google/uuid"
	"github.com/sandertv/gophertunnel/minecraft"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

// sendAvailableCommands advertises the registered commands so the Bedrock client
// autocompletes them and sends CommandRequest. Each command gets one optional
// rawtext overload (server-side parsing does the real work). Bedrock requires
// lowercase command names.
func (s *Server) sendAvailableCommands(conn *minecraft.Conn) {
	cmds := make([]protocol.Command, 0)
	for _, c := range command.Default().Commands() {
		perm := byte(protocol.CommandPermissionLevelAny)
		if c.Permission == command.PermOperator {
			perm = protocol.CommandPermissionLevelGameDirectors
		}
		cmds = append(cmds, protocol.Command{
			Name:            strings.ToLower(c.Name),
			Description:     c.Description,
			PermissionLevel: perm,
			Overloads: []protocol.CommandOverload{{
				Parameters: []protocol.CommandParameter{{
					Name:     "args",
					Type:     protocol.CommandArgValid | protocol.CommandArgTypeRawText,
					Optional: c.MinArgs == 0,
				}},
			}},
		})
	}
	_ = conn.WritePacket(&packet.AvailableCommands{Commands: cmds})
}

// --- command.Sender implementation on the Bedrock session ---

func (s *bedrockSession) Name() string    { return s.username }
func (s *bedrockSession) UUID() uuid.UUID { return s.id }

func (s *bedrockSession) IsOp() bool {
	if pl := s.pmRef().GetPlayer(s.id); pl != nil {
		return pl.Op
	}
	return false
}

func (s *bedrockSession) Edition() player.Edition { return player.EditionBedrock }

func (s *bedrockSession) Reply(msg string) { s.SendMessage(msg) }

// javaModeToBedrock maps the command system's Java-style gamemode (0 survival,
// 1 creative, 2 adventure, 3 spectator) to Bedrock's GameType constants.
func javaModeToBedrock(mode int) int32 {
	switch mode {
	case 1:
		return packet.GameTypeCreative
	case 2:
		return packet.GameTypeAdventure
	case 3:
		return packet.GameTypeSpectator
	default:
		return packet.GameTypeSurvival
	}
}

func (s *bedrockSession) SetGameMode(mode int) error {
	if mode < 0 || mode > 3 {
		return fmt.Errorf("invalid gamemode %d", mode)
	}
	s.write(&packet.SetPlayerGameType{GameType: javaModeToBedrock(mode)})
	return nil
}

func (s *bedrockSession) Teleport(x, y, z float64) error {
	s.write(&packet.MovePlayer{
		EntityRuntimeID: bedrockLocalRuntime,
		Position:        mgl32.Vec3{float32(x), float32(y) + bedrockLocalEyeHeight, float32(z)},
		Mode:            packet.MoveModeTeleport,
		OnGround:        false,
		TeleportCause:   packet.TeleportCauseCommand,
	})
	s.pmRef().UpdatePosition(s.id, x, y, z, 0, 0, false)
	return nil
}

func (s *bedrockSession) GiveItem(itemName string, count int) error {
	// Bedrock item-stack giving requires runtime-ID resolution + InventorySlot;
	// not yet wired. Validate name so feedback is still useful.
	return fmt.Errorf("/give is not yet supported on Bedrock")
}
