// Package overworld serves SteelMC's Java 26.3 overworld terrain for LivingWorld
// (both canonical *world.Chunk and Dragonfly *chunk.Chunk).
package overworld

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	lwworld "livingworld/internal/world"

	dfworld "github.com/df-mc/dragonfly/server/world"
	dfchunk "github.com/df-mc/dragonfly/server/world/chunk"
)

const (
	overworldMinY     = -64
	overworldHeight   = 384
	overworldSections = overworldHeight / 16
	chunkLimit        = 1_800_000
	maxWorkers        = 256
	maxCacheChunks    = 4096
)

var (
	// ErrClosed is returned when a generator is used after Close begins.
	ErrClosed = errors.New("generator closed")
	// ErrInvalidWire is returned when the Rust bridge emits a malformed chunk buffer.
	ErrInvalidWire = errors.New("invalid chunk wire format")
)

// Options configures a Generator.
type Options struct {
	// Registry resolves Bedrock block names and properties. Nil uses dfworld.DefaultBlockRegistry.
	Registry dfworld.BlockRegistry
	// Logger receives one warning per unmapped block state or biome. Nil uses slog.Default().
	Logger *slog.Logger
	// Workers is the Rayon thread count. Zero uses available cores minus one (minimum one).
	Workers int
	// CacheChunks bounds deterministic-mode encoded output retention. Zero defaults to 512.
	CacheChunks int
	// Deterministic generates fixed stage-ordered regions instead of reusing a streaming world;
	// use it in repeatable tests, not live play.
	Deterministic bool
}

// Stats snapshots generator activity.
type Stats struct {
	Chunks         uint64
	Blocks         uint64
	FallbackBlocks uint64
	UnmappedStates uint64
	UnmappedBiomes uint64
}

type canonicalResolved struct {
	id       int32
	fallback bool
	unmapped bool
}

// Generator produces overworld chunks through the Rust SteelMC bridge.
type Generator struct {
	ctx             *nativeContext
	translator      *translator
	logger          *slog.Logger
	opts            Options
	seed            int64
	wg              sync.WaitGroup
	mu              sync.Mutex
	closed          atomic.Bool
	chunks          atomic.Uint64
	blocks          atomic.Uint64
	fallbackBlocks  atomic.Uint64
	unmapped        atomic.Uint64
	unmappedBiomes  atomic.Uint64
	canonicalStates sync.Map // map[string]canonicalResolved
}

type seedChunkKey struct {
	seed   int64
	cx, cz int32
}

var (
	sharedGenMu       sync.Mutex
	sharedGens        = map[int64]*Generator{}
	encodedCache      sync.Map // map[seedChunkKey][]byte (bounded)
	encodedCacheCount atomic.Int64
)

const (
	maxEncodedCacheEntries = 32
	defaultCacheChunks     = 32
)

func defaultWorkers() int {
	return min(max(runtime.NumCPU()/3, 2), 4)
}

func (g *Generator) generateWireBytes(cx, cz int32) ([]byte, error) {
	key := seedChunkKey{seed: g.seed, cx: cx, cz: cz}
	if cached, ok := encodedCache.Load(key); ok {
		return cached.([]byte), nil
	}
	buf, err := generateEncoded(g.ctx, cx, cz)
	if err != nil {
		return nil, err
	}
	defer buf.free()
	raw := append([]byte(nil), buf.bytes()...)
	if encodedCacheCount.Add(1) > maxEncodedCacheEntries {
		encodedCache.Clear()
		encodedCacheCount.Store(1)
	}
	encodedCache.Store(key, raw)
	return raw, nil
}

// New returns a canonical Overworld ChunkGenerator for the given seed, sharing
// the underlying SteelMC context across callers with the same seed.
func New(seed int64) *Generator {
	sharedGenMu.Lock()
	if g, ok := sharedGens[seed]; ok && !g.closed.Load() {
		sharedGenMu.Unlock()
		return g
	}
	sharedGenMu.Unlock()

	g, err := openInternal(seed, Options{})
	if err != nil {
		panic(fmt.Sprintf("overworld.New(%d): %v", seed, err))
	}

	sharedGenMu.Lock()
	if existing, ok := sharedGens[seed]; ok && !existing.closed.Load() {
		sharedGenMu.Unlock()
		_ = g.Close()
		return existing
	}
	sharedGens[seed] = g
	sharedGenMu.Unlock()
	return g
}

