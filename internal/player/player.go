package player

import (
	"livingworld/internal/shared/constants/gameplay"
	"livingworld/internal/shared/constants/system"
	"livingworld/internal/world"

	"github.com/google/uuid"
)

func NewPlayer(uuid_ uuid.UUID, username string, edition Edition) *Player {
	return &Player{
		UUID:       uuid_,
		Username:   username,
		Edition:    edition,
		Health:     gameplay.MaxHealth,
		Food:       gameplay.MaxFood,
		Saturation: gameplay.DefaultSaturation,
		Inventory:  NewInventory(),
		Position:   world.Position{X: 0, Y: defaultSpawnY, Z: 0},
		Rotation:   world.Rotation{Pitch: 0, Yaw: 0},
		OnGround:   true,
		SkinParts:  system.DefaultSkinParts,
	}
}

func (p *Player) Snapshot() PlayerSnapshot {
	return PlayerSnapshot{
		UUID:              p.UUID,
		Username:          p.Username,
		Edition:           p.Edition,
		EntityRuntimeID:   p.EntityRuntimeID,
		Position:          p.Position,
		Rotation:          p.Rotation,
		OnGround:          p.OnGround,
		Sneaking:          p.Sneaking,
		ProfileProperties: append([]ProfileProperty(nil), p.ProfileProperties...),
		BedrockSkinURL:    p.BedrockSkinURL,
		Skin:              p.Skin,
		SkinParts:         p.SkinParts,
		Creative:          p.Creative,
	}
}

func (p *Player) Teleport(x, y, z float64) {
	p.Position = world.Position{X: x, Y: y, Z: z}
}

func (p *Player) SetRotation(pitch, yaw float32) {
	p.Rotation = world.Rotation{Pitch: pitch, Yaw: yaw}
}

func (p *Player) Damage(amount float32) {
	p.Health -= amount
	if p.Health < 0 {
		p.Health = 0
	}
}

func (p *Player) Heal(amount float32) {
	p.Health += amount
	if p.Health > gameplay.MaxHealth {
		p.Health = gameplay.MaxHealth
	}
}

func (p *Player) SendMessage(message string)       {}
func (p *Player) SendTitle(title, subtitle string) {}
func (p *Player) Kick(reason string)               {}
func (p *Player) Push(vx, vy, vz float64)          {}
func (p *Player) Hurt(amount float32)              {}
