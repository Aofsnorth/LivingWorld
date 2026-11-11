package overworld

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"

	_ "github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/df-mc/dragonfly/server/world/biome"
)

const (
	flagSolid       = 1 << 0
	flagWaterlogged = 1 << 1
)

//go:embed mappings/table.json.gz
var encodedMappings []byte

type mappingTable struct {
	Blocks map[string]bedrockState `json:"blocks"`
	Biomes map[string]uint32       `json:"biomes"`
}

type bedrockState struct {
	Name       string         `json:"name"`
	Properties map[string]any `json:"properties"`
	States     map[string]any `json:"states"`
}

var (
	loadMappingsOnce sync.Once
	loadedMappings   mappingTable
	loadMappingsErr  error
	baseNameMappings map[string]bedrockState
	canonicalBedrock sync.Map // map[int32]bedrockState
)

func ensureMappingsLoaded() error {
	loadMappingsOnce.Do(func() {
		zr, err := gzip.NewReader(bytes.NewReader(encodedMappings))
		if err != nil {
			loadMappingsErr = fmt.Errorf("open mappings: %w", err)
			return
		}
		defer zr.Close()
		if err := json.NewDecoder(zr).Decode(&loadedMappings); err != nil {
			loadMappingsErr = fmt.Errorf("decode mappings: %w", err)
			return
		}
		baseNameMappings = make(map[string]bedrockState, len(loadedMappings.Blocks))
		for k, v := range loadedMappings.Blocks {
			if len(v.Properties) == 0 && len(v.States) > 0 {
				v.Properties = v.States
				v.States = nil
				loadedMappings.Blocks[k] = v
			}
			if idx := strings.IndexByte(k, '['); idx < 0 {
				baseNameMappings[k] = v
			} else {
				base := k[:idx]
				if _, exists := baseNameMappings[base]; !exists || strings.Contains(k, "half=lower") || strings.Contains(k, "axis=y") {
					baseNameMappings[base] = v
				}
			}
		}
	})
	return loadMappingsErr
}

// LookupBedrockBiomeID translates a Java biome resource name (e.g. "minecraft:plains")
// into its Dragonfly/Bedrock biome ID, falling back to plains if unknown.
func LookupBedrockBiomeID(javaBiomeName string) uint32 {
	if err := ensureMappingsLoaded(); err == nil {
		if id, ok := loadedMappings.Biomes[javaBiomeName]; ok {
			return id
		}
	}
	return uint32(biome.Plains{}.EncodeBiome())
}

// RecordCanonicalBedrockState associates a canonical LivingWorld block state ID
// with its corresponding Bedrock state from the Geyser mapping table.
func recordCanonicalBedrockState(id int32, javaState, javaDef string) {
	if id == 0 {
		return
	}
	if _, ok := canonicalBedrock.Load(id); ok {
		return
	}
	if err := ensureMappingsLoaded(); err != nil {
		return
	}
	if bs, ok := loadedMappings.Blocks[javaState]; ok {
		canonicalBedrock.LoadOrStore(id, bs)
		return
	}
	if javaDef != "" {
		if bs, ok := loadedMappings.Blocks[javaDef]; ok {
			canonicalBedrock.LoadOrStore(id, bs)
			return
		}
	}
	base := javaState
	if idx := strings.IndexByte(base, '['); idx >= 0 {
		base = base[:idx]
	}
	if bs, ok := baseNameMappings[base]; ok {
		canonicalBedrock.LoadOrStore(id, bs)
	}
}

var (
	sharedTranslatorOnce sync.Once
	sharedTranslator     *translator
)

func getSharedTranslator() *translator {
	sharedTranslatorOnce.Do(func() {
		world.DefaultBlockRegistry.Finalize()
		var unmapped, unmappedBiomes atomic.Uint64
		t, err := newTranslator(world.DefaultBlockRegistry, slog.Default(), &unmapped, &unmappedBiomes)
		if err == nil {
			sharedTranslator = t
		}
	})
	return sharedTranslator
}

