// Package gameplay menyediakan konstanta terkait gameplay mechanics.
package gameplay

// Player push mechanics constants
const (
	// PushTickHz adalah frekuensi push calculation per detik
	PushTickHz = 10

	// PushRadius adalah jarak horizontal maksimum untuk player push (dalam blocks)
	PushRadius = 0.6

	// PushVertical adalah gap vertikal maksimum untuk player push (dalam blocks)
	PushVertical = 1.0

	// PushStrength adalah kekuatan base push force
	PushStrength = 0.08

	// PushMaxPerTick adalah velocity maksimum per tick untuk mencegah launch
	PushMaxPerTick = 0.4
)

// Fall damage constants
const (
	// FallSafeBlocks adalah jarak jatuh yang aman tanpa damage (dalam blocks)
	FallSafeBlocks = 3.0
)

// Block breaking constants
const (
	// BedrockCrackBreakSeconds adalah durasi default untuk break animation di Bedrock
	BedrockCrackBreakSeconds = 0.75
)

// Movement detection constants
const (
	// TeleportDistanceSquared adalah threshold untuk mendeteksi teleport vs movement normal
	// Jika movement > 4 blocks (~16 squared), dianggap teleport
	TeleportDistanceSquared = 16.0
)
