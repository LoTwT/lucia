package petpack

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const (
	maxSourceBytes    = 4 << 20
	maxSourceHotzones = 64
	maxSourceMenu     = 32
	sourceFileName    = "pack.source.json"
)

// Source is the author-facing contract used by the build command. It is
// deliberately not Manifest: runtime-derived fields such as clips, hashes,
// actions, states and events are generated and must not be authored here.
type Source struct {
	Source       int                `json:"source"`
	ID           string             `json:"id"`
	Name         string             `json:"name"`
	License      json.RawMessage    `json:"license,omitempty"`
	Canvas       *Size              `json:"canvas,omitempty"`
	Window       *Size              `json:"window,omitempty"`
	Sprite       *Size              `json:"sprite,omitempty"`
	Crop         *Rect              `json:"crop,omitempty"`
	Clips        map[string]string  `json:"clips,omitempty"`
	BubbleAnchor *Anchor            `json:"bubbleAnchor,omitempty"`
	Hotzones     []SourceHotzone    `json:"hotzones,omitempty"`
	Menu         *[]SourceMenuEntry `json:"menu,omitempty"`
	IdleMotion   string             `json:"idleMotion,omitempty"`
}

// SourceHotzone is a clickable region in canvas pixel coordinates. Its
// response is embedded here so the source stays smaller than a runtime
// manifest; the builder creates the interaction and bubble key.
type SourceHotzone struct {
	ID            string       `json:"id"`
	Label         string       `json:"label,omitempty"`
	Rect          Rect         `json:"rect"`
	Steps         []SourceStep `json:"steps,omitempty"`
	Bubble        []string     `json:"bubble,omitempty"`
	Sound         bool         `json:"sound,omitempty"`
	Motion        string       `json:"motion,omitempty"`
	ReducedPoster string       `json:"reducedPoster,omitempty"`
}

// SourceStep is one finite action in a hotzone response.
type SourceStep struct {
	Action  string `json:"action"`
	DelayMs int    `json:"delayMs,omitempty"`
}

// SourceMenuEntry references a hotzone interaction or a one-shot action.
type SourceMenuEntry struct {
	Interaction string `json:"interaction,omitempty"`
	Action      string `json:"action,omitempty"`
}

