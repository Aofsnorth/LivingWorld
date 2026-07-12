package world

import (
	"fmt"
	"os"
	"path/filepath"
)

// EnablePersistence attaches storage to every world. The default logical world
// keeps the name "world" for API compatibility but persists under
// <baseDir>/dimensions, matching the dimension-oriented on-disk layout.
func (m *Manager) EnablePersistence(baseDir string) error {
	for _, w := range m.GetAllWorlds() {
		storageName := w.Name()
		if w == m.defaultWorld && storageName == legacyDefaultWorldStorageName {
			storageName = defaultDimensionStorageName
			if err := migrateLegacyDefaultWorldStorage(baseDir); err != nil {
				return err
			}
		}
		store, err := NewRegionStorage(filepath.Join(baseDir, storageName))
		if err != nil {
			return err
		}
		w.SetStorage(store)
	}
	return nil
}

func migrateLegacyDefaultWorldStorage(baseDir string) error {
	legacyDir := filepath.Join(baseDir, legacyDefaultWorldStorageName)
	dimensionDir := filepath.Join(baseDir, defaultDimensionStorageName)
	if _, err := os.Stat(dimensionDir); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat dimension storage %s: %w", dimensionDir, err)
	}
	info, err := os.Stat(legacyDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat legacy world storage %s: %w", legacyDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("legacy world storage %s is not a directory", legacyDir)
	}
	if err := os.Rename(legacyDir, dimensionDir); err != nil {
		return fmt.Errorf("rename legacy world storage %s -> %s: %w", legacyDir, dimensionDir, err)
	}
	return nil
}
