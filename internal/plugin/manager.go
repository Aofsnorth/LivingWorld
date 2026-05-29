package plugin

import (
	"fmt"
	"log"
	"sync"
)

type Plugin interface {
	OnEnable() error
	OnDisable() error
	Name() string
	Version() string
}

type loadedPlugin struct {
	plug Plugin
}

var globalManager *PluginManager

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
	server        interface{}
}

func NewManager() *PluginManager {
	return &PluginManager{
		plugins:       make(map[string]*loadedPlugin),
		eventHandlers: make(map[EventType][]func(Event)),
	}
}

func (m *PluginManager) SetServer(server interface{}) {
	m.server = server
}

func (m *PluginManager) Register(p Plugin) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.plugins[p.Name()]; exists {
		return fmt.Errorf("plugin already registered: %s", p.Name())
	}

	if err := p.OnEnable(); err != nil {
		return fmt.Errorf("OnEnable failed: %w", err)
	}

	m.plugins[p.Name()] = &loadedPlugin{plug: p}
	log.Printf("[Plugin] Registered: %s v%s", p.Name(), p.Version())
	return nil
}

func (m *PluginManager) Unregister(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.plugins[name]
	if !ok {
		return fmt.Errorf("plugin not found: %s", name)
	}

	if err := p.plug.OnDisable(); err != nil {
		return fmt.Errorf("OnDisable failed: %w", err)
	}

	delete(m.plugins, name)
	log.Printf("[Plugin] Unregistered: %s", name)
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

func (m *PluginManager) Emit(event Event) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	handlers := m.eventHandlers[event.Type()]
	for _, handler := range handlers {
		go handler(event)
	}
}

func (m *PluginManager) On(eventType EventType, handler func(Event)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.eventHandlers[eventType] = append(m.eventHandlers[eventType], handler)
}