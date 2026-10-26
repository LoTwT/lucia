package petpack

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const sourceClipsDir = "clips"

var sourceClipRoles = map[string]bool{
	"idle_loop":     true,
	"happy_enter":   true,
	"happy_loop":    true,
	"happy_exit":    true,
	"laugh_enter":   true,
	"laugh_loop":    true,
	"laugh_exit":    true,
	"listen_enter":  true,
	"listen_loop":   true,
	"listen_exit":   true,
	"speak_enter":   true,
	"speak_loop":    true,
	"speak_exit":    true,
	"think_enter":   true,
	"think_loop":    true,
	"think_exit":    true,
	"sad_enter":     true,
	"sad_loop":      true,
	"sad_exit":      true,
	"wave_enter":    true,
	"wave_loop":     true,
	"wave_exit":     true,
	"grab_enter":    true,
	"drag_loop":     true,
	"drop_exit":     true,
	"belly_taunt":   true,
	"pretend_deaf":  true,
	"smug_stumble":  true,
	"sleep_denial":  true,
	"play_innocent": true,
	"edge_peek":     true,
}

// BuildResult describes a generated source artifact.
type BuildResult struct {
	Path         string
	ID           string
	Canvas       Size
	Crop         Rect
	Clips        int
	Frames       int
	Actions      int
	Hotzones     int
	Interactions int
	Warnings     []string
}

type sourceClip struct {
	id     string
	path   string
	entry  ClipEntry
	bounds image.Rectangle
}

