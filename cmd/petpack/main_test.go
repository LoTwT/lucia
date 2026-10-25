package main

import (
	"strings"
	"testing"

	"github.com/LoTwT/lucia/internal/petpack"
)

func TestStrictManifestErrorRequiresAuthoredSchema2Data(t *testing.T) {
	raw := petpack.Manifest{Schema: 1}
	if err := strictManifestError("tiny", raw); err == nil || !strings.Contains(err.Error(), "schema 2") {
		t.Fatalf("schema 1 error = %v", err)
	}
	raw.Schema = 2
	if err := strictManifestError("tiny", raw); err == nil || !strings.Contains(err.Error(), "hotzone") {
		t.Fatalf("missing hotzone error = %v", err)
	}
	raw.Hotzones = map[string]petpack.Hotzone{"face": {}}
	if err := strictManifestError("tiny", raw); err == nil || !strings.Contains(err.Error(), "preset") {
		t.Fatalf("missing preset error = %v", err)
	}
	raw.Presets = map[string]petpack.Preset{"tap": {}}
	if err := strictManifestError("tiny", raw); err != nil {
		t.Fatalf("explicit interaction data rejected: %v", err)
	}
}
