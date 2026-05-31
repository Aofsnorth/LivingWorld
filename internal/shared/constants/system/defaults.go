// Package system menyediakan konstanta terkait system configuration.
package system

// Server defaults
const (
	// DefaultServerName adalah nama server default
	DefaultServerName = "LivingWorld Server"

	// DefaultMOTD adalah MOTD default
	DefaultMOTD = "A Minecraft Server"

	// DefaultMaxPlayers adalah jumlah maksimum players default
	DefaultMaxPlayers = 100

	// DefaultWorldSeed adalah seed default untuk world generation
	DefaultWorldSeed = int64(12345)
)

// Player defaults
const (
	// DefaultSpawnX adalah X coordinate default untuk spawn
	DefaultSpawnX = 0.0

	// DefaultSpawnY adalah Y coordinate default untuk spawn
	DefaultSpawnY = 4.0

	// DefaultSpawnZ adalah Z coordinate default untuk spawn
	DefaultSpawnZ = 0.0

	// DefaultSpawnYaw adalah yaw default untuk spawn
	DefaultSpawnYaw = 0.0

	// DefaultSpawnPitch adalah pitch default untuk spawn
	DefaultSpawnPitch = 0.0

	// DefaultSkinParts adalah skin parts default (semua enabled)
	DefaultSkinParts = byte(0x7F)
)

// Bedrock specific defaults
const (
	// BedrockLocalRuntime adalah runtime ID untuk local player di Bedrock
	BedrockLocalRuntime = uint64(1)

	// BedrockPlayerRuntimeIDOffset adalah offset untuk player runtime IDs
	BedrockPlayerRuntimeIDOffset = uint64(100000)
)

// World type names
const (
	// WorldTypeSuperflat adalah identifier untuk superflat world type
	WorldTypeSuperflat = "superflat"

	// WorldTypeDefault adalah world type default
	WorldTypeDefault = WorldTypeSuperflat
)

// Skin source names
const (
	// SkinSourceAuto adalah auto skin source (try all)
	SkinSourceAuto = "auto"

	// SkinSourceMojang adalah Mojang skin source
	SkinSourceMojang = "mojang"

	// SkinSourceEly adalah Ely.by skin source
	SkinSourceEly = "ely"

	// SkinSourceNone adalah no skin fetching
	SkinSourceNone = "none"
)
