package bedrocktests

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"

	_ "livingworld/internal/bedrock/inventory" // embed init: must parse the asset
)

// TestVanillaItemsAssetMatchesDragonfly pins the embedded Bedrock item
// registry to the dragonfly version we link against. The stale v0.10.13 copy
// shipped item runtime IDs of an older protocol and desynced inventory state
// on 1.26.50 clients. When bumping dragonfly, re-copy
// server/world/vanilla_items.nbt into internal/bedrock/data/ and update this
// hash (and the comment in internal/bedrock/inventory/items.go).
func TestVanillaItemsAssetMatchesDragonfly(t *testing.T) {
	const want = dragonflyVanillaItemsSHA256 // from dragonfly v0.11.5
	got, err := embeddedVanillaItemsSHA()
	if err != nil {
		t.Fatalf("read embedded asset: %v", err)
	}
	if got != want {
		t.Fatalf("internal/bedrock/data/vanilla_items.nbt is out of sync with dragonfly@v0.11.5\n got sha256 %s\nwant sha256 %s", got, want)
	}
}

func embeddedVanillaItemsSHA() (string, error) {
	// The asset is embedded into the inventory package; to hash the exact
	// bytes the embed carries, read the same file from the repo. go test runs
	// with the package dir (tests/bedrock) as CWD, so walk up two levels.
	data, err := os.ReadFile("../../internal/bedrock/data/vanilla_items.nbt")
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
