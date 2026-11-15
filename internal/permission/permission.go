package permission

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Package permission implements an edition-agnostic, hierarchical RBAC permission engine
// rebuilt from scratch for LivingWorld using GoCraft's core/permission architectural principles:
// - Weighted groups with multi-parent DAG inheritance and cycle protection
// - Exact node, prefix wildcard ("livingworld.command.*"), and global wildcard ("*") matching
// - Specificity-first resolution with explicit negation (`false` overrides `true` at equal weight/specificity)
// - Operator bypass and atomic JSON file persistence

const DocumentVersion = 1

type Group struct {
	Weight      int             `json:"weight"`
	Prefix      string          `json:"prefix,omitempty"`
	Parents     []string        `json:"parents,omitempty"`
	Permissions map[string]bool `json:"permissions,omitempty"`
}

type User struct {
	Groups      []string        `json:"groups,omitempty"`
	Permissions map[string]bool `json:"permissions,omitempty"`
}

type Document struct {
	Version int              `json:"version"`
	Groups  map[string]Group `json:"groups"`
	Users   map[string]User  `json:"users"`
}

type Engine struct {
	mu     sync.RWMutex
	groups map[string]Group
	users  map[string]User
}

func DefaultDocument() Document {
	return Document{
		Version: DocumentVersion,
		Groups: map[string]Group{
			"default": {
				Weight: 0,
				Permissions: map[string]bool{
					"livingworld.command.help":  true,
					"livingworld.command.list":  true,
					"livingworld.command.spawn": true,
					"livingworld.command.msg":   true,
				},
			},
			"vip": {
				Weight:  10,
				Prefix:  "[VIP] ",
				Parents: []string{"default"},
				Permissions: map[string]bool{
					"livingworld.command.tp":   true,
					"livingworld.command.home": true,
				},
			},
			"moderator": {
				Weight:  50,
				Prefix:  "[Mod] ",
				Parents: []string{"vip"},
				Permissions: map[string]bool{
					"livingworld.command.kick":     true,
					"livingworld.command.ban":      true,
					"livingworld.command.gamemode": true,
					"livingworld.command.time":     true,
					"livingworld.command.weather":  true,
				},
			},
			"admin": {
				Weight:  100,
				Prefix:  "[Admin] ",
				Parents: []string{"moderator"},
				Permissions: map[string]bool{
					"*": true,
				},
			},
		},
		Users: map[string]User{},
	}
}

func NewEngine(doc Document) (*Engine, error) {
	e := &Engine{}
	if err := e.LoadDocument(doc); err != nil {
		return nil, err
	}
	return e, nil
}

func LoadOrCreateFile(path string) (*Engine, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("read permission file: %w", err)
		}
		doc := DefaultDocument()
		eng, err := NewEngine(doc)
		if err != nil {
			return nil, err
		}
		if err := eng.SaveFile(path); err != nil {
			return nil, err
		}
		return eng, nil
	}

	var doc Document
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("decode permission file: %w", err)
	}
	return NewEngine(doc)
}

func (e *Engine) LoadDocument(doc Document) error {
	if doc.Version == 0 {
		doc.Version = DocumentVersion
	}
	if doc.Version != DocumentVersion {
		return fmt.Errorf("unsupported permission document version %d", doc.Version)
	}

	groups := make(map[string]Group, len(doc.Groups))
	for name, g := range doc.Groups {
		key := normalizeKey(name)
		if key == "" {
			continue
		}
		parents := make([]string, 0, len(g.Parents))
		for _, p := range g.Parents {
			if pk := normalizeKey(p); pk != "" {
				parents = append(parents, pk)
			}
		}
		groups[key] = Group{
			Weight:      g.Weight,
			Prefix:      g.Prefix,
			Parents:     parents,
			Permissions: normalizePermMap(g.Permissions),
		}
	}
	if _, ok := groups["default"]; !ok {
		groups["default"] = DefaultDocument().Groups["default"]
	}

	users := make(map[string]User, len(doc.Users))
	for name, u := range doc.Users {
		key := normalizeKey(name)
		if key == "" {
			continue
		}
		ugroups := make([]string, 0, len(u.Groups))
		for _, g := range u.Groups {
			if gk := normalizeKey(g); gk != "" {
				ugroups = append(ugroups, gk)
			}
		}
		users[key] = User{
			Groups:      ugroups,
			Permissions: normalizePermMap(u.Permissions),
		}
	}

	e.mu.Lock()
	e.groups = groups
	e.users = users
	e.mu.Unlock()
	return nil
}