// LookupBedrockRID resolves a canonical LivingWorld block state ID (or Java
// block name/state string) into a Dragonfly Bedrock runtime ID using the
// embedded Geyser 26.3 mapping table.
func LookupBedrockRID(id int32, javaNameOrState string) (uint32, bool) {
	t := getSharedTranslator()
	if t == nil {
		return 0, false
	}
	if id != 0 {
		if v, ok := canonicalBedrock.Load(id); ok {
			if rid, found := t.lookup(v.(bedrockState)); found {
				return rid, true
			}
		}
	}
	if javaNameOrState == "" {
		return 0, false
	}
	if bs, ok := t.blocks[javaNameOrState]; ok {
		if rid, found := t.lookup(bs); found {
			return rid, true
		}
	}
	base := javaNameOrState
	if idx := strings.IndexByte(base, '['); idx >= 0 {
		base = base[:idx]
	}
	if bs, ok := baseNameMappings[base]; ok {
		if rid, found := t.lookup(bs); found {
			return rid, true
		}
	}
	return 0, false
}

// resolvedState is the cached Dragonfly representation of one Java palette entry.
type resolvedState struct {
	id          uint32
	waterlogged bool
	fallback    bool
}

// translator converts SteelMC's Java block and biome names into Dragonfly runtime IDs.
type translator struct {
	registry                 world.BlockRegistry
	logger                   *slog.Logger
	blocks                   map[string]bedrockState
	biomes                   map[string]uint32
	propTypes                map[string]map[string]any
	states                   sync.Map
	logged                   sync.Map
	plains                   uint32
	air, stone, water        uint32
	unmapped, unmappedBiomes *atomic.Uint64
}

// newTranslator learns the NBT property types from Dragonfly's finalized block registry,
// then loads the generated Java-to-Bedrock table.
func newTranslator(registry world.BlockRegistry, logger *slog.Logger, unmapped, unmappedBiomes *atomic.Uint64) (*translator, error) {
	if err := ensureMappingsLoaded(); err != nil {
		return nil, err
	}

	propTypes := make(map[string]map[string]any)
	for id := uint32(0); ; id++ {
		name, props, ok := registry.RuntimeIDToState(id)
		if !ok {
			break
		}
		if len(props) == 0 {
			continue
		}
		types := propTypes[name]
		if types == nil {
			types = make(map[string]any, len(props))
			propTypes[name] = types
		}
		for k, v := range props {
			types[k] = v
		}
	}

	t := &translator{
		registry:       registry,
		logger:         logger,
		blocks:         loadedMappings.Blocks,
		biomes:         loadedMappings.Biomes,
		propTypes:      propTypes,
		plains:         uint32(biome.Plains{}.EncodeBiome()),
		unmapped:       unmapped,
		unmappedBiomes: unmappedBiomes,
	}
	var ok bool
	if t.air, ok = t.lookup(t.blocks["minecraft:air"]); !ok {
		return nil, fmt.Errorf("missing minecraft:air runtime ID")
	}
	if t.stone, ok = t.lookup(t.blocks["minecraft:stone"]); !ok {
		return nil, fmt.Errorf("missing minecraft:stone runtime ID")
	}
	if t.water, ok = t.lookup(t.blocks["minecraft:water[level=0]"]); !ok {
		return nil, fmt.Errorf("missing minecraft:water runtime ID")
	}
	return t, nil
}

