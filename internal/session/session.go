// Package session tracks live connections and in-flight queries so the UI can
// open, reuse and cancel them by id.
package session

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/HamzaBaghdadi/quarry/internal/driver"
)

// Manager owns every open driver.Conn.
type Manager struct {
	mu      sync.Mutex
	conns   map[string]driver.Conn
	running map[string]context.CancelFunc
}

func NewManager() *Manager {
	return &Manager{conns: map[string]driver.Conn{}, running: map[string]context.CancelFunc{}}
}

// Connect opens (or reuses) the connection for profile id.
func (m *Manager) Connect(ctx context.Context, id, driverID string, cfg driver.Config) error {
	m.mu.Lock()
	if _, ok := m.conns[id]; ok {
		m.mu.Unlock()
		return nil
	}
	m.mu.Unlock()

	conn, err := Dial(ctx, driverID, cfg)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.conns[id]; ok { // lost a race; keep the first
		_ = conn.Close()
		_ = existing
		return nil
	}
	m.conns[id] = conn
	return nil
}

// Dial opens a connection with a sane timeout.
func Dial(ctx context.Context, driverID string, cfg driver.Config) (driver.Conn, error) {
	d, err := driver.Get(driverID)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	conn, err := d.Open(ctx, cfg)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("timed out connecting to %s", d.Info().Name)
		}
		return nil, err
	}
	return conn, nil
}

// Get returns the open connection for id.
func (m *Manager) Get(id string) (driver.Conn, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.conns[id]
	if !ok {
		return nil, errors.New("not connected")
	}
	return c, nil
}

// IsOpen reports whether id has a live connection.
func (m *Manager) IsOpen(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.conns[id]
	return ok
}

// Disconnect closes the connection for id.
func (m *Manager) Disconnect(id string) error {
	m.mu.Lock()
	c, ok := m.conns[id]
	delete(m.conns, id)
	m.mu.Unlock()
	if !ok {
		return nil
	}
	return c.Close()
}

// CloseAll closes every connection (app shutdown).
func (m *Manager) CloseAll() {
	m.mu.Lock()
	conns := m.conns
	m.conns = map[string]driver.Conn{}
	for _, cancel := range m.running {
		cancel()
	}
	m.mu.Unlock()
	for _, c := range conns {
		_ = c.Close()
	}
}

// Track registers a cancellable query under runID and returns its context.
func (m *Manager) Track(parent context.Context, runID string) (context.Context, func()) {
	ctx, cancel := context.WithCancel(parent)
	if runID == "" {
		return ctx, cancel
	}
	m.mu.Lock()
	m.running[runID] = cancel
	m.mu.Unlock()
	return ctx, func() {
		cancel()
		m.mu.Lock()
		delete(m.running, runID)
		m.mu.Unlock()
	}
}

// Cancel aborts the query registered under runID.
func (m *Manager) Cancel(runID string) bool {
	m.mu.Lock()
	cancel, ok := m.running[runID]
	m.mu.Unlock()
	if ok {
		cancel()
	}
	return ok
}
