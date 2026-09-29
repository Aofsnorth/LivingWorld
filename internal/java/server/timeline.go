package server

import (
	"bytes"
	"embed"
	"encoding/json"
	"math"

	"github.com/Tnze/go-mc/nbt"
)

//go:embed registrydata/timeline/*.json
var timelineFS embed.FS

// timelineNBT loads a bundled vanilla timeline element (extracted verbatim from
// 26.3-server.jar/data/minecraft/timeline) and encodes it as network NBT
// (nameless root) for use as registry entry data.
//
// We send the FULL data (hasData=true) instead of relying on the client's
// built-in "core" pack via SelectKnownPacks. The server cannot know the
// connecting client's patch version, so a single announced known-pack version
// can never match all of them; sending full data is version-independent and
// always accepted.
//
// The bundled copies must be re-extracted whenever the target Minecraft
// version moves: 26.3 changed minecraft:visual/cloud_color from a packed int
// (-1 / -15132378) to a hex string ("#ffffffff" / "#ff191926"). A stale copy
// fails the client's timeline decode, which leaves minecraft:timeline unbound
// and makes the dimension_type "timelines" reference unresolvable — the
// dimension_type then fails with "Failed to parse value ... for key
// minecraft:overworld from server".
func timelineNBT(element string) ([]byte, error) {
	raw, err := timelineFS.ReadFile("registrydata/timeline/" + element + ".json")
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	enc := nbt.NewEncoder(&buf)
	enc.NetworkFormat(true) // nameless root: [tagType][payload][TagEnd]
	if err := enc.Encode(nbtify(m), ""); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// nbtify maps decoded JSON onto Go values with explicit NBT types: whole numbers
// become int32 (TagInt), fractional become float64 (TagDouble). Numeric arrays
// are forced all-float so they encode as a homogeneous TagList (go-mc otherwise
// special-cases []int as TagIntArray). Mojang's NbtOps decodes numbers
// leniently, so int/double are interchangeable for the timeline codecs while
// strings/bools keep their natural tag.
func nbtify(v any) any {
	switch x := v.(type) {
	case map[string]any:
		m := make(map[string]any, len(x))
		for k, val := range x {
			m[k] = nbtify(val)
		}
		return m
	case []any:
		allNum := len(x) > 0
		for _, e := range x {
			if _, ok := e.(float64); !ok {
				allNum = false
				break
			}
		}
		out := make([]any, len(x))
		for i, e := range x {
			if allNum {
				out[i] = e // keep float64 → TagList of TagDouble
			} else {
				out[i] = nbtify(e)
			}
		}
		return out
	case float64:
		if x == math.Trunc(x) && !math.IsInf(x, 0) {
			return int32(x)
		}
		return x
	default:
		return v // bool, string, nil
	}
}
