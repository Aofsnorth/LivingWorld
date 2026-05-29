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

type PluginManager struct {
	plugins       map[string]*loadedPlugin
	eventHandlers map[EventType][]func(Event)
	mu            sync.RWMutex
	host          Host
}

func NewManager() *PluginManager {
	return &PluginManager{
		plugins:       make(map[string]*loadedPlugin),
		eventHandlers: make(map[EventType][]func(Event)),
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
	m.mu.Lock()
	if _, exists := m.plugins[p.Name()]; exists {
		m.mu.Unlock()
		return fmt.Errorf("plugin already registered: %s", p.Name())
	}
	host := m.host
	m.mu.Unlock()

	if err := p.OnEnable(host); err != nil {
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
func (m *PluginManager) On(eventType EventType, handler func(Event)) {
	m.mu.Lock()
	m.eventHandlers[eventType] = append(m.eventHandlers[eventType], handler)
	m.mu.Unlock()
}

// Emit dispatches an event to all handlers synchronously, so that cancellable
// events observe handler decisions before the server acts. After Emit returns,
// callers should check evt.Cancelled() for cancellable events.
func (m *PluginManager) Emit(event Event) {
	m.mu.RLock()
	handlers := make([]func(Event), len(m.eventHandlers[event.Type()]))
	copy(handlers, m.eventHandlers[event.Type()])
	m.mu.RUnlock()
	for _, handler := range handlers {
		handler(event)
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