// Build validates a source directory and writes a deterministic pack zip.
// It never installs the pack or writes to the user data directory.
func Build(sourceDir, output string, force bool) (BuildResult, error) {
	sourceDir, err := filepath.Abs(sourceDir)
	if err != nil {
		return BuildResult{}, err
	}
	if output == "" {
		return BuildResult{}, fmt.Errorf("petpack build: output path is required")
	}
	output, err = filepath.Abs(output)
	if err != nil {
		return BuildResult{}, err
	}

	source, err := LoadSource(sourceDir)
	if err != nil {
		return BuildResult{}, err
	}
	clips, warnings, canvas, alphaBounds, err := scanSourceClips(sourceDir, source)
	if err != nil {
		return BuildResult{}, err
	}
	if source.Canvas != nil {
		if source.Canvas.Width != canvas.Width || source.Canvas.Height != canvas.Height {
			return BuildResult{}, fmt.Errorf("petpack build: canvas %dx%d, want %dx%d",
				canvas.Width, canvas.Height, source.Canvas.Width, source.Canvas.Height)
		}
	}
	crop := Rect{}
	if source.Crop != nil {
		crop = *source.Crop
		if !crop.ImageRect().In(image.Rect(0, 0, canvas.Width, canvas.Height)) {
			return BuildResult{}, fmt.Errorf("petpack build: crop %v outside canvas", crop)
		}
		if !alphaBounds.In(crop.ImageRect()) {
			warnings = append(warnings, fmt.Sprintf(
				"crop %v excludes opaque pixels at %v", crop, alphaBounds))
		}
	} else {
		union := alphaBounds.Inset(-2).Intersect(image.Rect(0, 0, canvas.Width, canvas.Height))
		crop = Rect{X: union.Min.X, Y: union.Min.Y, Width: union.Dx(), Height: union.Dy()}
		if len(source.Hotzones) > 0 {
			warnings = append(warnings, "hotzones use an automatically computed crop; pin crop to keep them stable")
		}
	}
	if source.License == nil {
		warnings = append(warnings, "license is missing; the pack is treated as local-only")
	}
	if len(source.Hotzones) == 0 {
		warnings = append(warnings, "no hotzones: the pet is not clickable")
	}

	ids := make(map[string]bool, len(clips))
	for id := range clips {
		ids[id] = true
	}
	actions := buildActionsFromIDs(ids)
	states := buildStates(actions)
	events := buildEvents(actions)
	menu := buildMenu(actions)
	if source.Menu != nil {
		menu = make([]MenuEntry, 0, len(*source.Menu))
		for i, entry := range *source.Menu {
			if entry.Interaction != "" {
				if _, ok := sourceHotzoneByID(source.Hotzones, entry.Interaction); !ok {
					return BuildResult{}, fmt.Errorf("petpack build: menu entry %d references missing hotzone %q",
						i, entry.Interaction)
				}
				menu = append(menu, MenuEntry{Interaction: entry.Interaction})
				continue
			}
			action, ok := actions[entry.Action]
			if !ok {
				return BuildResult{}, fmt.Errorf("petpack build: menu entry %d references missing action %q",
					i, entry.Action)
			}
			if err := validateFiniteAction(entry.Action, action); err != nil {
				return BuildResult{}, fmt.Errorf("petpack build: menu entry %d: %w", i, err)
			}
			menu = append(menu, MenuEntry{Action: entry.Action})
		}
	}

	hotzones := make([]Hotzone, 0, len(source.Hotzones))
	interactions := make(map[string]Interaction, len(source.Hotzones))
	bubbles := make(map[string][]string, len(source.Hotzones))
	for _, zone := range source.Hotzones {
		if !zone.Rect.ImageRect().In(crop.ImageRect()) {
			return BuildResult{}, fmt.Errorf("petpack build: hotzone %q is outside crop %v; pin crop or move the rect inside the alpha bounds",
				zone.ID, crop)
		}
		if _, ok := actions["idle"]; !ok {
			return BuildResult{}, fmt.Errorf("petpack build: missing idle action")
		}
		for i, step := range zone.Steps {
			action, ok := actions[step.Action]
			if !ok {
				return BuildResult{}, fmt.Errorf("petpack build: hotzone %q step %d references missing action %q",
					zone.ID, i, step.Action)
			}
			if err := validateFiniteAction(step.Action, action); err != nil {
				return BuildResult{}, fmt.Errorf("petpack build: hotzone %q step %d: %w",
					zone.ID, i, err)
			}
		}
		interaction := Interaction{
			Label:         zone.Label,
			Steps:         make([]Step, 0, len(zone.Steps)),
			Sound:         zone.Sound,
			Motion:        zone.Motion,
			ReducedPoster: zone.ReducedPoster,
		}
		for _, step := range zone.Steps {
			interaction.Steps = append(interaction.Steps, Step{Action: step.Action, DelayMs: step.DelayMs})
		}
		if len(zone.Bubble) > 0 {
			interaction.Bubble = zone.ID
			bubbles[zone.ID] = append([]string(nil), zone.Bubble...)
		}
		if interaction.ReducedPoster == "auto" {
			interaction.ReducedPoster = actions["idle"].Poster
			if interaction.ReducedPoster == "" {
				return BuildResult{}, fmt.Errorf("petpack build: idle action has no poster for reducedPoster auto")
			}
		}
		interactions[zone.ID] = interaction
		hotzones = append(hotzones, Hotzone{
			ID:    zone.ID,
			Label: zone.Label,
			Rect: NormalizedRect{
				X:      float64(zone.Rect.X-crop.X) / float64(crop.Width),
				Y:      float64(zone.Rect.Y-crop.Y) / float64(crop.Height),
				Width:  float64(zone.Rect.Width) / float64(crop.Width),
				Height: float64(zone.Rect.Height) / float64(crop.Height),
			},
			Interaction: zone.ID,
		})
	}

	base := Base{
		Window: Size{Width: 248, Height: 310},
		Sprite: Size{Width: 240, Height: 300},
		Margin: 4,
	}
	if source.Window != nil {
		base.Window = *source.Window
	}
	if source.Sprite != nil {
		base.Sprite = *source.Sprite
	}
	anchor := Anchor{X: 0.5, Y: 0.05}
	if source.BubbleAnchor != nil {
		anchor = *source.BubbleAnchor
	}
	seed := Manifest{
		Schema:          1,
		ID:              source.ID,
		Name:            source.Name,
		License:         source.License,
		Canvas:          canvas,
		Crop:            crop,
		Base:            base,
		BubbleAnchor:    anchor,
		BubbleAnchorSet: true,
		Actions:         actions,
		States:          states,
		Events:          events,
		Hotzones:        hotzones,
		Interactions:    interactions,
		Bubbles:         bubbles,
		Menu:            menu,
		IdleMotion:      source.IdleMotion,
	}

	tmp, err := os.MkdirTemp("", "lucia-pack-build-")
	if err != nil {
		return BuildResult{}, err
	}
	defer os.RemoveAll(tmp)
	seedZip := filepath.Join(tmp, "source.zip")
	if err := writeSeedZip(seedZip, seed, clips); err != nil {
		return BuildResult{}, err
	}
	packsRoot := filepath.Join(tmp, "packs")
	if err := os.MkdirAll(packsRoot, 0o755); err != nil {
		return BuildResult{}, err
	}
	pack, err := ImportZip(seedZip, packsRoot, ImportOptions{
		ID:              source.ID,
		Name:            source.Name,
		Base:            base,
		BubbleAnchor:    anchor,
		BubbleAnchorSet: true,
	})
	if err != nil {
		return BuildResult{}, err
	}
	sourceBytes, err := artifactSourceBytes(source, clips)
	if err != nil {
		return BuildResult{}, err
	}
	if err := os.WriteFile(filepath.Join(pack.Dir, sourceFileName), sourceBytes, 0o644); err != nil {
		return BuildResult{}, err
	}
	backup, err := writeDeterministicZip(pack.Dir, output, force)
	if err != nil {
		return BuildResult{}, err
	}
	if err := verifyBuiltZip(output, pack, source.ID); err != nil {
		removeErr := os.Remove(output)
		var restoreErr error
		if backup != "" {
			restoreErr = os.Rename(backup, output)
		}
		if removeErr != nil || restoreErr != nil {
			return BuildResult{}, fmt.Errorf("%w (restore previous output: remove=%v rename=%v, backup=%q)",
				err, removeErr, restoreErr, backup)
		}
		return BuildResult{}, err
	}
	if backup != "" {
		_ = os.Remove(backup)
	}
	result := BuildResult{
		Path:         output,
		ID:           source.ID,
		Canvas:       pack.Manifest.Canvas,
		Crop:         pack.Manifest.Crop,
		Clips:        len(pack.Clips),
		Actions:      len(pack.Manifest.Actions),
		Hotzones:     len(pack.Manifest.Hotzones),
		Interactions: len(pack.Manifest.Interactions),
		Warnings:     warnings,
	}
	for _, clip := range pack.Clips {
		result.Frames += clip.FrameCount()
	}
	return result, nil
}

