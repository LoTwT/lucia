package main

import (
	"encoding/json"
	"log"
	"maps"
	"os"
	"path/filepath"
	"runtime"

	"github.com/egoist/mygo"
)

// configureWindowStateStore works around MyGo v0.9.0 treating macOS's
// isZoomed on a fixed-size panel as maximized, which skips normal bounds
// capture. Keep MyGo's file and keys; only replace the pet's saved bounds.
func configureWindowStateStore(dataDir string, pet func() *petWindow) {
	if runtime.GOOS != "darwin" {
		return
	}
	store := &petWindowStateStore{
		file: filepath.Join(dataDir, "window-state.json"),
		petState: func() (mygo.WindowState, bool) {
			p := pet()
			if p == nil || p.win == nil || p.win.IsDestroyed() {
				return mygo.WindowState{}, false
			}
			b := p.win.Bounds()
			return mygo.WindowState{X: b.X, Y: b.Y, Width: b.Width, Height: b.Height}, true
		},
	}
	mygo.App.SetWindowStateStore(store)
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) {
		// MyGo may have no dirty state when only the pet moved. Flush while
		// the panel still exists, before the framework closes any windows.
		if err := store.savePetState(); err != nil {
			log.Printf("lucia: save window state: %v", err)
		}
	})
}

// petWindowStateStore runs on the UI thread, like MyGo's default store.
// lastPet survives the panel closing so later saves of the console cannot
// replace its actual bounds with MyGo's stale maximized state.
type petWindowStateStore struct {
	file     string
	states   map[string]mygo.WindowState
	petState func() (mygo.WindowState, bool)
	lastPet  mygo.WindowState
	hasPet   bool
	cleared  bool
}

func (s *petWindowStateStore) LoadWindowStates() (map[string]mygo.WindowState, error) {
	data, err := os.ReadFile(s.file)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var states map[string]mygo.WindowState
	if err := json.Unmarshal(data, &states); err != nil {
		return nil, err
	}
	s.states = maps.Clone(states)
	return states, nil
}

func (s *petWindowStateStore) SaveWindowStates(states map[string]mygo.WindowState) error {
	next := maps.Clone(states)
	if next == nil {
		next = make(map[string]mygo.WindowState)
	}
	if len(next) == 0 {
		// An empty save is App.ClearWindowStates: keep it cleared for the
		// rest of this run, including our explicit quit-time flush.
		s.cleared, s.hasPet = true, false
	} else if !s.cleared {
		s.capturePetState()
		if s.hasPet {
			next["pet"] = s.lastPet
		}
	}
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.file), "window-state.json.tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), s.file); err != nil {
		return err
	}
	s.states = next
	return nil
}

func (s *petWindowStateStore) capturePetState() {
	if state, ok := s.petState(); ok && state.Width > 0 && state.Height > 0 {
		s.lastPet, s.hasPet = state, true
	}
}

func (s *petWindowStateStore) savePetState() error {
	if s.cleared {
		return nil
	}
	s.capturePetState()
	if !s.hasPet {
		return nil
	}
	states := maps.Clone(s.states)
	if states == nil {
		states = make(map[string]mygo.WindowState)
	}
	states["pet"] = s.lastPet
	return s.SaveWindowStates(states)
}
