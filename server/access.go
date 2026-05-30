package server

import (
	"bufio"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

// nameSet is a case-insensitive, optionally file-backed set of player names. It
// is the shared core of OpsList and Whitelist. The on-disk format is one name
// per line; blank lines and lines starting with '#' are ignored.
type nameSet struct {
	mu    sync.RWMutex
	path  string
	names map[string]string // lower-case key -> original casing
}

func loadNameSet(path string) (*nameSet, error) {
	ns := &nameSet{path: path, names: map[string]string{}}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ns, nil
		}
		return nil, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		ns.names[strings.ToLower(line)] = line
	}
	return ns, sc.Err()
}

// Has reports whether name is in the set (case-insensitive).
func (n *nameSet) Has(name string) bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	_, ok := n.names[strings.ToLower(name)]
	return ok
}

// Add inserts name and persists the set. It reports false if already present.
func (n *nameSet) Add(name string) (bool, error) {
	name = strings.TrimSpace(name)
	n.mu.Lock()
	key := strings.ToLower(name)
	if name == "" || n.names[key] != "" {
		n.mu.Unlock()
		return false, nil
	}
	n.names[key] = name
	n.mu.Unlock()
	return true, n.save()
}

// Remove deletes name and persists the set. It reports false if not present.
func (n *nameSet) Remove(name string) (bool, error) {
	n.mu.Lock()
	key := strings.ToLower(name)
	if _, ok := n.names[key]; !ok {
		n.mu.Unlock()
		return false, nil
	}
	delete(n.names, key)
	n.mu.Unlock()
	return true, n.save()
}

// List returns the names in sorted order.
func (n *nameSet) List() []string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	out := make([]string, 0, len(n.names))
	for _, v := range n.names {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func (n *nameSet) save() error {
	if n.path == "" {
		return nil
	}
	out := n.List()
	var data string
	if len(out) > 0 {
		data = strings.Join(out, "\n") + "\n"
	}
	return os.WriteFile(n.path, []byte(data), 0o644)
}

// OpsList is the set of server operators (admins). Membership grants permission
// to run operator-level commands; it is consumed at login via Config.IsOp
// (Main copies List() into Config.Ops).
type OpsList struct{ *nameSet }

// LoadOps reads the ops list from path (one name per line). A missing file
// yields an empty, file-backed list.
func LoadOps(path string) (*OpsList, error) {
	ns, err := loadNameSet(path)
	if err != nil {
		return nil, err
	}
	return &OpsList{ns}, nil
}

// newOps builds an in-memory (non-persistent) ops list from names.
func newOps(names ...string) *OpsList {
	ns := &nameSet{names: map[string]string{}}
	for _, name := range names {
		if name = strings.TrimSpace(name); name != "" {
			ns.names[strings.ToLower(name)] = name
		}
	}
	return &OpsList{ns}
}

// Whitelist is the set of players allowed to join while it is enabled (R2.4).
// When disabled, everyone is allowed.
type Whitelist struct {
	*nameSet
	enabled atomic.Bool
}

// LoadWhitelist reads the whitelist from path (one name per line). A missing
// file yields an empty, file-backed, disabled whitelist.
func LoadWhitelist(path string) (*Whitelist, error) {
	ns, err := loadNameSet(path)
	if err != nil {
		return nil, err
	}
	return &Whitelist{nameSet: ns}, nil
}

// newWhitelist builds an empty, in-memory, disabled whitelist.
func newWhitelist() *Whitelist {
	return &Whitelist{nameSet: &nameSet{names: map[string]string{}}}
}

// Enabled reports whether whitelist enforcement is on.
func (w *Whitelist) Enabled() bool { return w.enabled.Load() }

// SetEnabled toggles whitelist enforcement.
func (w *Whitelist) SetEnabled(on bool) { w.enabled.Store(on) }

// Allowed reports whether name may join: always true when the whitelist is
// disabled, otherwise true only when name is listed. The protocol login path
// is expected to deny connections for which Allowed returns false.
func (w *Whitelist) Allowed(name string) bool {
	return !w.enabled.Load() || w.Has(name)
}