func (e *Engine) SaveFile(path string) error {
	e.mu.RLock()
	doc := Document{
		Version: DocumentVersion,
		Groups:  e.groups,
		Users:   e.users,
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	e.mu.RUnlock()
	if err != nil {
		return fmt.Errorf("encode permissions: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir permissions dir: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("write permissions tmp: %w", err)
	}
	return os.Rename(tmp, path)
}

func (e *Engine) Allowed(username, node string, operator, defaultAllowed bool) bool {
	if operator {
		return true
	}
	if e == nil {
		return defaultAllowed
	}

	targetNode := normalizeKey(node)
	if targetNode == "" {
		return defaultAllowed
	}
	userKey := normalizeKey(username)

	e.mu.RLock()
	defer e.mu.RUnlock()

	if u, ok := e.users[userKey]; ok {
		if decision, matched := matchBestInMap(u.Permissions, targetNode); matched {
			return decision
		}
	}

	bestMatched := false
	bestDecision := defaultAllowed
	bestWeight := -1 << 30
	bestSpec := -1

	visited := make(map[string]bool, len(e.groups))
	var visitGroup func(groupName string)
	visitGroup = func(groupName string) {
		gKey := normalizeKey(groupName)
		if gKey == "" || visited[gKey] {
			return
		}
		visited[gKey] = true

		g, ok := e.groups[gKey]
		if !ok {
			return
		}
		for _, parent := range g.Parents {
			visitGroup(parent)
		}

		for pattern, allow := range g.Permissions {
			spec, matched := matchSpecificity(pattern, targetNode)
			if !matched {
				continue
			}
			if !bestMatched ||
				g.Weight > bestWeight ||
				(g.Weight == bestWeight && spec > bestSpec) ||
				(g.Weight == bestWeight && spec == bestSpec && !allow && bestDecision) {
				bestMatched = true
				bestWeight = g.Weight
				bestSpec = spec
				bestDecision = allow
			}
		}
	}

	visitGroup("default")
	if u, ok := e.users[userKey]; ok {
		for _, gName := range u.Groups {
			visitGroup(gName)
		}
	}

	if bestMatched {
		return bestDecision
	}
	return defaultAllowed
}

func (e *Engine) Prefix(username string) string {
	if e == nil {
		return ""
	}
	userKey := normalizeKey(username)
	e.mu.RLock()
	defer e.mu.RUnlock()

	bestWeight := -1 << 30
	bestPrefix := ""
	visited := make(map[string]bool, len(e.groups))

	var visit func(groupName string)
	visit = func(groupName string) {
		gKey := normalizeKey(groupName)
		if gKey == "" || visited[gKey] {
			return
		}
		visited[gKey] = true
		g, ok := e.groups[gKey]
		if !ok {
			return
		}
		for _, p := range g.Parents {
			visit(p)
		}
		if g.Prefix != "" && g.Weight >= bestWeight {
			bestWeight = g.Weight
			bestPrefix = g.Prefix
		}
	}

	visit("default")
	if u, ok := e.users[userKey]; ok {
		for _, gName := range u.Groups {
			visit(gName)
		}
	}
	return bestPrefix
}

func (e *Engine) AssignUserGroup(username, groupName string) {
	uKey := normalizeKey(username)
	gKey := normalizeKey(groupName)
	if uKey == "" || gKey == "" {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	u := e.users[uKey]
	for _, existing := range u.Groups {
		if existing == gKey {
			return
		}
	}
	u.Groups = append(u.Groups, gKey)
	if u.Permissions == nil {
		u.Permissions = make(map[string]bool)
	}
	e.users[uKey] = u
}

func (e *Engine) SetUserPermission(username, node string, allow bool) {
	uKey := normalizeKey(username)
	nKey := normalizeKey(node)
	if uKey == "" || nKey == "" {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	u := e.users[uKey]
	if u.Permissions == nil {
		u.Permissions = make(map[string]bool)
	}
	u.Permissions[nKey] = allow
	e.users[uKey] = u
}

func matchBestInMap(perms map[string]bool, node string) (bool, bool) {
	bestSpec := -1
	bestDecision := false
	matchedAny := false

	for pattern, allow := range perms {
		spec, ok := matchSpecificity(pattern, node)
		if !ok {
			continue
		}
		if !matchedAny || spec > bestSpec || (spec == bestSpec && !allow && bestDecision) {
			matchedAny = true
			bestSpec = spec
			bestDecision = allow
		}
	}
	return bestDecision, matchedAny
}

func matchSpecificity(pattern, node string) (int, bool) {
	p := normalizeKey(pattern)
	if p == "" {
		return 0, false
	}
	if p == "*" {
		return 1, true
	}
	if p == node {
		return 1000 + len(p), true
	}
	if strings.HasSuffix(p, ".*") {
		prefix := strings.TrimSuffix(p, ".*")
		if prefix != "" && (node == prefix || strings.HasPrefix(node, prefix+".")) {
			return 100 + len(prefix), true
		}
	}
	return 0, false
}

func normalizePermMap(in map[string]bool) map[string]bool {
	out := make(map[string]bool, len(in))
	for k, v := range in {
		if nk := normalizeKey(k); nk != "" {
			out[nk] = v
		}
	}
	return out
}

func normalizeKey(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
