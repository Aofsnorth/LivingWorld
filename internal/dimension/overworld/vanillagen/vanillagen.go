// Package vanillagen adapts bedrock-mc/vanilla-gen — a vanilla-faithful
// Dragonfly world generator — to LivingWorld's canonical chunk model.
//
// vanilla-gen writes Bedrock runtime IDs (plus 3D biomes) into a dragonfly
// chunk. The adapter runs it into a scratch dragonfly chunk, then walks
// the chunk translating each Bedrock state to the canonical Java global
// state ID (internal/blockconv + world.StateIDWithProperties), so the
// result flows through the same storage, light engine, Java protocol and
// Bedrock serving paths as every other LivingWorld generator. Stateful
// blocks keep their state (log axes, stair facings, snow thickness, fluid
// levels); states with no Java representation fall back to the block's
// default Java state, and names missing from the Java palette are dropped
// to air and counted (see the package-level counters) rather than failing
// the chunk.
//
// The adapter generates chunks concurrently: World.LoadChunk holds the
// world write lock while generating, but independent worlds and test
// harnesses may generate in parallel. vanilla-gen documents
// ConcurrentChunkGeneration() == true, and each Generate call allocates
// its own scratch chunk; only the RID translation caches below are
// shared, behind their own lock.
//
// Local prototype: the upstream module currently ships no LICENSE file.
// Do not redistribute builds containing it until that is resolved.
package vanillagen

import (
	"log"
	"sync"

	vanilla "github.com/bedrock-mc/vanilla-gen"
	// Registers the vanilla block implementations vanilla-gen needs on
	// top of upstream dragonfly (amethyst, chorus, ...). The blank import
	// of dragonfly/server/block inside that package also performs the
	// registry registration LivingWorld's Bedrock layer relies on.
	_ "github.com/bedrock-mc/vanilla-gen/block"
	dfworld "github.com/df-mc/dragonfly/server/world"
	dfchunk "github.com/df-mc/dragonfly/server/world/chunk"

	"livingworld/internal/blockconv"
	lwworld "livingworld/internal/world"
)

func init() {
	// Generator construction finalizes the registry too, but the scratch
	// chunk allocation below happens after New returns; finalize up front
	// so the invariant "registry finalized before any RID is read" is
	// obvious at the point of use. Finalize is idempotent in dragonfly.
	dfworld.DefaultBlockRegistry.Finalize()
}

// Generator generates canonical chunks with vanilla-gen. It implements
// world.ChunkGenerator.
type Generator struct {
	gen vanilla.Generator
	dim dfworld.Dimension

	mu        sync.Mutex
	ridToID   map[uint32]int32
	unmapped  map[string]struct{}
	converted uint64
	dropped   uint64
}

// Compile-time contract check.
var _ lwworld.ChunkGenerator = (*Generator)(nil)

// New returns an overworld generator for the given world seed.
func New(seed int64) *Generator {
	return newForDimension(seed, dfworld.Overworld)
}

// NewNether returns a nether generator for the given world seed.
func NewNether(seed int64) *Generator {
	return newForDimension(seed, dfworld.Nether)
}

// NewEnd returns an end generator for the given world seed.
func NewEnd(seed int64) *Generator {
	return newForDimension(seed, dfworld.End)
}

func newForDimension(seed int64, dim dfworld.Dimension) *Generator {
	g := &Generator{
		gen:     vanilla.NewForDimension(seed, dim),
		dim:     dim,
		ridToID: make(map[uint32]int32),
	}
	return g
}

// Generate builds the canonical chunk at (cx, cz).
func (g *Generator) Generate(cx, cz int) *lwworld.Chunk {
	rng := g.dim.Range()
	scratch := dfchunk.New(dfworld.DefaultBlockRegistry, rng)
	g.gen.GenerateChunk(dfworld.ChunkPos{int32(cx), int32(cz)}, scratch)

	out := lwworld.NewChunk()
	minY, maxY := rng.Min(), rng.Max()
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			highest := minY - 1
			for y := minY; y <= maxY; y++ {
				rid := scratch.Block(uint8(x), int16(y), uint8(z), 0)
				id, isAir := g.javaStateID(rid)
				if isAir {
					// Layer 1 (waterlogging) has no canonical
					// representation; vanilla-gen only displaces
					// liquids there, and the layer-0 state already
					// carries the fluid where both coincide.
					continue
				}
				out.SetBlock(x, y, z, lwworld.BlockByID(id))
				if y > highest {
					highest = y
				}
			}
			// SetHeightmap expects the surface Y; consumers add their
			// own conventions on top (HighestSolidY adds 1).
			out.SetHeightmap(x, z, int32(highest))
		}
	}
	return out
}

// javaStateID translates one Bedrock runtime ID to the canonical Java
// state ID, memoized per RID. The second result reports whether the
// canonical block is air (either real air or a dropped unmapped state).
func (g *Generator) javaStateID(rid uint32) (int32, bool) {
	g.mu.Lock()
	if id, ok := g.ridToID[rid]; ok {
		g.mu.Unlock()
		return id, id == lwworld.AirID
	}
	name, props, found := dfworld.DefaultBlockRegistry.RuntimeIDToState(rid)
	if !found {
		g.ridToID[rid] = lwworld.AirID
		g.mu.Unlock()
		return lwworld.AirID, true
	}
	javaName, javaProps, translated := blockconv.Translate(name, props)
	id := lwworld.StateID(javaName)
	if translated {
		id = lwworld.StateIDWithProperties(javaName, javaProps)
	}
	g.converted++
	if id == lwworld.AirID && javaName != "minecraft:air" {
		// The Bedrock block has no canonical representation. Count each
		// distinct name once so palette drift is visible without log spam.
		g.dropped++
		if g.unmapped == nil {
			g.unmapped = make(map[string]struct{})
		}
		if _, seen := g.unmapped[javaName]; !seen {
			g.unmapped[javaName] = struct{}{}
			log.Printf("[vanillagen] block %s has no canonical Java state; generating as air", javaName)
		}
	}
	g.ridToID[rid] = id
	g.mu.Unlock()
	return id, id == lwworld.AirID
}

// Stats reports translation totals: converted counts distinct Bedrock
// states resolved to canonical IDs, dropped counts distinct states that
// had to be generated as air. Useful for palette-coverage diagnostics.
func (g *Generator) Stats() (converted, dropped uint64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.converted, g.dropped
}
