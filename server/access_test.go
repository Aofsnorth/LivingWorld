package server

import (
	"path/filepath"
	"testing"
)

func TestOpsListPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ops.txt")
	ops, err := LoadOps(path)
	if err != nil {
		t.Fatal(err)
	}
	if added, err := ops.Add("Notch"); err != nil || !added {
		t.Fatalf("Add Notch = %v, %v", added, err)
	}
	if added, _ := ops.Add("notch"); added {
		t.Fatal("case-insensitive duplicate Add should report false")
	}
	if !ops.Has("NOTCH") {
		t.Fatal("Has should be case-insensitive")
	}

	reloaded, err := LoadOps(path) // re-read from disk
	if err != nil {
		t.Fatal(err)
	}
	if !reloaded.Has("notch") {
		t.Fatal("ops were not persisted")
	}
	if removed, err := reloaded.Remove("Notch"); err != nil || !removed {
		t.Fatalf("Remove = %v, %v", removed, err)
	}
	if again, _ := LoadOps(path); again.Has("notch") {
		t.Fatal("removal was not persisted")
	}
}

func TestWhitelistEnforcement(t *testing.T) {
	wl := newWhitelist()
	if !wl.Allowed("anyone") {
		t.Fatal("a disabled whitelist must allow everyone")
	}
	wl.SetEnabled(true)
	if wl.Allowed("stranger") {
		t.Fatal("an enabled whitelist must reject unlisted players")
	}
	if _, err := wl.Add("Friend"); err != nil {
		t.Fatal(err)
	}
	if !wl.Allowed("friend") {
		t.Fatal("a listed player must be allowed (case-insensitive)")
	}
}
