package world

import (
	"log"
	"sync"

	// Side-effect import: registers all vanilla block implementations (light
	// filter/emission, hardness, ...) into DefaultBlockRegistry via init().
	// Without it Finalize() ships blockInfos where AIR has lightFilter 15 —
	// every heightmap computes a bogus surface and the client renders terrain
	// in misplaced bands (the corrupted-terrain report). Dragonfly servers get
	// this for free by importing server/block.
	_ "github.com/df-mc/dragonfly/server/block"

	dfworld "github.com/df-mc/dragonfly/server/world"
	dfchunk "github.com/df-mc/dragonfly/server/world/chunk"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
)

func init() {
	// Finalise the vanilla Bedrock block registry so that runtime-ID lookups work
	// independently of a Dragonfly world instance.
	dfworld.DefaultBlockRegistry.Finalize()
}

// ChunkCache is a thread-safe cache wrapper for Bedrock chunks to prevent concurrent
// access issues between player connection loops and block update loops.
//
// Mu guards the MAP itself; chunkMu guards the CHUNK BODIES: dragonfly's
// dfchunk.Chunk is not thread-safe, and HandleSubChunkRequest encodes a chunk
// (EncodeSubChunk + HeightMap) while broadcastBlockUpdate concurrently calls
// SetBlock on the same cached chunk — a torn encode can ship a corrupted
// palette to the client. Encode paths take chunkMu.RLock per chunk pos, and
// SetBlock paths take chunkMu.Lock.
type ChunkCache struct {
	Mu    sync.RWMutex
	Cache map[protocol.ChunkPos]*dfchunk.Chunk

	chunkMuMu sync.Mutex
	chunkMu   map[protocol.ChunkPos]*sync.RWMutex
}

func NewChunkCache() *ChunkCache {
	return &ChunkCache{
		Cache:   make(map[protocol.ChunkPos]*dfchunk.Chunk),
		chunkMu: make(map[protocol.ChunkPos]*sync.RWMutex),
	}
}

// chunkLock returns (creating if needed) the body lock for a chunk position.
func (c *ChunkCache) chunkLock(pos protocol.ChunkPos) *sync.RWMutex {
	c.chunkMuMu.Lock()
	defer c.chunkMuMu.Unlock()
	if c.chunkMu == nil {
		c.chunkMu = make(map[protocol.ChunkPos]*sync.RWMutex)
	}
	l, ok := c.chunkMu[pos]
	if !ok {
		l = &sync.RWMutex{}
		c.chunkMu[pos] = l
	}
	return l
}

// WithChunkBody runs fn while holding the chunk body lock shared (encode/read).
// The chunk must already be present in the cache (or fn must not touch it).
func (c *ChunkCache) WithChunkBody(pos protocol.ChunkPos, fn func(ch *dfchunk.Chunk)) {
	c.Mu.RLock()
	ch, ok := c.Cache[pos]
	c.Mu.RUnlock()
	if !ok {
		return
	}
	l := c.chunkLock(pos)
	l.RLock()
	defer l.RUnlock()
	fn(ch)
}

// MutateChunkBody runs fn while holding the chunk body lock exclusive
// (SetBlock etc.). No-op if the chunk is not cached.
func (c *ChunkCache) MutateChunkBody(pos protocol.ChunkPos, fn func(ch *dfchunk.Chunk)) {
	c.Mu.RLock()
	ch, ok := c.Cache[pos]
	c.Mu.RUnlock()
	if !ok {
		return
	}
	l := c.chunkLock(pos)
	l.Lock()
	defer l.Unlock()
	fn(ch)
}

// ChunkConverter handles converting internal world chunks to Bedrock format.
type ChunkConverter struct{}

func NewChunkConverter() *ChunkConverter {
	return &ChunkConverter{}
}

// BlockRID resolves a block's runtime ID for Bedrock. Note the returned id is
// only meaningful when the name exists in the registry — runtime ID 0 is a
// real block (dragonfly sorts by FNV name hash; the lowest belongs to
// minecraft:cyan_terracotta), so a bare 0 is NOT an air/failure sentinel.
// Prefer blockRIDFound to check resolvability.
func BlockRID(name string, properties ...map[string]any) uint32 {
	rid, _ := blockRIDFoundProps(name, properties...)
	return rid
}

// blockRID resolves name (+optional properties) and reports whether the
// registry actually contains it. On failure the returned rid is the registry's
// raw fallback (often 0 = cyan_terracotta) and must not be used as block data.
func blockRID(name string, props map[string]any) uint32 {
	rid, _ := blockRIDFoundProps(name, props)
	return rid
}

// blockRIDFound reports whether the given name (with its default properties,
// plus any caller overrides) exists in the finalized Bedrock registry.
func blockRIDFound(name string) bool {
	_, ok := blockRIDFoundProps(name)
	return ok
}

// BlockRIDFound reports whether the namespaced block name exists in the
// finalized Bedrock registry (any default state). Exported for regression
// tests that pin registry invariants.
func BlockRIDFound(name string) bool {
	return blockRIDFound(name)
}

func blockRIDFoundProps(name string, properties ...map[string]any) (uint32, bool) {
	var props map[string]any
	if len(properties) > 0 && properties[0] != nil {
		props = properties[0]
	}
	rid, ok := dfworld.DefaultBlockRegistry.StateToRuntimeID(name, props)
	if !ok && props != nil {
		// Retry without property overrides so callers can distinguish
		// "state exists, props wrong" (still found, default state) from
		// "name entirely missing".
		rid, ok = dfworld.DefaultBlockRegistry.StateToRuntimeID(name, nil)
	}
	return rid, ok
}

// LogBlockPaletteVersion logs key diagnostic info about the block palette.
// It resolves a spread of terrain-critical names and reports any that the
// Bedrock registry cannot represent. Comparisons are by RESOLVABILITY, never
// by rid==0: runtime ID 0 is a valid block in the FNV-sorted registry, so
// "rid==0 means broken" would never fire.
func LogBlockPaletteVersion() {
	airRID, airOK := blockRIDFoundProps("minecraft:air")
	bedrockOK := blockRIDFound("minecraft:bedrock")
	dirtOK := blockRIDFound("minecraft:dirt")
	grassOK := blockRIDFound("minecraft:grass_block")

	if !airOK {
		log.Printf("[Bedrock] WARNING: minecraft:air missing from Bedrock registry")
	} else {
		log.Printf("[Bedrock] palette: airRID=%d (rid 0 is a real block, not air)", airRID)
	}
	if !bedrockOK || !dirtOK || !grassOK {
		log.Printf("[Bedrock] WARNING: core terrain blocks unresolvable (bedrock=%v dirt=%v grass_block=%v); palette mismatch likely",
			bedrockOK, dirtOK, grassOK)
	}
}
