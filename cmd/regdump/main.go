// Command regdump writes every entry LivingWorld sends to a Java client into
// <outdir>/<registry>/<key>.nbt, so the vanilla codecs can be pointed at the
// exact bytes we produce.
//
// It exists because a "Failed to parse value {...}" crash report does not say
// WHICH field the codec choked on, and the client only lists whole registries
// as failed. Decoding the same bytes with the real DimensionType.CODEC gives
// the exact field path.
//
// Usage:
//
//	regdump <outdir>
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"livingworld/internal/java/registry"

	"github.com/Tnze/go-mc/nbt"
	mcregistry "github.com/Tnze/go-mc/registry"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: regdump <outdir>")
		os.Exit(2)
	}
	out := os.Args[1]
	if err := os.MkdirAll(out, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	regs, sizes := registry.Build()

	names := make([]string, 0, len(sizes))
	for name := range sizes {
		names = append(names, name)
	}
	sort.Strings(names)

	total := 0
	for _, name := range names {
		r := regs.Registry(name)
		if r == nil {
			fmt.Fprintf(os.Stderr, "skip %s: not present in built registries\n", name)
			continue
		}
		typed, ok := r.(*mcregistry.Registry[nbt.RawMessage])
		if !ok {
			continue
		}
		dir := filepath.Join(out, sanitize(name))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		for i, key := range typed.Keys() {
			if key == "" {
				continue
			}
			val := typed.GetByID(int32(i))
			if val == nil {
				continue
			}
			path := filepath.Join(dir, sanitize(key)+".nbt")
			// NETWORK nbt has no root name: the root is a bare TAG_Compound
			// tag byte followed straight by the payload. Writing a name length
			// here would make the reader see an extra TAG_End and see nothing.
			full := append([]byte{0x0A}, val.Data...)
			if err := os.WriteFile(path, full, 0o644); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			total++
		}
		fmt.Printf("%-40s %d entries\n", name, sizes[name])
	}
	fmt.Printf("wrote %d NBT files to %s\n", total, out)
}

func sanitize(s string) string {
	return strings.NewReplacer(":", "_", "/", "_").Replace(s)
}
