package bedrocktests

import (
	dfworld "github.com/df-mc/dragonfly/server/world"
)

func dfworldRuntimeIDToState(rid uint32) (string, map[string]any, bool) {
	return dfworld.DefaultBlockRegistry.RuntimeIDToState(rid)
}
