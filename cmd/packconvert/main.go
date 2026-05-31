// Command packconvert converts a vanilla resource pack between the Java and
// Bedrock editions (auto-detecting the source edition).
//
// Usage:
//
//	packconvert <sourcePack> <outDir>
//
// sourcePack may be a directory, .zip, .mcpack or .jar. The opposite-edition
// pack is written into outDir. v1 converts structure, manifest, icon and
// textures; models, sounds, languages and Bedrock addons are not yet converted
// (see package resourcepack docs).
package main

import (
	"fmt"
	"os"

	"livingworld/internal/resourcepack"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: packconvert <sourcePack(.zip/.mcpack/.jar/dir)> <outDir>")
		os.Exit(2)
	}
	rep, err := resourcepack.Convert(os.Args[1], os.Args[2])
	if err != nil {
		fmt.Fprintf(os.Stderr, "packconvert: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("done: %s -> %s pack\n  copied %d file(s), skipped %d (not converted by v1)\n  out: %s\n",
		rep.Source, rep.Target, rep.Copied, rep.Skipped, rep.OutDir)
}
