// Package prefs stores application preferences that are separate from data source settings.
package prefs

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// Window remembers the last normal window size and whether it was maximised.
type Window struct {
	Width     int  `json:"width,omitempty"`
	Height    int  `json:"height,omitempty"`
	Maximised bool `json:"maximised,omitempty"`
}

// Preferences are persisted as JSON next to settings.json.
type Preferences struct {
	UpdateCheck *bool  `json:"update_check,omitempty"`
	Window      Window `json:"window"`
}

// AutoUpdateCheck reports whether startup update checks are enabled (default on).
func (p Preferences) AutoUpdateCheck() bool {
	return p.UpdateCheck == nil || *p.UpdateCheck
}

// Store reads and writes preferences.json.
type Store struct {
	mu    sync.Mutex
	path  string
	value Preferences
}

// Open loads preferences from path; a missing or damaged file yields defaults.
func Open(path string) *Store {
	store := &Store{path: path}
	if data, err := os.ReadFile(path); err == nil {
		if json.Unmarshal(data, &store.value) != nil {
			store.value = Preferences{}
		}
	}
	return store
}

// Get returns a copy of the current preferences.
func (s *Store) Get() Preferences {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.value
}

// Update applies change and writes the file atomically.
func (s *Store) Update(change func(*Preferences)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.value
	change(&next)
	if s.path == "" {
		return errors.New("preferences path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	temporary := s.path + ".tmp"
	if err := os.WriteFile(temporary, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(temporary, s.path); err != nil {
		os.Remove(temporary)
		return err
	}
	s.value = next
	return nil
}
