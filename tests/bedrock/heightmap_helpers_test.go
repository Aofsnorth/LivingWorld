package bedrocktests

import (
	dfworld "github.com/df-mc/dragonfly/server/world"
	dfchunk "github.com/df-mc/dragonfly/server/world/chunk"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
)

const (
	dfchunkSubChunkHasData = protocol.HeightMapDataHasData
	dfchunkSubChunkTooLow  = protocol.HeightMapDataTooLow
	dfchunkSubChunkTooHigh = protocol.HeightMapDataTooHigh
)

// subChunkHeightMapForTest exposes the internal classifier via the same
// algorithm the production code uses (kept in one place: send.go's
// subChunkHeightMap is unexported; this test-side copy asserts the CONTRACT).
func subChunkHeightMapForTest(ch *dfchunk.Chunk, ind int16) (byte, []int8, bool) {
	hm := ch.HeightMap()
	higher, lower := true, true
	subMap := make([]int8, 256)
	minInd := int16(dfworld.Overworld.Range().Min() >> 4)
	for z := uint8(0); z < 16; z++ {
		for x := uint8(0); x < 16; x++ {
			y := hm.At(x, z)
			otherInd := (y >> 4) - minInd
			switch {
			case otherInd > ind:
				subMap[z*16+x], lower = 16, false
			case otherInd < ind:
				subMap[z*16+x], higher = -1, false
			default:
				subMap[z*16+x], lower, higher = int8(y&15), false, false
			}
		}
	}
	subMapType := byte(protocol.HeightMapDataHasData)
	switch {
	case higher:
		subMapType = protocol.HeightMapDataTooHigh
		subMap = nil
	case lower:
		subMapType = protocol.HeightMapDataTooLow
		subMap = nil
	}
	return subMapType, subMap, subMapType == protocol.HeightMapDataHasData
}
