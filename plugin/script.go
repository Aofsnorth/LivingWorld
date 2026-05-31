package plugin

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/dop251/goja"
)

// ScriptPlugin is a JavaScript plugin loaded from a .js file.
//
// Running foreign-ecosystem plugins (Bukkit/Paper JARs on the JVM, PocketMine
// PHP) inside a Go process is not feasible — each needs its own language runtime
// and full server API. Instead LivingWorld exposes its OWN scriptable plugin
// surface: drop a .js file in the plugins/ directory and it receives a `server`
// global with the whole Host API (broadcast, setBlock, …) plus event hooks
// (server.onPlayerJoin(fn), server.onBlockBreak(fn), …). Event objects expose
// their fields in camelCase and cancellable ones a cancel() method, e.g.
//
//	server.onPlayerChat(function (e) {
//	    if (e.message.indexOf("badword") >= 0) { e.cancel(); }
//	});
//
// Each script runs on its own goja runtime guarded by mu, because events are
// emitted from many server goroutines and a goja.Runtime is single-threaded.
type ScriptPlugin struct {
	name string
	vm   *goja.Runtime
	mu   sync.Mutex
}

func (sp *ScriptPlugin) call(fn goja.Callable, e Event) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	if _, err := fn(goja.Undefined(), sp.vm.ToValue(e)); err != nil {
		log.Printf("[Plugin] %s: handler error: %v", sp.name, err)
	}
}

// LoadScripts loads every *.js file in dir as a JavaScript plugin and registers
// its event handlers. A missing directory is not an error. SetHost must have
// been called first. Returns the number of scripts loaded.
func (m *PluginManager) LoadScripts(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	host := m.Host()
	if host == nil {
		return 0, fmt.Errorf("LoadScripts: no Host set")
	}
	loaded := 0
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".js") {
			continue
		}
		if err := m.loadScript(filepath.Join(dir, e.Name()), host); err != nil {
			log.Printf("[Plugin] script %s failed to load: %v", e.Name(), err)
			continue
		}
		loaded++
	}
	return loaded, nil
}

func (m *PluginManager) loadScript(path string, host Host) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	sp := &ScriptPlugin{
		name: strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
		vm:   goja.New(),
	}
	sp.vm.SetFieldNameMapper(goja.UncapFieldNameMapper()) // JS sees e.playerName, e.cancel()

	server := sp.vm.NewObject()
	_ = server.Set("broadcast", host.Broadcast)
	_ = server.Set("message", host.Message)
	_ = server.Set("players", host.Players)
	_ = server.Set("playerCount", host.PlayerCount)
	_ = server.Set("getBlock", host.GetBlock)
	_ = server.Set("setBlock", host.SetBlock)
	_ = server.Set("stateID", host.StateID)
	_ = server.Set("log", func(msg string) { host.Log("[%s] %s", sp.name, msg) })

	on := func(jsName string, t EventType) {
		_ = server.Set(jsName, func(fn goja.Value) {
			cb, ok := goja.AssertFunction(fn)
			if !ok {
				panic(sp.vm.ToValue(jsName + ": argument must be a function"))
			}
			m.On(t, func(e Event) { sp.call(cb, e) })
		})
	}
	on("onPlayerJoin", EventPlayerJoin)
	on("onPlayerLeave", EventPlayerLeave)
	on("onPlayerChat", EventPlayerChat)
	on("onPlayerCommand", EventPlayerCommand)
	on("onPlayerInteract", EventPlayerInteract)
	on("onBlockBreak", EventBlockBreak)
	on("onBlockPlace", EventBlockPlace)
	on("onPlayerAttack", EventPlayerAttack)
	on("onEntityDamage", EventEntityDamage)
	on("onEntityDeath", EventEntityDeath)
	on("onItemDrop", EventItemDrop)
	on("onItemPickup", EventItemPickup)
	on("onPlayerRespawn", EventPlayerRespawn)
	on("onServerStart", EventServerStart)
	on("onServerStop", EventServerStop)
	_ = sp.vm.Set("server", server)

	if _, err := sp.vm.RunScript(path, string(src)); err != nil {
		return err
	}
	log.Printf("[Plugin] Loaded script: %s", sp.name)
	return nil
}
