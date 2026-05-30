package plugin

import (
	"fmt"
	"log"
	"sync"
)

// Plugin is a unit of server-side behaviour. OnEnable receives the Host so the
// plugin can register event handlers and act on the running server.
type Plugin interface {
	Name() string
	Version() string
	OnEnable(host Host) error
	OnDisable() error
}

// Host is the capability surface a plugin gets to act on the running server. It
// is intentionally primitive-typed so the plugin package stays dependency-free.
// Block IDs are LivingWorld canonical state IDs (= vanilla global block-state IDs).
type Host interface {
	// Broadcast sends a chat line to every connected player.
	Broadcast(msg string)
	// Message sends a chat line to one player by name (no-op if absent).
	Message(playerName, msg string)
	// Players returns the names of currently connected players.
	Players() []string
	// PlayerCount returns the number of connected players.
	PlayerCount() int
	// GetBlock returns the block state ID at a world position.
	GetBlock(x, y, z int) int32
	// SetBlock sets the block state ID at a world position and notifies clients.
	SetBlock(x, y, z int, stateID int32)
	// StateID resolves a block state ID from a namespaced name (e.g. "minecraft:stone").
	StateID(name string) int32
	// Log writes a line to the server log, prefixed by the calling plugin.
	Log(format string, args ...any)
}

type loadedPlugin struct {
	plug Plugin
}

var globalManager *PluginManager

// Manager returns the process-wide plugin manager.
func Manager() *PluginManager {
	if globalManager == nil {
		globalManager = NewManager()
	}
	return globalManager
}

// handler pairs an event callback with the plugin that registered it, so a
// panicking handler can be traced back and its plugin disabled.
type handler struct {
	owner string // plugin name; "" when registered outside a plugin's OnEnable
	fn    func(Event)
}

type PluginManager struct {
	plugins       map[string]*loadedPlugin
	eventHandlers map[EventType][]handler
	mu            sync.RWMutex
	regMu         sync.Mutex // serializes Register so handler attribution is correct
	host          Host
	enabling      string // plugin currently in OnEnable (for handler attribution)
}

func NewManager() *PluginManager {
	return &PluginManager{
		plugins:       make(map[string]*loadedPlugin),
		eventHandlers: make(map[EventType][]handler),
	}
}

// SetHost installs the server capability surface handed to plugins.
func (m *PluginManager) SetHost(h Host) {
	m.mu.Lock()
	m.host = h
	m.mu.Unlock()
}

// Host returns the installed Host (may be nil before the server is wired up).
func (m *PluginManager) Host() Host {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.host
}

// Register enables a plugin, handing it the Host.
func (m *PluginManager) Register(p Plugin) error {
	m.regMu.Lock()
	defer m.regMu.Unlock()

	m.mu.Lock()
	if _, exists := m.plugins[p.Name()]; exists {
		m.mu.Unlock()
		return fmt.Errorf("plugin already registered: %s", p.Name())
	}
	host := m.host
	m.enabling = p.Name() // tag handlers registered during OnEnable
	m.mu.Unlock()

	err := p.OnEnable(host)

	m.mu.Lock()
	m.enabling = ""
	m.mu.Unlock()

	if err != nil {
		m.removeHandlers(p.Name()) // roll back partial registration
		return fmt.Errorf("OnEnable failed: %w", err)
	}

	m.mu.Lock()
	m.plugins[p.Name()] = &loadedPlugin{plug: p}
	m.mu.Unlock()
	log.Printf("[Plugin] Enabled: %s v%s", p.Name(), p.Version())
	return nil
}

func (m *PluginManager) Unregister(name string) error {
	m.mu.Lock()
	p, ok := m.plugins[name]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("plugin not found: %s", name)
	}
	delete(m.plugins, name)
	m.mu.Unlock()

	if err := p.plug.OnDisable(); err != nil {
		return fmt.Errorf("OnDisable failed: %w", err)
	}
	log.Printf("[Plugin] Disabled: %s", name)
	return nil
}

func (m *PluginManager) Get(name string) Plugin {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if p, ok := m.plugins[name]; ok {
		return p.plug
	}
	return nil
}

func (m *PluginManager) List() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, 0, len(m.plugins))
	for name := range m.plugins {
		names = append(names, name)
	}
	return names
}

// On registers a raw handler for an event type.
func (m *PluginManager) On(eventType EventType, fn func(Event)) {
	m.mu.Lock()
	m.eventHandlers[eventType] = append(m.eventHandlers[eventType], handler{owner: m.enabling, fn: fn})
	m.mu.Unlock()
}

