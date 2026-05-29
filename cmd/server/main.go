package main

import (
	"flag"
	"log"

	"livingworld/server"
)

var (
	Version   = "0.0.1"
	BuildDate = "2026-05-27"
)

// main is a thin entry point over the public livingworld/server API. Everything
// it does can be done from your own program by importing "livingworld/server".
func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Printf("LivingWorld Server v%s (build: %s)", Version, BuildDate)

	configPath := flag.String("config", "config/config.yml", "path to YAML config")
	flag.Parse()

	cfg, err := server.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	srv := server.New(cfg)
	if err := srv.Run(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
