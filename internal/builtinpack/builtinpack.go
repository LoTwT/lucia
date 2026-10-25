// Package builtinpack embeds Lucia's default pet artwork as a normal pack.
// The built-in pack uses the same loader, manifest and player as installed
// packs; replacing the default skin means replacing this directory.
package builtinpack

import (
	"embed"
	"fmt"
	"io/fs"

	"github.com/LoTwT/lucia/internal/petpack"
)

//go:embed whale
var content embed.FS

// FS returns the embedded whale pack rooted at its manifest directory.
func FS() (fs.FS, error) {
	sub, err := fs.Sub(content, "whale")
	if err != nil {
		return nil, fmt.Errorf("built-in pack: %w", err)
	}
	return sub, nil
}

// Load validates and loads the embedded whale pack.
func Load(displayScale float64) (*petpack.Pack, error) {
	fsys, err := FS()
	if err != nil {
		return nil, err
	}
	pack, err := petpack.LoadFS(fsys, petpack.BuiltinPackID, displayScale)
	if err != nil {
		return nil, fmt.Errorf("built-in pack: %w", err)
	}
	return pack, nil
}

// Summary builds the console row for the embedded pack.
func Summary(pack *petpack.Pack, active bool) petpack.Summary {
	summary := petpack.SummaryFromManifest(pack.Manifest, active)
	summary.Builtin = true
	return summary
}

// Preview encodes the built-in pack's idle poster.
func Preview(pack *petpack.Pack) ([]byte, error) {
	return pack.PreviewPNG()
}
