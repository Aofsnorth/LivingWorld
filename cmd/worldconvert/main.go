// Command worldconvert converts Minecraft worlds between vanilla formats and the
// LivingWorld format.
//
// Usage:
//
//	worldconvert import-java   <vanillaJavaWorldDir> <livingWorldDir>
//	worldconvert export-java   <livingWorldDir>      <vanillaJavaWorldDir>
//	worldconvert import-bedrock <vanillaBedrockWorldDir> <livingWorldDir>
//	worldconvert export-bedrock <livingWorldDir>        <vanillaBedrockWorldDir>
//
// "import" brings a vanilla world into this server; "export" writes a vanilla
// world out from a LivingWorld save. The source directory is never modified.
package main

import (
	"fmt"
	"os"

	"livingworld/internal/worldconvert"
)

func main() {
	if len(os.Args) != 4 {
		usage()
		os.Exit(2)
	}
	mode, src, dst := os.Args[1], os.Args[2], os.Args[3]

	var (
		st  worldconvert.Stats
		err error
	)
	switch mode {
	case "import-java":
		st, err = worldconvert.ImportJavaWorld(src, dst)
	case "export-java":
		st, err = worldconvert.ExportJavaWorld(src, dst)
	case "import-bedrock":
		st, err = worldconvert.ImportBedrockWorld(src, dst)
	case "export-bedrock":
		st, err = worldconvert.ExportBedrockWorld(src, dst)
	default:
		fmt.Fprintf(os.Stderr, "unknown mode %q\n\n", mode)
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "worldconvert: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("done: %d chunks, %d sections (%s)\n  %s -> %s\n", st.Chunks, st.Sections, mode, src, dst)
}

func usage() {
	fmt.Fprint(os.Stderr, `worldconvert — convert Minecraft worlds to/from LivingWorld

  worldconvert import-java    <vanillaJavaWorldDir>    <livingWorldDir>
  worldconvert export-java    <livingWorldDir>         <vanillaJavaWorldDir>
  worldconvert import-bedrock <vanillaBedrockWorldDir> <livingWorldDir>
  worldconvert export-bedrock <livingWorldDir>         <vanillaBedrockWorldDir>

Block types are preserved via the shared global block-state palette; block-state
properties default, and lighting/biomes/entities are not transferred (Java).
`)
}
