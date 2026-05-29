// Package item is LivingWorld's item registry. It wraps the full vanilla 26.1
// item table shipped by go-mc and adds name-based lookup plus item->block
// linkage so held items can be placed as blocks.
package item

import (
	"strings"

	"livingworld/internal/world"

	gmitem "github.com/Tnze/go-mc/data/item"
)

// Item describes a registered item. It mirrors go-mc's item record.
type Item struct {
	ID          int32
	Name        string // namespaced, e.g. "minecraft:stone"
	DisplayName string
	StackSize   int
}

var (
	byID   = map[int32]Item{}
	byName = map[string]Item{}
	all    []Item
)

func init() {
	for id, it := range gmitem.ByID {
		name := "minecraft:" + it.Name
		entry := Item{
			ID:          int32(id),
			Name:        name,
			DisplayName: it.DisplayName,
			StackSize:   int(it.StackSize),
		}
		byID[entry.ID] = entry
		byName[name] = entry
		all = append(all, entry)
	}
}

// normalize ensures a namespaced name ("stone" -> "minecraft:stone").
func normalize(name string) string {
	if !strings.Contains(name, ":") {
		return "minecraft:" + name
	}
	return name
}

// ByID returns the item with the given protocol ID.
func ByID(id int32) (Item, bool) {
	it, ok := byID[id]
	return it, ok
}

// ByName returns the item with the given name. Accepts "stone" or
// "minecraft:stone".
func ByName(name string) (Item, bool) {
	it, ok := byName[normalize(name)]
	return it, ok
}

// All returns every registered item (unspecified order).
func All() []Item {
	out := make([]Item, len(all))
	copy(out, all)
	return out
}

// Count returns the number of registered items.
func Count() int { return len(all) }

// BlockStateID returns the default block-state ID an item places when used, and
// ok=true if the item corresponds to a placeable block. Non-block items (tools,
// food, ...) return ok=false.
func BlockStateID(name string) (int32, bool) {
	n := normalize(name)
	if n == "minecraft:air" {
		return world.AirID, false
	}
	id := world.StateID(n)
	if id == world.AirID {
		return world.AirID, false
	}
	return id, true
}
