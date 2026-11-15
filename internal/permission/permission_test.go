package permission

import (
	"path/filepath"
	"testing"
)

func TestPermissionHierarchyAndWildcards(t *testing.T) {
	eng, err := NewEngine(DefaultDocument())
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	if !eng.Allowed("Alex", "livingworld.command.help", false, false) {
		t.Fatalf("expected default group to allow livingworld.command.help")
	}
	if eng.Allowed("Alex", "livingworld.command.gamemode", false, false) {
		t.Fatalf("expected default group to deny livingworld.command.gamemode")
	}
	if !eng.Allowed("Alex", "livingworld.command.gamemode", true, false) {
		t.Fatalf("expected operator to bypass permission check")
	}

	eng.AssignUserGroup("Steve", "moderator")
	if !eng.Allowed("Steve", "livingworld.command.gamemode", false, false) {
		t.Fatalf("expected moderator to allow gamemode")
	}
	if !eng.Allowed("Steve", "livingworld.command.tp", false, false) {
		t.Fatalf("expected moderator to inherit vip permission livingworld.command.tp")
	}
	if got := eng.Prefix("Steve"); got != "[Mod] " {
		t.Fatalf("expected prefix '[Mod] ', got %q", got)
	}

	eng.SetUserPermission("Steve", "livingworld.command.ban", false)
	if eng.Allowed("Steve", "livingworld.command.ban", false, true) {
		t.Fatalf("expected explicit user negation to deny livingworld.command.ban")
	}

	eng.SetUserPermission("Builder", "livingworld.build.*", true)
	eng.SetUserPermission("Builder", "livingworld.build.lava", false)
	if !eng.Allowed("Builder", "livingworld.build.place", false, false) {
		t.Fatalf("expected wildcard livingworld.build.* to allow livingworld.build.place")
	}
	if eng.Allowed("Builder", "livingworld.build.lava", false, true) {
		t.Fatalf("expected exact node negation to override wildcard")
	}

	tmpFile := filepath.Join(t.TempDir(), "permissions.json")
	if err := eng.SaveFile(tmpFile); err != nil {
		t.Fatalf("SaveFile failed: %v", err)
	}
	loaded, err := LoadOrCreateFile(tmpFile)
	if err != nil {
		t.Fatalf("LoadOrCreateFile failed: %v", err)
	}
	if !loaded.Allowed("Steve", "livingworld.command.gamemode", false, false) {
		t.Fatalf("expected loaded engine to preserve Steve's moderator group")
	}
}