// PreviewSource builds the source in a temporary location and writes one
// decoded frame as a PNG. It never installs the pack.
func PreviewSource(sourceDir, output, clipID string, frame int) (BuildResult, error) {
	if output == "" {
		return BuildResult{}, fmt.Errorf("petpack preview: output path is required")
	}
	if frame < 0 {
		return BuildResult{}, fmt.Errorf("petpack preview: frame must not be negative")
	}
	tmp, err := os.MkdirTemp("", "lucia-pack-preview-")
	if err != nil {
		return BuildResult{}, err
	}
	defer os.RemoveAll(tmp)
	zipPath := filepath.Join(tmp, "pack.zip")
	result, err := Build(sourceDir, zipPath, true)
	if err != nil {
		return BuildResult{}, err
	}
	packsRoot := filepath.Join(tmp, "packs")
	if err := os.MkdirAll(packsRoot, 0o755); err != nil {
		return BuildResult{}, err
	}
	if _, err := ImportZip(zipPath, packsRoot, ImportOptions{ID: result.ID}); err != nil {
		return BuildResult{}, err
	}
	pack, err := LoadDir(filepath.Join(packsRoot, result.ID), result.ID, 1)
	if err != nil {
		return BuildResult{}, err
	}
	posterClip := clipID
	if posterClip == "" {
		action, ok := pack.Action("idle")
		if !ok {
			return BuildResult{}, fmt.Errorf("petpack preview: pack has no idle action")
		}
		posterClip = action.Loop
		if posterClip == "" {
			posterClip, _, err = splitPoster(action.Poster)
			if err != nil {
				return BuildResult{}, fmt.Errorf("petpack preview: idle poster: %w", err)
			}
		}
	}
	clip, ok := pack.Clip(posterClip)
	if !ok {
		return BuildResult{}, fmt.Errorf("petpack preview: missing clip %q", posterClip)
	}
	if frame >= clip.FrameCount() {
		return BuildResult{}, fmt.Errorf("petpack preview: frame %d outside clip %q (%d frames)",
			frame, posterClip, clip.FrameCount())
	}
	decoder, err := clip.NewDecoder()
	if err != nil {
		return BuildResult{}, err
	}
	defer decoder.Close()
	var image image.Image
	for i := 0; i <= frame; i++ {
		image, _, err = decoder.Next()
		if err != nil {
			return BuildResult{}, fmt.Errorf("petpack preview: clip %q frame %d: %w", posterClip, i, err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return BuildResult{}, err
	}
	file, err := os.Create(output)
	if err != nil {
		return BuildResult{}, err
	}
	encodeErr := png.Encode(file, image)
	closeErr := file.Close()
	if encodeErr != nil {
		return BuildResult{}, encodeErr
	}
	if closeErr != nil {
		return BuildResult{}, closeErr
	}
	result.Canvas = pack.Manifest.Canvas
	result.Crop = pack.Manifest.Crop
	return result, nil
}

func scanSourceClips(sourceDir string, source Source) (map[string]sourceClip, []string, Size, image.Rectangle, error) {
	files := make([]sourceFile, 0)
	if source.Clips != nil {
		ids := make([]string, 0, len(source.Clips))
		for id := range source.Clips {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			if !sourceClipRoles[id] {
				return nil, nil, Size{}, image.Rectangle{},
					fmt.Errorf("petpack build: clip %q is not a supported role", id)
			}
			if windowsReservedPackID(id) {
				return nil, nil, Size{}, image.Rectangle{},
					fmt.Errorf("petpack build: clip id %q is a reserved Windows name", id)
			}
			files = append(files, sourceFile{id: id, rel: source.Clips[id]})
		}
	} else {
		clipsDir := filepath.Join(sourceDir, sourceClipsDir)
		err := filepath.WalkDir(clipsDir, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
				return fmt.Errorf("petpack build: clip %q is not a regular file", path)
			}
			rel, err := filepath.Rel(sourceDir, path)
			if err != nil {
				return err
			}
			relSlash := filepath.ToSlash(rel)
			if isMacMetadata(relSlash) || !strings.EqualFold(filepath.Ext(path), ".webp") {
				return nil
			}
			id, err := clipIDFromName(relSlash)
			if err != nil {
				return fmt.Errorf("petpack build: %q: %w", rel, err)
			}
			if windowsReservedPackID(id) {
				return fmt.Errorf("petpack build: clip id %q is a reserved Windows name", id)
			}
			files = append(files, sourceFile{id: id, rel: rel})
			return nil
		})
		if err != nil {
			if os.IsNotExist(err) {
				return nil, nil, Size{}, image.Rectangle{},
					fmt.Errorf("petpack build: missing %s directory", sourceClipsDir)
			}
			return nil, nil, Size{}, image.Rectangle{}, err
		}
		sort.Slice(files, func(i, j int) bool { return files[i].id < files[j].id })
	}

	clips := make(map[string]sourceClip, len(files))
	var canvas Size
	var bounds image.Rectangle
	var total int64
	warnings := []string{}
	for _, file := range files {
		if _, exists := clips[file.id]; exists {
			return nil, nil, Size{}, image.Rectangle{},
				fmt.Errorf("petpack build: duplicate clip id %q", file.id)
		}
		full, err := sourceClipPath(sourceDir, file.rel)
		if err != nil {
			return nil, nil, Size{}, image.Rectangle{}, fmt.Errorf("petpack build: clip %q: %w", file.id, err)
		}
		info, err := os.Stat(full)
		if err != nil {
			return nil, nil, Size{}, image.Rectangle{}, fmt.Errorf("petpack build: clip %q: %w", file.id, err)
		}
		if info.Size() > maxImportFile {
			return nil, nil, Size{}, image.Rectangle{},
				fmt.Errorf("petpack build: clip %q exceeds the per-file limit", file.id)
		}
		total += info.Size()
		if total > maxImportTotal {
			return nil, nil, Size{}, image.Rectangle{},
				fmt.Errorf("petpack build: clips exceed the total limit")
		}
		data, err := os.ReadFile(full)
		if err != nil {
			return nil, nil, Size{}, image.Rectangle{}, fmt.Errorf("petpack build: clip %q: %w", file.id, err)
		}
		clip, err := ParseClip(file.id, data)
		if err != nil {
			return nil, nil, Size{}, image.Rectangle{}, fmt.Errorf("petpack build: clip %q: %w", file.id, err)
		}
		if canvas.Width == 0 {
			canvas = Size{Width: clip.Width, Height: clip.Height}
		} else if canvas.Width != clip.Width || canvas.Height != clip.Height {
			return nil, nil, Size{}, image.Rectangle{},
				fmt.Errorf("petpack build: clip %q has canvas %dx%d, want %dx%d",
					file.id, clip.Width, clip.Height, canvas.Width, canvas.Height)
		}
		clipBounds, err := clip.AlphaBounds()
		if err != nil {
			return nil, nil, Size{}, image.Rectangle{}, fmt.Errorf("petpack build: clip %q: %w", file.id, err)
		}
		if clipBounds.Empty() {
			return nil, nil, Size{}, image.Rectangle{},
				fmt.Errorf("petpack build: clip %q is fully transparent", file.id)
		}
		if bounds.Empty() {
			bounds = clipBounds
		} else {
			bounds = bounds.Union(clipBounds)
		}
		if !sourceClipRoles[file.id] {
			warnings = append(warnings, fmt.Sprintf("clip %q has no generated action", file.id))
		}
		clips[file.id] = sourceClip{
			id:   file.id,
			path: full,
			entry: ClipEntry{File: filepath.ToSlash(filepath.Join(sourceClipsDir, file.id+".webp")),
				Frames: clip.FrameCount()},
			bounds: clipBounds,
		}
	}
	if len(clips) == 0 {
		return nil, nil, Size{}, image.Rectangle{}, fmt.Errorf("petpack build: no WebP clips found")
	}
	if _, ok := clips["idle_loop"]; !ok {
		return nil, nil, Size{}, image.Rectangle{}, fmt.Errorf("petpack build: missing idle_loop clip")
	}
	return clips, warnings, canvas, bounds, nil
}

