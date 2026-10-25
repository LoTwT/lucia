package main

import (
	"testing"

	"github.com/LoTwT/lucia/internal/builtinpack"
)

func TestNewPetModelFallsBackToBuiltin(t *testing.T) {
	builtin, err := builtinpack.Load(2)
	if err != nil {
		t.Fatal(err)
	}
	model, fellBack, err := newPetModel(t.TempDir(), "missing", builtin)
	if err != nil || model == nil || !fellBack {
		t.Fatalf("fallback model=%v fellBack=%v err=%v", model != nil, fellBack, err)
	}
	model.Close()

	model, fellBack, err = newPetModel(t.TempDir(), "", builtin)
	if err != nil || model == nil || fellBack {
		t.Fatalf("default model=%v fellBack=%v err=%v", model != nil, fellBack, err)
	}
	if model.HasEvent("intro") {
		t.Fatal("built-in fallback must not claim a first-run intro")
	}
	model.Close()
}