// lookup coerces JSON numbers and booleans to the exact integer types Dragonfly hashed.
func (t *translator) lookup(state bedrockState) (uint32, bool) {
	if len(state.Properties) == 0 && state.Name != "minecraft:vine" {
		return t.registry.StateToRuntimeID(state.Name, nil)
	}
	types := t.propTypes[state.Name]
	props := make(map[string]any, len(state.Properties)+1)
	for k, raw := range state.Properties {
		switch types[k].(type) {
		case bool:
			switch v := raw.(type) {
			case bool:
				props[k] = v
			case float64:
				props[k] = v != 0
			default:
				props[k] = raw
			}
		case uint8:
			switch v := raw.(type) {
			case bool:
				if v {
					props[k] = uint8(1)
				} else {
					props[k] = uint8(0)
				}
			case float64:
				props[k] = uint8(v)
			default:
				props[k] = raw
			}
		case int32:
			if v, ok := raw.(float64); ok {
				props[k] = int32(v)
			} else {
				props[k] = raw
			}
		default:
			props[k] = raw
		}
	}
	// Bedrock renders minecraft:vine with vine_direction_bits=0 as 0 faces
	// (completely invisible). Geyser maps canopy vines (up=true, all horizontal
	// faces false) to vine_direction_bits=0; force 15 (all 4 horizontal faces)
	// using the exact integer type Dragonfly registered for vine_direction_bits.
	if state.Name == "minecraft:vine" {
		switch types["vine_direction_bits"].(type) {
		case uint8:
			if v, ok := props["vine_direction_bits"].(uint8); !ok || v == 0 {
				props["vine_direction_bits"] = uint8(15)
			}
		default:
			if v, ok := props["vine_direction_bits"].(int32); !ok || v == 0 {
				props["vine_direction_bits"] = int32(15)
			}
		}
	}
	if rid, ok := t.registry.StateToRuntimeID(state.Name, props); ok {
		return rid, true
	}
	return t.registry.StateToRuntimeID(state.Name, nil)
}

// translate caches one lookup per Java palette state and logs each fallback once.
func (t *translator) translate(state, def string, flags byte) (resolvedState, bool) {
	if cached, ok := t.states.Load(state); ok {
		res := cached.(resolvedState)
		return res, res.fallback
	}
	waterlogged := flags&flagWaterlogged != 0 || isInherentlyWaterloggedJavaState(state)
	if mapped, ok := t.blocks[state]; ok {
		if id, ok := t.lookup(mapped); ok {
			res := resolvedState{id: id, waterlogged: waterlogged && id != t.water}
			t.states.Store(state, res)
			return res, false
		}
	}
	if def != "" && def != state {
		if mapped, ok := t.blocks[def]; ok {
			if id, ok := t.lookup(mapped); ok {
				t.logOnce("block:"+state, "using default block state", "java", state, "default", def)
				res := resolvedState{id: id, waterlogged: waterlogged && id != t.water, fallback: true}
				t.states.Store(state, res)
				return res, true
			}
		}
	}
	fallback := t.air
	if flags&flagSolid != 0 {
		fallback = t.stone
	} else if waterlogged {
		fallback = t.water
		waterlogged = false
	}
	t.unmapped.Add(1)
	t.logOnce("block:"+state, "unmapped block state", "java", state)
	res := resolvedState{id: fallback, waterlogged: waterlogged, fallback: true}
	t.states.Store(state, res)
	return res, true
}

// isInherentlyWaterloggedJavaState reports whether a Java block state is an
// underwater plant/column that does not carry a `[waterlogged=true]` property
// in Java Edition, but requires water on subchunk Layer 1 in Bedrock Edition
// so Bedrock does not render an air pocket around it.
func isInherentlyWaterloggedJavaState(state string) bool {
	name := state
	if idx := strings.IndexByte(state, '['); idx >= 0 {
		name = state[:idx]
	}
	switch name {
	case "minecraft:seagrass",
		"minecraft:tall_seagrass",
		"minecraft:kelp",
		"minecraft:kelp_plant",
		"minecraft:bubble_column":
		return true
	default:
		return strings.Contains(state, "waterlogged=true")
	}
}

// translateBiome resolves a Java biome resource name or falls back to plains.
func (t *translator) translateBiome(name string) (uint32, bool) {
	if id, ok := t.biomes[name]; ok {
		if _, ok := world.BiomeByID(int(id)); ok {
			return id, false
		}
	}
	t.unmappedBiomes.Add(1)
	t.logOnce("biome:"+name, "unmapped biome", "java", name)
	return t.plains, true
}

func (t *translator) logOnce(key, msg string, args ...any) {
	if _, loaded := t.logged.LoadOrStore(key, struct{}{}); !loaded {
		t.logger.Warn(msg, args...)
	}
}