// Open validates options, loads the embedded Geyser mappings, and starts the
// native SteelMC context.
func Open(seed int64, opts Options) (*Generator, error) {
	var toClose *Generator
	sharedGenMu.Lock()
	if existing, ok := sharedGens[seed]; ok && !existing.closed.Load() {
		workers := opts.Workers
		if workers == 0 {
			workers = defaultWorkers()
		}
		cacheChunks := opts.CacheChunks
		if cacheChunks == 0 {
			cacheChunks = defaultCacheChunks
		}
		if existing.opts.Deterministic != opts.Deterministic ||
			existing.opts.Workers != workers ||
			existing.opts.CacheChunks != cacheChunks {
			delete(sharedGens, seed)
			toClose = existing
		}
	}
	sharedGenMu.Unlock()
	if toClose != nil {
		_ = toClose.Close()
	}
	return openInternal(seed, opts)
}

func openInternal(seed int64, opts Options) (*Generator, error) {
	if opts.Registry == nil {
		dfworld.DefaultBlockRegistry.Finalize()
		opts.Registry = dfworld.DefaultBlockRegistry
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.Workers == 0 {
		opts.Workers = defaultWorkers()
	}
	if opts.CacheChunks == 0 {
		opts.CacheChunks = defaultCacheChunks
	}
	if opts.Workers < 1 || opts.Workers > maxWorkers {
		return nil, fmt.Errorf("workers must be 1..%d, got %d", maxWorkers, opts.Workers)
	}
	if opts.CacheChunks < 1 || opts.CacheChunks > maxCacheChunks {
		return nil, fmt.Errorf("cache chunks must be 1..%d, got %d", maxCacheChunks, opts.CacheChunks)
	}

	g := &Generator{
		logger: opts.Logger,
		opts:   opts,
		seed:   seed,
	}
	tr, err := newTranslator(opts.Registry, opts.Logger, &g.unmapped, &g.unmappedBiomes)
	if err != nil {
		return nil, err
	}
	ctx, err := openContext(seed, uint32(opts.Workers), uint32(opts.CacheChunks), opts.Deterministic)
	if err != nil {
		return nil, err
	}
	g.translator = tr
	g.ctx = ctx
	return g, nil
}

// Stats returns a point-in-time counter snapshot.
func (g *Generator) Stats() Stats {
	return Stats{
		Chunks:         g.chunks.Load(),
		Blocks:         g.blocks.Load(),
		FallbackBlocks: g.fallbackBlocks.Load(),
		UnmappedStates: g.unmapped.Load(),
		UnmappedBiomes: g.unmappedBiomes.Load(),
	}
}

// Generate implements livingworld/internal/world.ChunkGenerator.
// It generates the chunk at (cx, cz) using SteelMC and decodes the SWG1
// payload into LivingWorld's canonical *lwworld.Chunk representation.
func (g *Generator) Generate(cx, cz int) *lwworld.Chunk {
	c := lwworld.NewChunk()
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			c.SetHeightmap(x, z, int32(lwworld.MinWorldHeight-1))
		}
	}

	g.mu.Lock()
	if g.closed.Load() {
		g.mu.Unlock()
		return c
	}
	g.wg.Add(1)
	g.mu.Unlock()
	defer g.wg.Done()

	wire, err := g.generateWireBytes(int32(cx), int32(cz))
	if err != nil {
		g.logger.Error("SteelMC chunk generation failed", "cx", cx, "cz", cz, "err", err)
		return c
	}

	placed, err := g.FillCanonicalChunk(c, wire)
	if err != nil {
		g.logger.Error("SteelMC SWG1 decode failed", "cx", cx, "cz", cz, "err", err)
		return c
	}
	c.ClearDirty()
	g.chunks.Add(1)
	g.blocks.Add(placed)
	return c
}

