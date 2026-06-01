package main

import (
	"flag"
	"log"
	"os"

	"livingworld/internal/infrastructure/logging"
	"livingworld/server"

	"github.com/mattn/go-isatty"
)

var (
	Version   = "0.0.1"
	BuildDate = "2026-05-27"
)

// main is a thin entry point over the public livingworld/server API. Everything
// it does can be done from your own program by importing "livingworld/server".
func main() {
	logging.Setup()
	log.Printf("LivingWorld Server v%s (build: %s)", Version, BuildDate)

	configPath := flag.String("config", "config/config.yml", "path to YAML config")
	noTUI := flag.Bool("no-tui", false, "disable the terminal UI and use a plain console")
	debugChunks := flag.Bool("debug-chunks", false, "log chunk streaming (boundary crossings, forget/send lists) for diagnosing chunk render bugs")
	flag.Parse()

	cfg, err := server.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	cfg.Java.DebugChunks = *debugChunks

	srv := server.New(cfg)
	// Use the TUI by default on an interactive terminal; fall back to the plain
	// console when --no-tui is set or stdout is redirected (e.g. piped to a file).
	if !*noTUI && isatty.IsTerminal(os.Stdout.Fd()) {
		err = srv.RunTUI()
	} else {
		err = srv.Run()
	}
	if err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
