package driver

import (
	"fmt"
	"sort"
	"sync"
)

var (
	mu      sync.RWMutex
	drivers = map[string]Driver{}
	order   []string
)

// Register makes a driver available. Adapters call it from init(), so wiring a
// new engine into the app is a single blank import in main.go.
func Register(d Driver) {
	mu.Lock()
	defer mu.Unlock()
	id := d.Info().ID
	if _, dup := drivers[id]; dup {
		panic("driver: duplicate registration for " + id)
	}
	drivers[id] = d
	order = append(order, id)
}

// Get returns the driver registered under id.
func Get(id string) (Driver, error) {
	mu.RLock()
	defer mu.RUnlock()
	d, ok := drivers[id]
	if !ok {
		return nil, fmt.Errorf("unknown driver %q", id)
	}
	return d, nil
}

// All lists every registered driver's Info, sorted by name.
func All() []Info {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Info, 0, len(order))
	for _, id := range order {
		out = append(out, drivers[id].Info())
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
