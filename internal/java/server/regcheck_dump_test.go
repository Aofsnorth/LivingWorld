package server

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	pk "github.com/Tnze/go-mc/net/packet"
)

// TestDumpConfigStream writes the exact packet payload bytes the configuration
// phase sends (known-packs announcement, every registry-data packet in order,
// the update-tags packet) so tools/java/ClientSim.java can replay them through
// the REAL 26.3 client registry-loading code. Skipped unless REGCHECK_DIR is
// set.
//
//	REGCHECK_DIR=/tmp/x go test ./internal/java/server/ -run DumpConfigStream -v
func TestDumpConfigStream(t *testing.T) {
	dir := os.Getenv("REGCHECK_DIR")
	if dir == "" {
		t.Skip("set REGCHECK_DIR to dump the configuration stream for ClientSim")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	// SelectKnownPacks payload (announcement), as built in AcceptConfig.
	{
		var buf bytes.Buffer
		_, _ = pk.VarInt(1).WriteTo(&buf)
		_, _ = pk.String("minecraft").WriteTo(&buf)
		_, _ = pk.String("core").WriteTo(&buf)
		_, _ = pk.String("26.3").WriteTo(&buf)
		if err := os.WriteFile(filepath.Join(dir, "knownpacks.bin"), buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// One payload file per registry packet, plus a manifest with send order.
	var manifest bytes.Buffer
	for i, reg := range vanillaSyncRegistries {
		p := vanillaRegistryDataPacket(reg)
		name := fmt.Sprintf("regdata_%02d.bin", i)
		if err := os.WriteFile(filepath.Join(dir, name), p.Data, 0o644); err != nil {
			t.Fatal(err)
		}
		manifest.WriteString(name)
		manifest.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(dir, "regdata_manifest.txt"), manifest.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "updatetags.bin"), buildVanillaUpdateTagsPayload(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestDumpTimelineNBT writes the exact timeline/world_clock NBT the
// configuration phase sends so tools/java/RegCheck.java can run it through the
// real 26.3 codecs. It is skipped unless REGCHECK_DIR is set, so it does not
// litter the repo during a normal test run.
//
//	REGCHECK_DIR=/tmp/x go test ./internal/java/server/ -run DumpTimeline -v
func TestDumpTimelineNBT(t *testing.T) {
	dir := os.Getenv("REGCHECK_DIR")
	if dir == "" {
		t.Skip("set REGCHECK_DIR to dump timeline NBT for RegCheck")
	}
	for _, sub := range []string{"minecraft_timeline", "minecraft_world_clock"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	for _, name := range []string{"day", "moon", "early_game", "villager_schedule"} {
		b, err := timelineNBT(name)
		if err != nil {
			t.Fatalf("timelineNBT(%s): %v", name, err)
		}
		// A nameless-root TAG_Compound is exactly 0x0A + payload; the payload
		// already ends with TAG_End, which is what a reader expects.
		if b[0] != 0x0A {
			t.Errorf("%s root tag 0x%02X, want 0x0A", name, b[0])
		}
		path := filepath.Join(dir, "minecraft_timeline", "minecraft_"+name+".nbt")
		if err := os.WriteFile(path, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// The world_clock entry is an empty compound, written by hand in config.go
	// exactly like this one.
	empty := []byte{0x0a, 0x00}
	if err := os.WriteFile(filepath.Join(dir, "minecraft_world_clock", "minecraft_overworld.nbt"), empty, 0o644); err != nil {
		t.Fatal(err)
	}
}