type sourceFile struct {
	id  string
	rel string
}

func sourceClipPath(root, rel string) (string, error) {
	if err := validateSourceClipPath(rel); err != nil {
		return "", err
	}
	clean := filepath.Clean(filepath.FromSlash(rel))
	full := filepath.Join(root, clean)
	within, err := filepath.Rel(root, full)
	if err != nil {
		return "", err
	}
	if within == ".." || strings.HasPrefix(within, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes the source directory")
	}
	current := root
	for _, part := range strings.Split(clean, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("symlink %q is not allowed", current)
		}
	}
	info, err := os.Lstat(full)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", fmt.Errorf("not a regular file")
	}
	return full, nil
}

func artifactSourceBytes(source Source, clips map[string]sourceClip) ([]byte, error) {
	if source.Clips != nil {
		source.Clips = make(map[string]string, len(clips))
		for id := range clips {
			source.Clips[id] = sourceClipsDir + "/" + id + ".webp"
		}
	}
	data, err := json.MarshalIndent(source, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func sourceHotzoneByID(zones []SourceHotzone, id string) (SourceHotzone, bool) {
	for _, zone := range zones {
		if zone.ID == id {
			return zone, true
		}
	}
	return SourceHotzone{}, false
}

func writeSeedZip(path string, manifest Manifest, clips map[string]sourceClip) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	writer := zip.NewWriter(file)
	writeErr := func() error {
		data, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			return err
		}
		data = append(data, '\n')
		header := &zip.FileHeader{Name: "pack.json", Method: zip.Store}
		header.SetMode(0o644)
		entry, err := writer.CreateHeader(header)
		if err != nil {
			return err
		}
		if _, err := entry.Write(data); err != nil {
			return err
		}
		ids := make([]string, 0, len(clips))
		for id := range clips {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			header := &zip.FileHeader{Name: "clips/" + id + ".webp", Method: zip.Store}
			header.SetMode(0o644)
			entry, err := writer.CreateHeader(header)
			if err != nil {
				return err
			}
			source, err := os.Open(clips[id].path)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(entry, source)
			closeErr := source.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		}
		return nil
	}()
	closeWriterErr := writer.Close()
	closeFileErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeWriterErr != nil {
		return closeWriterErr
	}
	return closeFileErr
}

