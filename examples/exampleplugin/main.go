// Command exampleplugin is a complete, runnable example of embedding LivingWorld
// as a library and writing plugin behaviour using only the public API.
//
// Run it just like the bundled server:
//
//	go run ./examples/exampleplugin
package main

import (
	"log"
	"strings"

	"livingworld/plugin"
	"livingworld/server"
)

// greeterPlugin is a minimal plugin implementing the plugin.Plugin interface.
type greeterPlugin struct {
	host plugin.Host
}

func (g *greeterPlugin) Name() string    { return "greeter" }
func (g *greeterPlugin) Version() string { return "1.0.0" }

func (g *greeterPlugin) OnEnable(host plugin.Host) error {
	g.host = host
	host.Log("[greeter] enabled")
	return nil
}

func (g *greeterPlugin) OnDisable() error { return nil }

func main() {
	srv := server.New(server.DefaultConfig())
	pm := srv.Plugins()

	// 1) The easy path: typed event handlers, no plugin struct required.
	pm.OnPlayerJoin(func(e *plugin.PlayerJoinEvent) {
		srv.Broadcast("§e" + e.PlayerName + " joined the world!")
	})

	// 2) A cancellable event: protect bedrock from being broken.
	bedrock := srv.StateID("minecraft:bedrock")
	pm.OnBlockBreak(func(e *plugin.BlockBreakEvent) {
		if e.BlockID == bedrock {
			srv.Message(e.PlayerName, "§cYou can't break bedrock.")
			e.Cancel()
		}
	})

	// 3) A simple chat command handled by cancelling the chat event.
	pm.OnPlayerChat(func(e *plugin.PlayerChatEvent) {
		if strings.HasPrefix(e.Message, "!players") {
			srv.Message(e.PlayerName, "Online: "+strings.Join(srv.Players(), ", "))
			e.Cancel() // don't echo the command to chat
		}
	})

	// 4) A full plugin object with lifecycle hooks.
	if err := pm.Register(&greeterPlugin{}); err != nil {
		log.Fatalf("failed to register plugin: %v", err)
	}

	if err := srv.Run(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
