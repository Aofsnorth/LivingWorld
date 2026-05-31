package world

import (
	"log"
	"sync"

	dfbiome "github.com/df-mc/dragonfly/server/world/biome"
	dfchunk "github.com/df-mc/dragonfly/server/world/chunk"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
)

// ChunkCache is a thread-safe cache wrapper for Bedrock chunks to prevent concurrent
// access issues between player connection loops and block update loops.
type ChunkCache struct {
	Mu    sync.RWMutex
	Cache map[protocol.ChunkPos]*dfchunk.Chunk
}

func NewChunkCache() *ChunkCache {
	return &ChunkCache{
		Cache: make(map[protocol.ChunkPos]*dfchunk.Chunk),
	}
}

// ChunkConverter handles converting internal world chunks to Bedrock format.
type ChunkConverter struct{}

func NewChunkConverter() *ChunkConverter {
	return &ChunkConverter{}
}

// BlockRID resolves a block's runtime ID for Bedrock.
func BlockRID(name string, properties ...map[string]any) uint32 {
	var props map[string]any
	if len(properties) > 0 {
		props = properties[0]
	}
	rid, ok := dfchunk.StateToRuntimeID(name, props)
	if !ok {
		log.Printf("[Bedrock] Could not resolve runtime ID for %s", name)
		return 0
	}
	return rid
}

// LogBlockPaletteVersion logs key diagnostic info about the block palette.
func LogBlockPaletteVersion() {
	airRID := BlockRID("minecraft:air")
	bedrockRID := BlockRID("minecraft:bedrock")
	dirtRID := BlockRID("minecraft:dirt")
	grassRID := BlockRID("minecraft:grass_block", map[string]any{"minecraft:snowy_bit": false})
	plainsBiome := uint32(dfbiome.Plains{}.EncodeBiome())

	_, props, found := dfchunk.RuntimeIDToState(airRID)
	ver := int32(-1)
	if found {
		if v, ok := props["version"]; ok {
			ver = v.(int32)
		}
	}

	log.Printf("[Bedrock] Block palette: air=%d bedrock=%d dirt=%d grass=%d biome_plains=%d",
		airRID, bedrockRID, dirtRID, grassRID, plainsBiome)
	log.Printf("[Bedrock] air state: found=%v props=%+v blockVersion=%d", found, props, ver)
	log.Printf("[Bedrock] gophertunnel protocol=%d version=%s",
		protocol.CurrentProtocol, protocol.CurrentVersion)

	if airRID == 0 {
		log.Printf("[Bedrock] WARNING: air RID is 0 â€” blocks will be invisible!")
	}
	if bedrockRID == 0 || dirtRID == 0 || grassRID == 0 {
		log.Printf("[Bedrock] WARNING: one or more terrain block RIDs are 0 â€” palette mismatch likely")
	}
}