func writeDeterministicZip(dir, output string, force bool) (backup string, err error) {
	if !force {
		if _, err := os.Stat(output); err == nil {
			return "", fmt.Errorf("petpack build: output %q already exists (use -force to replace it)", output)
		} else if !os.IsNotExist(err) {
			return "", err
		}
	}
	parent := filepath.Dir(output)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", err
	}
	temp, err := os.CreateTemp(parent, ".petpack-build-*.zip")
	if err != nil {
		return "", err
	}
	tempPath := temp.Name()
	keep := false
	defer func() {
		if !keep {
			_ = os.Remove(tempPath)
		}
	}()
	writer := zip.NewWriter(temp)
	walkErr := func() error {
		return filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
				return fmt.Errorf("petpack build: %q is not a regular file", path)
			}
			rel, err := filepath.Rel(dir, path)
			if err != nil {
				return err
			}
			header := &zip.FileHeader{Name: filepath.ToSlash(rel), Method: zip.Store}
			header.Modified = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)
			header.SetMode(0o644)
			entryWriter, err := writer.CreateHeader(header)
			if err != nil {
				return err
			}
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(entryWriter, file)
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			return closeErr
		})
	}()
	closeErr := writer.Close()
	fileCloseErr := temp.Close()
	if walkErr != nil {
		return "", walkErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	if fileCloseErr != nil {
		return "", fileCloseErr
	}
	if err := os.Chmod(tempPath, 0o644); err != nil {
		return "", err
	}
	if force {
		if _, err := os.Stat(output); err == nil {
			backupFile, err := os.CreateTemp(parent, ".petpack-bak-*")
			if err != nil {
				return "", err
			}
			backup = backupFile.Name()
			if err := backupFile.Close(); err != nil {
				return "", err
			}
			if err := os.Remove(backup); err != nil {
				return "", err
			}
			if err := os.Rename(output, backup); err != nil {
				return "", err
			}
		} else if !os.IsNotExist(err) {
			return "", err
		}
	}
	if err := os.Rename(tempPath, output); err != nil {
		if backup != "" {
			if restoreErr := os.Rename(backup, output); restoreErr != nil {
				return "", fmt.Errorf("%w (restore previous output: %v, backup=%q)", err, restoreErr, backup)
			}
		}
		return "", err
	}
	keep = true
	return backup, nil
}

