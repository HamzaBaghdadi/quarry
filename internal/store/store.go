// Package store persists saved connection profiles (Repository pattern).
// Profiles live in a JSON file under the OS config dir; passwords go to the
// OS keychain (Windows Credential Manager, macOS Keychain, Secret Service) and
// only fall back to the file when no keychain is available.
package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/zalando/go-keyring"

	"github.com/HamzaBaghdadi/quarry/internal/driver"
)

const keyringService = "dev.quarry.app"

// Profile is a saved connection.
type Profile struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	Driver    string        `json:"driver"`
	Color     string        `json:"color"`
	Config    driver.Config `json:"config"`
	ReadOnly  bool          `json:"readOnly"`
	CreatedAt time.Time     `json:"createdAt"`
	UpdatedAt time.Time     `json:"updatedAt"`
	// PlainPassword is only used when the OS keychain is unavailable.
	PlainPassword string `json:"plainPassword,omitempty"`
}

// Store is a file-backed repository of profiles.
type Store struct {
	mu   sync.Mutex
	path string
}

// Open returns a store rooted in the user's config directory.
func Open() (*Store, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	dir = filepath.Join(dir, "Quarry")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Store{path: filepath.Join(dir, "connections.json")}, nil
}

func (s *Store) load() ([]Profile, error) {
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return []Profile{}, nil
	}
	if err != nil {
		return nil, err
	}
	var ps []Profile
	if err := json.Unmarshal(b, &ps); err != nil {
		return nil, err
	}
	return ps, nil
}

func (s *Store) save(ps []Profile) error {
	b, err := json.MarshalIndent(ps, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// List returns all profiles, without passwords.
func (s *Store) List() ([]Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ps, err := s.load()
	if err != nil {
		return nil, err
	}
	sort.SliceStable(ps, func(i, j int) bool { return ps[i].Name < ps[j].Name })
	for i := range ps {
		ps[i].Config.Password = ""
		ps[i].PlainPassword = ""
	}
	return ps, nil
}

// Get returns one profile with its password filled in.
func (s *Store) Get(id string) (Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ps, err := s.load()
	if err != nil {
		return Profile{}, err
	}
	for _, p := range ps {
		if p.ID == id {
			if pw, err := keyring.Get(keyringService, id); err == nil {
				p.Config.Password = pw
			} else {
				p.Config.Password = p.PlainPassword
			}
			p.PlainPassword = ""
			return p, nil
		}
	}
	return Profile{}, errors.New("connection not found")
}

// Save inserts or updates a profile. An empty password on update keeps the
// stored one, so the UI never needs to read secrets back.
func (s *Store) Save(p Profile, keepPassword bool) (Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ps, err := s.load()
	if err != nil {
		return Profile{}, err
	}
	now := time.Now()
	if p.ID == "" {
		p.ID = uuid.NewString()
		p.CreatedAt = now
	}
	p.UpdatedAt = now
	pw := p.Config.Password
	p.Config.Password = ""
	p.PlainPassword = ""

	idx := -1
	for i := range ps {
		if ps[i].ID == p.ID {
			idx = i
			p.CreatedAt = ps[i].CreatedAt
			if keepPassword {
				p.PlainPassword = ps[i].PlainPassword
			}
		}
	}
	if !keepPassword {
		if pw == "" {
			_ = keyring.Delete(keyringService, p.ID)
		} else if err := keyring.Set(keyringService, p.ID, pw); err != nil {
			p.PlainPassword = pw
		}
	}
	if idx >= 0 {
		ps[idx] = p
	} else {
		ps = append(ps, p)
	}
	if err := s.save(ps); err != nil {
		return Profile{}, err
	}
	p.PlainPassword = ""
	return p, nil
}

// Delete removes a profile and its secret.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ps, err := s.load()
	if err != nil {
		return err
	}
	out := ps[:0]
	for _, p := range ps {
		if p.ID != id {
			out = append(out, p)
		}
	}
	_ = keyring.Delete(keyringService, id)
	return s.save(out)
}