// GenerateDragonfly populates one Dragonfly chunk, honouring context cancellation before and during generation.
func (g *Generator) GenerateDragonfly(ctx context.Context, pos dfworld.ChunkPos, c *dfchunk.Chunk) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if c == nil {
		return errors.New("nil chunk")
	}
	if r := c.Range(); r.Min() != overworldMinY || r.Max()-r.Min()+1 != overworldHeight {
		return fmt.Errorf("chunk range %v does not match overworld %d..%d", r, overworldMinY, overworldMinY+overworldHeight-1)
	}
	if abs64(int64(pos[0])) > chunkLimit || abs64(int64(pos[1])) > chunkLimit {
		return fmt.Errorf("chunk %v outside supported border", pos)
	}

	g.mu.Lock()
	if g.closed.Load() {
		g.mu.Unlock()
		return ErrClosed
	}
	g.wg.Add(1)
	g.mu.Unlock()

	start := time.Now()
	if ctx.Done() == nil {
		defer g.wg.Done()
		wire, err := g.generateWireBytes(pos[0], pos[1])
		if err != nil {
			return err
		}
		return g.finishDragonfly(pos, c, wire, start)
	}

	type result struct {
		wire []byte
		err  error
	}
	done := make(chan result, 1)
	go func() {
		defer g.wg.Done()
		wire, err := g.generateWireBytes(pos[0], pos[1])
		done <- result{wire: wire, err: err}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case res := <-done:
		if res.err != nil {
			return res.err
		}
		return g.finishDragonfly(pos, c, res.wire, start)
	}
}

// finishDragonfly decodes a completed native buffer and records generation metrics.
func (g *Generator) finishDragonfly(pos dfworld.ChunkPos, c *dfchunk.Chunk, wire []byte, start time.Time) error {
	placed, err := g.FillDragonflyChunk(c, wire)
	if err != nil {
		return err
	}
	chunks := g.chunks.Add(1)
	g.blocks.Add(placed)
	g.logger.Debug("generated chunk", "x", pos[0], "z", pos[1], "blocks", placed, "chunks", chunks, "took", time.Since(start))
	return nil
}

// Close waits for in-flight calls and releases the native context.
func (g *Generator) Close() error {
	g.mu.Lock()
	if g.closed.Swap(true) {
		g.mu.Unlock()
		return nil
	}
	g.mu.Unlock()
	g.wg.Wait()
	closeContext(g.ctx)
	g.ctx = nil

	sharedGenMu.Lock()
	if sharedGens[g.seed] == g {
		delete(sharedGens, g.seed)
	}
	sharedGenMu.Unlock()
	return nil
}

// wireReader bounds-checks little-endian reads over a byte slice.
type wireReader struct {
	data []byte
	pos  int
	err  error
}

func (r *wireReader) take(n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || r.pos+n > len(r.data) {
		r.err = ErrInvalidWire
		return nil
	}
	v := r.data[r.pos : r.pos+n]
	r.pos += n
	return v
}

func (r *wireReader) u16() uint16 {
	v := r.take(2)
	if v == nil {
		return 0
	}
	return binary.LittleEndian.Uint16(v)
}

func (r *wireReader) str() string {
	n := int(r.u16())
	v := r.take(n)
	if v == nil {
		return ""
	}
	return string(v)
}

func parseJavaState(s string) (string, map[string]string) {
	idx := strings.IndexByte(s, '[')
	if idx < 0 || !strings.HasSuffix(s, "]") {
		return s, nil
	}
	name := s[:idx]
	rawProps := s[idx+1 : len(s)-1]
	if rawProps == "" {
		return name, nil
	}
	parts := strings.Split(rawProps, ",")
	props := make(map[string]string, len(parts))
	for _, part := range parts {
		if eq := strings.IndexByte(part, '='); eq > 0 {
			props[part[:eq]] = part[eq+1:]
		}
	}
	return name, props
}

