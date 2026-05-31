// Package network menyediakan konstanta terkait networking dan protokol.
package network

// Default network ports untuk setiap protokol
const (
	// DefaultJavaPort adalah port default untuk Java Edition server
	DefaultJavaPort = 25565

	// DefaultBedrockPort adalah port default untuk Bedrock Edition server
	DefaultBedrockPort = 19132

	// DefaultBindAddress adalah alamat bind default (listen on all interfaces)
	DefaultBindAddress = "0.0.0.0"

	// LocalhostAddress adalah alamat localhost
	LocalhostAddress = "127.0.0.1"
)

// Protocol namespaces
const (
	// MinecraftNamespace adalah namespace default untuk Minecraft resources
	MinecraftNamespace = "minecraft:"

	// BedrockNamespace adalah namespace untuk Bedrock-specific resources
	BedrockNamespace = "bedrock:"
)

// Protocol versions dan identifiers
const (
	// ProtocolName adalah nama protokol untuk logging
	ProtocolNameJava    = "Java"
	ProtocolNameBedrock = "Bedrock"
)
