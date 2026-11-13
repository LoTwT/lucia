package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo"
)

func TestPetWindowStateCapturesFixedPanelAndSurvivesClose(t *testing.T) {
	want := mygo.WindowState{X: 700, Y: 330, Width: 248, Height: 248}
	console := mygo.WindowState{X: 100, Y: 80, Width: 1000, Height: 700}
	alive := true
	store := &petWindowStateStore{
		file: filepath.Join(t.TempDir(), "window-state.json"),
		petState: func() (mygo.WindowState, bool) {
			return want, alive
		},
	}
	// MyGo sees isZoomed=true, so its snapshot retains the old normal
	// bounds (or has no pet entry at all on the first launch).
	stale := mygo.WindowState{X: 1240, Y: 670, Width: 248, Height: 248, Maximized: true}
	for _, petClosed := range []bool{false, true} {
		alive = !petClosed
		states := map[string]mygo.WindowState{"main": console, "pet": stale}
		if err := store.SaveWindowStates(states); err != nil {
			t.Fatal(err)
		}
		// The framework owns its input map; the store must not mutate it.
		if states["pet"] != stale {
			t.Fatal("save mutated the framework snapshot")
		}
		got := readWindowStates(t, store.file)
		if got["pet"] != want || got["main"] != console {
			t.Fatalf("pet closed=%v: states=%+v", petClosed, got)
		}
	}
	// A fresh store reads the same file format on the next launch.
	restarted := &petWindowStateStore{file: store.file}
	loaded, err := restarted.LoadWindowStates()
	if err != nil || loaded["pet"] != want || loaded["main"] != console {
		t.Fatalf("restart: states=%+v err=%v", loaded, err)
	}
}

func TestPetWindowStateQuitFlushWithoutFrameworkChanges(t *testing.T) {
	want := mygo.WindowState{X: 300, Y: 250, Width: 310, Height: 310}
	store := &petWindowStateStore{
		file: filepath.Join(t.TempDir(), "window-state.json"),
		petState: func() (mygo.WindowState, bool) {
			return want, true
		},
	}
	// Moving only the fixed panel need not mark MyGo's state dirty.
	// The quit-time flush must persist it without a framework save.
	if err := store.savePetState(); err != nil {
		t.Fatal(err)
	}
	if got := readWindowStates(t, store.file)["pet"]; got != want {
		t.Fatalf("quit saved %+v, want %+v", got, want)
	}
	if err := store.SaveWindowStates(nil); err != nil {
		t.Fatal(err)
	}
	if err := store.savePetState(); err != nil {
		t.Fatal(err)
	}
	if got := readWindowStates(t, store.file); len(got) != 0 {
		t.Fatalf("quit restored states after App.ClearWindowStates: %+v", got)
	}
}

func readWindowStates(t *testing.T, file string) map[string]mygo.WindowState {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var states map[string]mygo.WindowState
	if err := json.Unmarshal(data, &states); err != nil {
		t.Fatal(err)
	}
	return states
}