func verifyBuiltZip(path string, pack *Pack, id string) error {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer reader.Close()
	var manifestData []byte
	for _, file := range reader.File {
		if file.Name == "pack.json" {
			entry, err := file.Open()
			if err != nil {
				return err
			}
			manifestData, err = io.ReadAll(entry)
			closeErr := entry.Close()
			if err != nil {
				return err
			}
			if closeErr != nil {
				return closeErr
			}
			break
		}
	}
	if len(manifestData) == 0 {
		return fmt.Errorf("petpack build: generated zip has no pack.json")
	}
	expected, err := os.ReadFile(filepath.Join(pack.Dir, "pack.json"))
	if err != nil {
		return err
	}
	if !bytes.Equal(manifestData, expected) {
		return fmt.Errorf("petpack build: zip manifest differs from validated staging")
	}
	roundtrip, err := os.MkdirTemp("", "lucia-pack-verify-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(roundtrip)
	packsRoot := filepath.Join(roundtrip, "packs")
	if err := os.MkdirAll(packsRoot, 0o755); err != nil {
		return err
	}
	if _, err := ImportZip(path, packsRoot, ImportOptions{ID: id}); err != nil {
		return fmt.Errorf("petpack build: roundtrip import: %w", err)
	}
	installed, err := os.ReadFile(filepath.Join(packsRoot, id, "pack.json"))
	if err != nil {
		return err
	}
	if !bytes.Equal(installed, expected) {
		return fmt.Errorf("petpack build: installed manifest differs from staging")
	}
	return nil
}