// Emit dispatches an event to all handlers synchronously, so that cancellable
// events observe handler decisions before the server acts. After Emit returns,
// callers should check evt.Cancelled() for cancellable events.
func (m *PluginManager) Emit(event Event) {
	m.mu.RLock()
	handlers := make([]handler, len(m.eventHandlers[event.Type()]))
	copy(handlers, m.eventHandlers[event.Type()])
	m.mu.RUnlock()
	for _, h := range handlers {
		m.dispatch(h, event)
	}
}

// dispatch runs one handler with panic isolation: a panicking handler never
// crashes the server, and if it belongs to a plugin that plugin is disabled.
func (m *PluginManager) dispatch(h handler, event Event) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[Plugin] handler for %q panicked: %v", event.Type(), r)
			if h.owner != "" {
				log.Printf("[Plugin] disabling %q after panic", h.owner)
				m.disable(h.owner)
			}
		}
	}()
	h.fn(event)
}

// disable removes a plugin's handlers and calls OnDisable, tolerating a panic
// in OnDisable so cleanup of one bad plugin never cascades.
func (m *PluginManager) disable(name string) {
	m.removeHandlers(name)
	defer func() { _ = recover() }()
	_ = m.Unregister(name)
}

// removeHandlers drops every handler registered by the named plugin.
func (m *PluginManager) removeHandlers(owner string) {
	if owner == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for t, hs := range m.eventHandlers {
		kept := make([]handler, 0, len(hs))
		for _, h := range hs {
			if h.owner != owner {
				kept = append(kept, h)
			}
		}
		m.eventHandlers[t] = kept
	}
}

// EmitCancellable dispatches the event and reports whether it was cancelled.
func (m *PluginManager) EmitCancellable(event Event) bool {
	m.Emit(event)
	if c, ok := event.(Cancellable); ok {
		return c.Cancelled()
	}
	return false
}

// --- Ergonomic typed registration helpers (the easy path for plugin authors) ---

func (m *PluginManager) OnPlayerJoin(fn func(*PlayerJoinEvent)) {
	m.On(EventPlayerJoin, func(e Event) { fn(e.(*PlayerJoinEvent)) })
}
func (m *PluginManager) OnPlayerLeave(fn func(*PlayerLeaveEvent)) {
	m.On(EventPlayerLeave, func(e Event) { fn(e.(*PlayerLeaveEvent)) })
}
func (m *PluginManager) OnPlayerChat(fn func(*PlayerChatEvent)) {
	m.On(EventPlayerChat, func(e Event) { fn(e.(*PlayerChatEvent)) })
}
func (m *PluginManager) OnBlockBreak(fn func(*BlockBreakEvent)) {
	m.On(EventBlockBreak, func(e Event) { fn(e.(*BlockBreakEvent)) })
}
func (m *PluginManager) OnBlockPlace(fn func(*BlockPlaceEvent)) {
	m.On(EventBlockPlace, func(e Event) { fn(e.(*BlockPlaceEvent)) })
}
func (m *PluginManager) OnPlayerInteract(fn func(*PlayerInteractEvent)) {
	m.On(EventPlayerInteract, func(e Event) { fn(e.(*PlayerInteractEvent)) })
}
func (m *PluginManager) OnPlayerAttack(fn func(*PlayerAttackEvent)) {
	m.On(EventPlayerAttack, func(e Event) { fn(e.(*PlayerAttackEvent)) })
}
func (m *PluginManager) OnEntityDamage(fn func(*EntityDamageEvent)) {
	m.On(EventEntityDamage, func(e Event) { fn(e.(*EntityDamageEvent)) })
}
func (m *PluginManager) OnEntityDeath(fn func(*EntityDeathEvent)) {
	m.On(EventEntityDeath, func(e Event) { fn(e.(*EntityDeathEvent)) })
}
func (m *PluginManager) OnPlayerCommand(fn func(*PlayerCommandEvent)) {
	m.On(EventPlayerCommand, func(e Event) { fn(e.(*PlayerCommandEvent)) })
}
func (m *PluginManager) OnContainerClick(fn func(*ContainerClickEvent)) {
	m.On(EventContainerClick, func(e Event) { fn(e.(*ContainerClickEvent)) })
}
func (m *PluginManager) OnItemDrop(fn func(*ItemDropEvent)) {
	m.On(EventItemDrop, func(e Event) { fn(e.(*ItemDropEvent)) })
}
func (m *PluginManager) OnItemPickup(fn func(*ItemPickupEvent)) {
	m.On(EventItemPickup, func(e Event) { fn(e.(*ItemPickupEvent)) })
}
func (m *PluginManager) OnPlayerRespawn(fn func(*PlayerRespawnEvent)) {
	m.On(EventPlayerRespawn, func(e Event) { fn(e.(*PlayerRespawnEvent)) })
}
