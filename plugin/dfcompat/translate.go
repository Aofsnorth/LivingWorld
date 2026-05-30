package dfcompat

import (
	"github.com/df-mc/dragonfly/server/item"

	"livingworld/internal/registry"
)

// ItemBedrockRID resolves a dragonfly item stack to its Bedrock runtime id via
// the shared registry, keyed by the item's namespaced id. ok is false for empty
// stacks and unmapped items.
func ItemBedrockRID(reg *registry.Registry, s item.Stack) (int32, bool) {
	if s.Empty() {
		return 0, false
	}
	name, _ := s.Item().EncodeItem()
	return reg.ItemRuntime(name)
}

// BlockBedrockRID translates a canonical block state (= Java global state id) to
// its Bedrock runtime id via the shared registry.
func BlockBedrockRID(reg *registry.Registry, java registry.BlockState) (uint32, bool) {
	return reg.BlockToBedrock(java)
}