func (g *Generator) resolveCanonical(state, def string, flags byte) canonicalResolved {
	if cached, ok := g.canonicalStates.Load(state); ok {
		return cached.(canonicalResolved)
	}
	name, props := parseJavaState(state)
	if name == "minecraft:air" || name == "minecraft:cave_air" || name == "minecraft:void_air" {
		res := canonicalResolved{id: lwworld.AirID}
		g.canonicalStates.Store(state, res)
		return res
	}

	var id int32
	if len(props) > 0 {
		id = lwworld.StateIDWithProperties(name, props)
	} else {
		id = lwworld.StateID(name)
	}
	if id != lwworld.AirID {
		recordCanonicalBedrockState(id, state, def)
		res := canonicalResolved{id: id}
		g.canonicalStates.Store(state, res)
		return res
	}

	if def != "" && def != state {
		defName, defProps := parseJavaState(def)
		if len(defProps) > 0 {
			id = lwworld.StateIDWithProperties(defName, defProps)
		} else {
			id = lwworld.StateID(defName)
		}
		if id != lwworld.AirID {
			recordCanonicalBedrockState(id, state, def)
			res := canonicalResolved{id: id, fallback: true}
			g.canonicalStates.Store(state, res)
			return res
		}
	}

	id = lwworld.StateID(name)
	if id != lwworld.AirID {
		recordCanonicalBedrockState(id, state, def)
		res := canonicalResolved{id: id, fallback: true}
		g.canonicalStates.Store(state, res)
		return res
	}

	// Consult Geyser mapping table in case the Bedrock name matches a canonical state.
	if err := ensureMappingsLoaded(); err == nil {
		if bs, ok := loadedMappings.Blocks[state]; ok {
			if altID := lwworld.StateID(bs.Name); altID != lwworld.AirID {
				recordCanonicalBedrockState(altID, state, def)
				res := canonicalResolved{id: altID, fallback: true}
				g.canonicalStates.Store(state, res)
				return res
			}
		} else if bs, ok := baseNameMappings[name]; ok {
			if altID := lwworld.StateID(bs.Name); altID != lwworld.AirID {
				recordCanonicalBedrockState(altID, state, def)
				res := canonicalResolved{id: altID, fallback: true}
				g.canonicalStates.Store(state, res)
				return res
			}
		}
	}

	fallbackID := int32(lwworld.AirID)
	if flags&flagSolid != 0 {
		fallbackID = lwworld.StateID("minecraft:stone")
	} else if flags&flagWaterlogged != 0 {
		fallbackID = lwworld.WaterID
	}
	g.unmapped.Add(1)
	res := canonicalResolved{id: fallbackID, fallback: true, unmapped: true}
	g.canonicalStates.Store(state, res)
	return res
}

// FillCanonicalChunk decodes an SWG1 buffer into a canonical *lwworld.Chunk.
func (g *Generator) FillCanonicalChunk(c *lwworld.Chunk, data []byte) (uint64, error) {
	r := wireReader{data: data}
	header := r.take(8)
	if r.err != nil || string(header[:4]) != "SWG1" {
		return 0, ErrInvalidWire
	}
	minY := int(int16(binary.LittleEndian.Uint16(header[4:6])))
	sections := int(binary.LittleEndian.Uint16(header[6:8]))
	if minY != overworldMinY || sections != overworldSections {
		return 0, ErrInvalidWire
	}

	var heightmap [16][16]int32
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			heightmap[x][z] = int32(lwworld.MinWorldHeight - 1)
		}
	}

	var placed, fallbacks uint64
	for sec := 0; sec < sections; sec++ {
		count := int(r.u16())
		if r.err != nil || count == 0 || count > 4096 {
			return 0, ErrInvalidWire
		}
		palette := make([]canonicalResolved, count)
		allAir := true
		for i := range palette {
			state := r.str()
			def := r.str()
			flagBytes := r.take(1)
			if r.err != nil {
				return 0, r.err
			}
			res := g.resolveCanonical(state, def, flagBytes[0])
			palette[i] = res
			if res.id != lwworld.AirID {
				allAir = false
			}
		}

		indices := r.take(4096 * 2)
		if r.err != nil {
			return 0, r.err
		}
		baseY := minY + sec*16
		if count == 1 && allAir {
			for i := 0; i < len(indices); i += 2 {
				if binary.LittleEndian.Uint16(indices[i:i+2]) != 0 {
					return 0, ErrInvalidWire
				}
			}
		} else {
			for i := 0; i < 4096; i++ {
				index := int(binary.LittleEndian.Uint16(indices[i*2 : i*2+2]))
				if index >= len(palette) {
					return 0, ErrInvalidWire
				}
				entry := palette[index]
				if entry.fallback {
					fallbacks++
				}
				if entry.id != lwworld.AirID {
					x, dy, z := i%16, i/256, (i/16)%16
					y := baseY + dy
					c.SetBlock(x, y, z, lwworld.StateBlock{State: entry.id})
					placed++
					if int32(y) > heightmap[x][z] {
						heightmap[x][z] = int32(y)
					}
				}
			}
		}

		biomeCount := int(r.u16())
		if r.err != nil || biomeCount == 0 || biomeCount > 64 {
			return 0, ErrInvalidWire
		}
		biomePalette := make([]int32, biomeCount)
		for i := 0; i < biomeCount; i++ {
			name := r.str()
			if r.err != nil {
				return 0, r.err
			}
			biomePalette[i] = lwworld.BiomeID(name)
		}
		biomeIndices := r.take(64 * 2)
		if r.err != nil {
			return 0, r.err
		}
		var secBiomes [64]int32
		for i := 0; i < 64; i++ {
			index := int(binary.LittleEndian.Uint16(biomeIndices[i*2 : i*2+2]))
			if index >= biomeCount {
				return 0, ErrInvalidWire
			}
			secBiomes[i] = biomePalette[index]
		}
		c.SetSectionBiomes(sec, secBiomes)
	}
	if r.pos != len(r.data) {
		return 0, ErrInvalidWire
	}
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			c.SetHeightmap(x, z, heightmap[x][z])
		}
	}
	g.fallbackBlocks.Add(fallbacks)
	return placed, nil
}

