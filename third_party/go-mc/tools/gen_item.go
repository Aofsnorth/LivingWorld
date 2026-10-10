// gen_item generates data/item/item.go from items.json + registries.json.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// --- JSON models ---

// itemsJSONData: map of "minecraft:<name>" -> { components: { ... } }
type itemsJSONData map[string]struct {
	Components itemComponents `json:"components"`
}

type itemComponents struct {
	MaxStackSize int      `json:"minecraft:max_stack_size"`
	ItemName     itemName `json:"minecraft:item_name"`
}

type itemName struct {
	Translate string `json:"translate"`
}

type itemEntry struct {
	VarName     string // Go identifier, e.g. "PolishedGranite"
	ID          int    // protocol_id from registries.json
	DisplayName string // Title Case, e.g. "Polished Granite"
	Name        string // short name, e.g. "polished_granite"
	StackSize   int    // max_stack_size, defaults to 64
}

func genItem(jsonDir, goMCRoot string) error {
	itemsPath := filepath.Join(jsonDir, "items.json")
	registriesPath := filepath.Join(jsonDir, "registries.json")
	outPath := filepath.Join(goMCRoot, "data", "item", "item.go")

	// --- Parse items.json ---
	var items itemsJSONData
	if err := readJSON(itemsPath, &items); err != nil {
		// Fallback for Minecraft 26.1 / 1.21.2+ default components directory
		componentsDir := filepath.Join(goMCRoot, "temp", "cache", filepath.Base(jsonDir)+"-datagen", "generated", "reports", "minecraft", "components", "item")
		files, readErr := os.ReadDir(componentsDir)
		if readErr != nil {
			return fmt.Errorf("genItem: failed to read items.json (%v) and fallback components dir (%v)", err, readErr)
		}
		items = make(itemsJSONData)
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
				continue
			}
			name := strings.TrimSuffix(f.Name(), ".json")
			fullName := "minecraft:" + name
			var fileData struct {
				Components itemComponents `json:"components"`
			}
			if err := readJSON(filepath.Join(componentsDir, f.Name()), &fileData); err != nil {
				return fmt.Errorf("genItem: failed to read fallback component %s: %w", f.Name(), err)
			}
			items[fullName] = fileData
		}
	}

	// --- Parse registries.json ---
	var regs registriesJSON
	if err := readJSON(registriesPath, &regs); err != nil {
		return fmt.Errorf("genItem: %w", err)
	}

	itemReg, ok := regs["minecraft:item"]
	if !ok {
		return fmt.Errorf("genItem: registry 'minecraft:item' not found in registries.json")
	}

	// --- Merge data ---
	var entries []itemEntry
	for fullName, regEntry := range itemReg.Entries {
		shortName := stripMinecraftPrefix(fullName)

		stackSize := 64 // default
		if itemData, ok := items[fullName]; ok {
			if itemData.Components.MaxStackSize > 0 {
				stackSize = itemData.Components.MaxStackSize
			}
		}

		entries = append(entries, itemEntry{
			VarName:     snakeToCamel(shortName),
			ID:          regEntry.ProtocolID,
			DisplayName: snakeToTitle(shortName),
			Name:        shortName,
			StackSize:   stackSize,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].ID < entries[j].ID
	})

	tmpl, err := loadTemplate(goMCRoot, "item.go.tmpl")
	if err != nil {
		return fmt.Errorf("genItem: %w", err)
	}

	data := struct {
		Header string
		Items  []itemEntry
	}{
		Header: generatedHeader("gen_item.go", "items.json", "registries.json"),
		Items:  entries,
	}

	out, err := executeTemplate(tmpl, data)
	if err != nil {
		return fmt.Errorf("genItem: %w", err)
	}

	if err := writeFile(outPath, out); err != nil {
		return fmt.Errorf("genItem: %w", err)
	}
	logf("genItem: wrote %s (%d items, %d bytes)", outPath, len(entries), len(out))
	return nil
}