// LoadSource reads and validates pack.source.json from an authoring
// directory. It does not scan artwork files.
func LoadSource(dir string) (Source, error) {
	path := filepath.Join(dir, sourceFileName)
	info, err := os.Stat(path)
	if err != nil {
		return Source{}, fmt.Errorf("petpack source: read %s: %w", path, err)
	}
	if info.Size() > maxSourceBytes {
		return Source{}, fmt.Errorf("petpack source: %s exceeds the 4 MiB limit", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Source{}, fmt.Errorf("petpack source: read %s: %w", path, err)
	}
	source, err := DecodeSource(data)
	if err != nil {
		return Source{}, fmt.Errorf("petpack source: parse %s: %w", path, err)
	}
	if err := validateSource(source); err != nil {
		return Source{}, err
	}
	return source, nil
}

// DecodeSource parses the strict authoring schema.
func DecodeSource(data []byte) (Source, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var source Source
	if err := decoder.Decode(&source); err != nil {
		return Source{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return Source{}, fmt.Errorf("source contains more than one JSON value")
		}
		return Source{}, err
	}
	return source, nil
}

func validateSource(source Source) error {
	if source.Source != 1 {
		return fmt.Errorf("petpack source: source %d, want 1", source.Source)
	}
	if !validPackDirID(source.ID) || IsReservedPackID(source.ID) {
		return fmt.Errorf("petpack source: invalid or reserved id %q", source.ID)
	}
	if strings.TrimSpace(source.Name) == "" {
		return fmt.Errorf("petpack source: name is required")
	}
	if source.Canvas != nil {
		if err := validateCanvasSize(source.ID, source.Canvas.Width, source.Canvas.Height); err != nil {
			return err
		}
	}
	if source.Window != nil {
		if source.Window.Width <= 0 || source.Window.Height <= 0 ||
			source.Window.Width > maxCanvasEdge || source.Window.Height > maxCanvasEdge {
			return fmt.Errorf("petpack source: invalid window %dx%d", source.Window.Width, source.Window.Height)
		}
	}
	if source.Sprite != nil {
		if source.Sprite.Width <= 0 || source.Sprite.Height <= 0 ||
			source.Sprite.Width > maxCanvasEdge || source.Sprite.Height > maxCanvasEdge {
			return fmt.Errorf("petpack source: invalid sprite %dx%d", source.Sprite.Width, source.Sprite.Height)
		}
	}
	if source.Window != nil && source.Sprite != nil &&
		(source.Sprite.Width > source.Window.Width || source.Sprite.Height > source.Window.Height) {
		return fmt.Errorf("petpack source: sprite exceeds window")
	}
	if source.Crop != nil {
		if source.Crop.Empty() {
			return fmt.Errorf("petpack source: crop must not be empty")
		}
		if source.Canvas != nil && !source.Crop.ImageRect().In(
			imageRect(source.Canvas.Width, source.Canvas.Height)) {
			return fmt.Errorf("petpack source: crop %v outside canvas", *source.Crop)
		}
	}
	if source.BubbleAnchor != nil &&
		(source.BubbleAnchor.X < 0 || source.BubbleAnchor.X > 1 ||
			source.BubbleAnchor.Y < 0 || source.BubbleAnchor.Y > 1) {
		return fmt.Errorf("petpack source: bubbleAnchor is not normalized")
	}
	if source.IdleMotion != "" && source.IdleMotion != "none" && source.IdleMotion != "bob" {
		return fmt.Errorf("petpack source: invalid idleMotion %q", source.IdleMotion)
	}
	if len(source.Hotzones) > maxSourceHotzones {
		return fmt.Errorf("petpack source: more than %d hotzones", maxSourceHotzones)
	}
	seen := make(map[string]bool, len(source.Hotzones))
	for i, zone := range source.Hotzones {
		if !validPackID(zone.ID) {
			return fmt.Errorf("petpack source: hotzone %d: invalid id %q", i, zone.ID)
		}
		if seen[zone.ID] {
			return fmt.Errorf("petpack source: duplicate hotzone id %q", zone.ID)
		}
		seen[zone.ID] = true
		if zone.Rect.Width <= 0 || zone.Rect.Height <= 0 {
			return fmt.Errorf("petpack source: hotzone %q: empty rect", zone.ID)
		}
		if len(zone.Steps) > 32 {
			return fmt.Errorf("petpack source: hotzone %q: more than 32 steps", zone.ID)
		}
		for j, step := range zone.Steps {
			if step.Action == "" {
				return fmt.Errorf("petpack source: hotzone %q step %d: empty action", zone.ID, j)
			}
			if step.DelayMs < 0 || step.DelayMs > 60000 {
				return fmt.Errorf("petpack source: hotzone %q step %d: delay %d outside 0..60000",
					zone.ID, j, step.DelayMs)
			}
		}
		if len(zone.Bubble) > 32 {
			return fmt.Errorf("petpack source: hotzone %q: more than 32 bubble lines", zone.ID)
		}
		for j, line := range zone.Bubble {
			if strings.TrimSpace(line) == "" {
				return fmt.Errorf("petpack source: hotzone %q bubble line %d: empty", zone.ID, j)
			}
			if utf8.RuneCountInString(line) > 200 {
				return fmt.Errorf("petpack source: hotzone %q bubble line %d: exceeds 200 characters",
					zone.ID, j)
			}
		}
		switch zone.Motion {
		case "", "none", "bounce", "lift":
		default:
			return fmt.Errorf("petpack source: hotzone %q: invalid motion %q", zone.ID, zone.Motion)
		}
		hasEffect := len(zone.Steps) > 0 || len(zone.Bubble) > 0 || zone.Sound ||
			(zone.Motion != "" && zone.Motion != "none")
		if !hasEffect {
			return fmt.Errorf("petpack source: hotzone %q has no effect", zone.ID)
		}
		hasMotion := zone.Motion != "" && zone.Motion != "none"
		if len(zone.Steps) == 0 && hasMotion && zone.ReducedPoster == "" {
			return fmt.Errorf("petpack source: hotzone %q: reducedPoster is required for motion-only feedback",
				zone.ID)
		}
		if zone.ReducedPoster != "" && zone.ReducedPoster != "auto" {
			if _, _, err := splitPoster(zone.ReducedPoster); err != nil {
				return fmt.Errorf("petpack source: hotzone %q reducedPoster: %w", zone.ID, err)
			}
		}
	}
	if source.Menu != nil && len(*source.Menu) > maxSourceMenu {
		return fmt.Errorf("petpack source: more than %d menu entries", maxSourceMenu)
	}
	for i, entry := range derefMenu(source.Menu) {
		set := 0
		if entry.Interaction != "" {
			set++
		}
		if entry.Action != "" {
			set++
		}
		if set != 1 {
			return fmt.Errorf("petpack source: menu entry %d must set exactly one of interaction or action", i)
		}
	}
	for id, name := range source.Clips {
		if !validPackID(id) {
			return fmt.Errorf("petpack source: invalid clip id %q", id)
		}
		if err := validateSourceClipPath(name); err != nil {
			return fmt.Errorf("petpack source: clip %q: %w", id, err)
		}
	}
	return nil
}

func derefMenu(menu *[]SourceMenuEntry) []SourceMenuEntry {
	if menu == nil {
		return nil
	}
	return *menu
}

func validateSourceClipPath(name string) error {
	if name == "" {
		return fmt.Errorf("empty path")
	}
	if filepath.IsAbs(name) {
		return fmt.Errorf("absolute path %q", name)
	}
	clean := filepath.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path escapes the source directory")
	}
	if !strings.EqualFold(filepath.Ext(clean), ".webp") {
		return fmt.Errorf("must be a .webp file")
	}
	return nil
}

func imageRect(width, height int) (r image.Rectangle) {
	return image.Rect(0, 0, width, height)
}
