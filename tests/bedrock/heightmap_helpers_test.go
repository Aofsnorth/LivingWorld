package bedrocktests

import (
	bedrockworld "livingworld/internal/bedrock/world"

	dfchunk "github.com/df-mc/dragonfly/server/world/chunk"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
)

const (
	dfchunkSubChunkHasData = protocol.HeightMapDataHasData
	dfchunkSubChunkTooLow  = protocol.HeightMapDataTooLow
	dfchunkSubChunkTooHigh = protocol.HeightMapDataTooHigh
)

// subChunkHeightMapForTest delegates directly to bedrockworld.SubChunkHeightMap
// so tests exercise the exact production heightmap implementation.
func subChunkHeightMapForTest(ch *dfchunk.Chunk, ind int16) (byte, protocol.Optional[protocol.HeightMap], bool) {
	return bedrockworld.SubChunkHeightMap(ch, ind)
}