// FillDragonflyChunk decodes an SWG1 buffer into a Dragonfly chunk.
func (g *Generator) FillDragonflyChunk(c *dfchunk.Chunk, data []byte) (uint64, error) {
	r := wireReader{data: data}
	header := r.take(8)
	if r.err != nil || string(header[:4]) != "SWG1" {
		return 0, ErrInvalidWire
	}
	minY := int(int16(binary.LittleEndian.Uint16(header[4:6])))
	sections := int(binary.LittleEndian.Uint16(header[6:8]))
	if minY != overworldMinY || sections != overworldSections || len(c.Sub()) != sections {
		return 0, ErrInvalidWire
	}

	t := g.translator
	subs := c.Sub()
	var placed, fallbacks uint64
	for sec := 0; sec < sections; sec++ {
		count := int(r.u16())
		if r.err != nil || count == 0 || count > 4096 {
			return 0, ErrInvalidWire
		}
		palette := make([]resolvedState, count)
		allAir := true
		for i := range palette {
			state := r.str()
			def := r.str()
			flags := r.take(1)
			if r.err != nil {
				return 0, r.err
			}
			resolved, fallback := t.translate(state, def, flags[0])
			palette[i] = resolved
			if resolved.id != t.air || resolved.waterlogged || fallback {
				allAir = false
			}
		}

		indices := r.take(4096 * 2)
		if r.err != nil {
			return 0, r.err
		}
		sub := subs[sec]
		if count == 1 && allAir {
			for i := 0; i < len(indices); i += 2 {
				if binary.LittleEndian.Uint16(indices[i:i+2]) != 0 {
					return 0, ErrInvalidWire
				}
			}
		} else {
			blocks := sub.Layer(0)
			var water *dfchunk.PalettedStorage
			for i := 0; i < 4096; i++ {
				index := int(binary.LittleEndian.Uint16(indices[i*2 : i*2+2]))
				if index >= len(palette) {
					return 0, ErrInvalidWire
				}
				state := palette[index]
				x, y, z := uint8(i%16), uint8(i/256), uint8((i/16)%16)
				if state.id != t.air {
					blocks.Set(x, y, z, state.id)
					placed++
				}
				if state.waterlogged {
					if water == nil {
						water = sub.Layer(1)
					}
					water.Set(x, y, z, t.water)
				}
				if state.fallback {
					fallbacks++
				}
			}
		}

		biomeCount := int(r.u16())
		if r.err != nil || biomeCount == 0 || biomeCount > 64 {
			return 0, ErrInvalidWire
		}
		biomes := make([]uint32, biomeCount)
		for i := range biomes {
			name := r.str()
			if r.err != nil {
				return 0, r.err
			}
			biomes[i], _ = t.translateBiome(name)
		}
		biomeIndices := r.take(64 * 2)
		if r.err != nil {
			return 0, r.err
		}
		baseY := int16(minY + sec*16)
		for i := 0; i < 64; i++ {
			index := int(binary.LittleEndian.Uint16(biomeIndices[i*2 : i*2+2]))
			if index >= len(biomes) {
				return 0, ErrInvalidWire
			}
			id := biomes[index]
			bx, by, bz := uint8((i%4)*4), baseY+int16((i/16)*4), uint8(((i/4)%4)*4)
			for dx := uint8(0); dx < 4; dx++ {
				for dy := int16(0); dy < 4; dy++ {
					for dz := uint8(0); dz < 4; dz++ {
						c.SetBiome(bx+dx, by+dy, bz+dz, id)
					}
				}
			}
		}
	}
	if r.pos != len(r.data) {
		return 0, ErrInvalidWire
	}
	g.fallbackBlocks.Add(fallbacks)
	c.Compact()
	return placed, nil
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
