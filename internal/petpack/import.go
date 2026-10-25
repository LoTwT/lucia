package petpack

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
)

const (
	maxImportFile  = 64 << 20
	maxImportTotal = 512 << 20
)

type importedClip struct {
	clip  *Clip
	entry ClipEntry
}

// ImportOptions controls the metadata written by an import. Base is a
// presentation choice; the clips and crop come from the zip. A pack may
// carry its own pack.json, which supplies any field left zero here.
type ImportOptions struct {
	ID           string
	Name         string
	Base         Base
	BubbleAnchor Anchor
	Force        bool
	// OnStage reports the import's current stage for crash recovery. It is
	// called synchronously from ImportZip.
	OnStage func(string)
}

// ImportZip converts an animated-WebP zip into packs/<id> and returns the
// validated pack. The zip is never copied wholesale; only .webp clips are
// extracted. A root pack.json or pack.meta.json supplies id, name, geometry,
// actions, hotzones and presets; the ClipEntry list is rebuilt from the
// extracted files.
func ImportZip(zipPath, packsRoot string, opts ImportOptions) (*Pack, error) {
	stage := func(name string) {
		if opts.OnStage != nil {
			opts.OnStage(name)
		}
	}
	stage("reading")
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("petpack import: open zip: %w", err)
	}
	defer reader.Close()
	baseManifest, hasManifest, err := readZipManifest(reader.File)
	if err != nil {
		return nil, fmt.Errorf("petpack import: read manifest: %w", err)
	}
	if opts.ID == "" {
		opts.ID = baseManifest.ID
	}
	if opts.ID == "" {
		return nil, fmt.Errorf("petpack import: empty pack id")
	}
	if !validPackDirID(opts.ID) {
		return nil, fmt.Errorf("petpack import: invalid pack id %q", opts.ID)
	}
	if IsReservedPackID(opts.ID) {
		return nil, fmt.Errorf("petpack import: %q is reserved for the built-in pack", opts.ID)
	}
	if opts.Name == "" {
		opts.Name = baseManifest.Name
	}
	if opts.Name == "" {
		opts.Name = opts.ID
	}
	if opts.BubbleAnchor.X == 0 && opts.BubbleAnchor.Y == 0 {
		opts.BubbleAnchor = baseManifest.BubbleAnchor
	}
	if opts.BubbleAnchor.X == 0 && opts.BubbleAnchor.Y == 0 {
		opts.BubbleAnchor = Anchor{X: 0.5, Y: 0.05}
	}
	if err := os.MkdirAll(packsRoot, 0o755); err != nil {
		return nil, fmt.Errorf("petpack import: create packs directory: %w", err)
	}
	stage("extracting")

	tmp, err := os.MkdirTemp(packsRoot, ".import-"+opts.ID+"-")
	if err != nil {
		return nil, fmt.Errorf("petpack import: create temporary pack: %w", err)
	}
	defer os.RemoveAll(tmp)
	clipsDir := filepath.Join(tmp, "clips")
	if err := os.MkdirAll(clipsDir, 0o755); err != nil {
		return nil, fmt.Errorf("petpack import: create clips directory: %w", err)
	}

	imported := make(map[string]importedClip)
	var canvas Size
	var declaredTotal, actualTotal int64
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		name := zipEntryName(file)
		if isMacMetadata(name) {
			continue
		}
		if !strings.EqualFold(path.Ext(name), ".webp") {
			continue
		}
		clipID, err := clipIDFromName(name)
		if err != nil {
			return nil, fmt.Errorf("petpack import: %q: %w", name, err)
		}
		if _, exists := imported[clipID]; exists {
			return nil, fmt.Errorf("petpack import: duplicate clip %q", clipID)
		}
		if file.UncompressedSize64 > maxImportFile {
			return nil, fmt.Errorf("petpack import: %q exceeds the per-file limit", name)
		}
		declaredTotal += int64(file.UncompressedSize64)
		if declaredTotal > maxImportTotal {
			return nil, fmt.Errorf("petpack import: archive exceeds the total limit")
		}
		dest := filepath.Join(clipsDir, clipID+".webp")
		entry, data, err := extractZipFile(file, dest)
		if err != nil {
			return nil, fmt.Errorf("petpack import: %q: %w", name, err)
		}
		actualTotal += int64(len(data))
		if actualTotal > maxImportTotal {
			return nil, fmt.Errorf("petpack import: extracted data exceeds the total limit")
		}
		clip, err := ParseClip(clipID, data)
		if err != nil {
			return nil, fmt.Errorf("petpack import: %q: %w", name, err)
		}
		if canvas.Width == 0 {
			canvas = Size{Width: clip.Width, Height: clip.Height}
		} else if canvas.Width != clip.Width || canvas.Height != clip.Height {
			return nil, fmt.Errorf("petpack import: %q has canvas %dx%d, want %dx%d",
				name, clip.Width, clip.Height, canvas.Width, canvas.Height)
		}
		entry.Frames = clip.FrameCount()
		entry.File = "clips/" + clipID + ".webp"
		imported[clipID] = importedClip{clip: clip, entry: entry}
	}
	if len(imported) == 0 {
		return nil, fmt.Errorf("petpack import: no .webp clips found")
	}
	if _, ok := imported["idle_loop"]; !ok {
		return nil, fmt.Errorf("petpack import: missing idle_loop clip")
	}

	crop := image.Rectangle{}
	for _, item := range imported {
		bounds, err := item.clip.AlphaBounds()
		if err != nil {
			return nil, fmt.Errorf("petpack import: clip %q: %w", item.clip.ID, err)
		}
		if bounds.Empty() {
			continue
		}
		if crop.Empty() {
			crop = bounds
		} else {
			crop = crop.Union(bounds)
		}
	}
	if crop.Empty() {
		crop = image.Rect(0, 0, canvas.Width, canvas.Height)
	}
	crop = crop.Inset(-2).Intersect(image.Rect(0, 0, canvas.Width, canvas.Height))

	base := opts.Base
	if base.Window.Width == 0 && base.Window.Height == 0 {
		base = baseManifest.Base
	}
	if base.Window.Width <= 0 || base.Window.Height <= 0 ||
		base.Sprite.Width <= 0 || base.Sprite.Height <= 0 {
		return nil, fmt.Errorf("petpack import: base window and sprite sizes are required")
	}
	actions := baseManifest.Actions
	if len(actions) == 0 {
		actions = buildActions(imported)
	}
	states := baseManifest.States
	if states.Idle == "" {
		states = buildStates(actions)
	}
	events := baseManifest.Events
	menu := baseManifest.Menu
	if !hasManifest {
		events = buildEvents(actions)
		menu = buildMenu(actions)
	}
	manifest := Manifest{
		Schema:  1,
		ID:      opts.ID,
		Name:    opts.Name,
		License: baseManifest.License,
		Canvas:  canvas,
		Crop: Rect{
			X: crop.Min.X, Y: crop.Min.Y, Width: crop.Dx(), Height: crop.Dy(),
		},
		Base:         base,
		BubbleAnchor: opts.BubbleAnchor,
		Clips:        make(map[string]ClipEntry, len(imported)),
		Actions:      actions,
		States:       states,
		Events:       events,
		Hotzones:     baseManifest.Hotzones,
		Interactions: baseManifest.Interactions,
		Bubbles:      baseManifest.Bubbles,
		Menu:         menu,
		IdleMotion:   baseManifest.IdleMotion,
	}
	for id, item := range imported {
		entry := item.entry
		if old, ok := baseManifest.Clips[id]; ok {
			entry.Loop = old.Loop
		}
		manifest.Clips[id] = entry
	}
	if err := validateManifest(opts.ID, manifest); err != nil {
		return nil, fmt.Errorf("petpack import: %w", err)
	}
	stage("installing")
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("petpack import: encode manifest: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(tmp, "pack.json"), data, 0o644); err != nil {
		return nil, fmt.Errorf("petpack import: write manifest: %w", err)
	}

	final := filepath.Join(packsRoot, opts.ID)
	if filepath.Dir(final) != filepath.Clean(packsRoot) {
		return nil, fmt.Errorf("petpack import: unsafe destination %q", final)
	}
	backup := ""
	if _, err := os.Stat(final); err == nil {
		if !opts.Force {
			return nil, fmt.Errorf("petpack import: %q already exists (use -force to replace it)", final)
		}
		backup = final + ".backup"
		if err := os.RemoveAll(backup); err != nil {
			return nil, fmt.Errorf("petpack import: clear previous backup: %w", err)
		}
		if err := os.Rename(final, backup); err != nil {
			return nil, fmt.Errorf("petpack import: back up %q: %w", final, err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("petpack import: inspect %q: %w", final, err)
	}
	restore := func() error {
		var errs []error
		if err := os.RemoveAll(final); err != nil {
			errs = append(errs, fmt.Errorf("remove %q: %w", final, err))
		}
		if backup != "" {
			if err := os.Rename(backup, final); err != nil {
				errs = append(errs, fmt.Errorf("restore %q: %w", backup, err))
			}
		}
		return errors.Join(errs...)
	}
	if err := os.Rename(tmp, final); err != nil {
		if rollbackErr := restore(); rollbackErr != nil {
			return nil, fmt.Errorf("petpack import: install %q: %w; rollback failed: %v", final, err, rollbackErr)
		}
		return nil, fmt.Errorf("petpack import: install %q: %w", final, err)
	}
	// The deferred cleanup now finds nothing at tmp; final is validated below.
	stage("validating")
	pack, err := LoadDir(final, opts.ID, 1)
	if err != nil {
		if rollbackErr := restore(); rollbackErr != nil {
			return nil, fmt.Errorf("petpack import: validate installed pack: %w; rollback failed: %v", err, rollbackErr)
		}
		return nil, fmt.Errorf("petpack import: validate installed pack: %w", err)
	}
	if backup != "" {
		_ = os.RemoveAll(backup)
	}
	return pack, nil
}

func extractZipFile(file *zip.File, dest string) (ClipEntry, []byte, error) {
	reader, err := file.Open()
	if err != nil {
		return ClipEntry{}, nil, err
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, maxImportFile+1))
	if err != nil {
		return ClipEntry{}, nil, err
	}
	if len(data) > maxImportFile {
		return ClipEntry{}, nil, fmt.Errorf("file exceeds the per-file limit")
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return ClipEntry{}, nil, err
	}
	sum := sha256.Sum256(data)
	return ClipEntry{SHA256: hex.EncodeToString(sum[:])}, data, nil
}

// zipEntryName prefers the 0x7075 Unicode Path extra field. Go's archive/zip
// leaves GBK names as raw bytes, so the fallback decodes them explicitly.
func zipEntryName(file *zip.File) string {
	if name, ok := unicodePath(file.Extra, file.Name); ok {
		return name
	}
	if utf8.ValidString(file.Name) {
		return file.Name
	}
	decoded, err := simplifiedchinese.GBK.NewDecoder().String(file.Name)
	if err == nil && decoded != "" {
		return decoded
	}
	return file.Name
}

// isMacMetadata skips the resource-fork entries Finder adds when it creates a
// zip. They end in .webp too, but are not artwork.
func isMacMetadata(name string) bool {
	name = filepath.ToSlash(name)
	return strings.HasPrefix(name, "__MACOSX/") ||
		strings.HasPrefix(path.Base(name), "._")
}

func unicodePath(extra []byte, original string) (string, bool) {
	for len(extra) >= 4 {
		id := binary.LittleEndian.Uint16(extra[0:2])
		size := int(binary.LittleEndian.Uint16(extra[2:4]))
		if len(extra) < 4+size {
			return "", false
		}
		data := extra[4 : 4+size]
		if id == 0x7075 && len(data) >= 5 && data[0] == 1 {
			if binary.LittleEndian.Uint32(data[1:5]) != crc32.ChecksumIEEE([]byte(original)) {
				extra = extra[4+size:]
				continue
			}
			name := string(data[5:])
			if utf8.ValidString(name) {
				return name, true
			}
		}
		extra = extra[4+size:]
	}
	return "", false
}

// clipIDFromName uses the ASCII prefix before the localized suffix. It also
// strips the NN_ ordering prefix used by the special actions.
func clipIDFromName(name string) (string, error) {
	base := strings.TrimSuffix(path.Base(name), path.Ext(name))
	cut := len(base)
	for i, r := range base {
		if r > 127 {
			cut = i
			break
		}
	}
	id := strings.TrimRight(base[:cut], "_ ")
	if len(id) >= 3 && id[0] >= '0' && id[0] <= '9' &&
		id[1] >= '0' && id[1] <= '9' && id[2] == '_' {
		id = id[3:]
	}
	if id == "" {
		return "", fmt.Errorf("no ASCII clip id before the localized suffix")
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '_' {
			return "", fmt.Errorf("unsupported clip id %q", id)
		}
	}
	return id, nil
}

func buildActions(imported map[string]importedClip) map[string]Action {
	has := func(id string) bool {
		_, ok := imported[id]
		return ok
	}
	actions := make(map[string]Action)
	addChain := func(id, label string, loopCount int, interruptible bool) {
		enter, loop, exit := id+"_enter", id+"_loop", id+"_exit"
		action := Action{Label: label, Interruptible: interruptible}
		if has(enter) {
			action.Enter = enter
		}
		if has(loop) {
			action.Loop = loop
			action.Poster = loop + "#0"
		}
		if has(exit) {
			action.Exit = exit
		}
		if action.Poster == "" && action.Enter != "" {
			action.Poster = action.Enter + "#0"
		}
		if action.Enter == "" && action.Loop == "" && action.Exit == "" {
			return
		}
		if loopCount > 0 {
			action.LoopCount = loopCount
		}
		actions[id] = action
	}
	if has("idle_loop") {
		actions["idle"] = Action{Label: "待机", Loop: "idle_loop", Poster: "idle_loop#0", Interruptible: true, LoopCount: -1}
	}
	addChain("happy", "开心", 0, true)
	addChain("laugh", "大笑", 0, true)
	addChain("listen", "倾听", 1, false)
	addChain("speak", "说话", 1, false)
	addChain("think", "思考", 1, false)
	addChain("sad", "难过", 1, false)
	addChain("wave", "挥手", 1, false)
	if has("grab_enter") && has("drag_loop") && has("drop_exit") {
		actions["drag"] = Action{
			Label:     "拖动",
			Enter:     "grab_enter",
			Loop:      "drag_loop",
			Exit:      "drop_exit",
			Poster:    "drag_loop#0",
			LoopCount: -1,
		}
	}
	specials := []struct {
		id    string
		label string
	}{
		{"belly_taunt", "肚肚挑衅"},
		{"pretend_deaf", "假装没听见"},
		{"smug_stumble", "得意过头"},
		{"sleep_denial", "困到点头"},
		{"play_innocent", "装无辜"},
		{"edge_peek", "偷偷探头"},
	}
	for _, special := range specials {
		if !has(special.id) {
			continue
		}
		actions[special.id] = Action{
			Label:  special.label,
			Once:   special.id,
			Poster: special.id + "#0",
		}
	}
	return actions
}

func buildStates(actions map[string]Action) States {
	states := States{Idle: "idle"}
	if _, ok := actions["drag"]; ok {
		states.Drag = "drag"
	}
	if _, ok := actions["sleep_denial"]; ok {
		states.Drowsy = "sleep_denial"
	}
	return states
}

func buildEvents(actions map[string]Action) Events {
	events := Events{}
	if _, ok := actions["wave"]; ok {
		events.Intro = "wave"
		events.Activate = "wave"
		events.Reset = "wave"
	}
	if _, ok := actions["think"]; ok {
		events.UpdateChecking = "think"
	}
	if _, ok := actions["sad"]; ok {
		events.UpdateFailed = "sad"
	}
	return events
}

func buildMenu(actions map[string]Action) []MenuEntry {
	order := []string{"belly_taunt", "pretend_deaf", "smug_stumble", "sleep_denial", "play_innocent", "edge_peek"}
	var entries []MenuEntry
	seen := make(map[string]bool, len(order))
	for _, id := range order {
		action, ok := actions[id]
		if ok && action.Once != "" && action.Label != "" {
			entries = append(entries, MenuEntry{Action: id})
			seen[id] = true
		}
	}
	var rest []string
	for id, action := range actions {
		if seen[id] || action.Once == "" || action.Label == "" {
			continue
		}
		rest = append(rest, id)
	}
	sort.Strings(rest)
	for _, id := range rest {
		entries = append(entries, MenuEntry{Action: id})
	}
	return entries
}

// ReadZipManifest inspects a pack zip without extracting artwork. It returns
// the manifest as authored so the importer can build actions first and then
// fill the generated states, events and menu from them.
func ReadZipManifest(zipPath string) (Manifest, bool, error) {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return Manifest{}, false, fmt.Errorf("petpack: open zip: %w", err)
	}
	defer reader.Close()
	return readZipManifest(reader.File)
}

func readZipManifest(files []*zip.File) (Manifest, bool, error) {
	entry := findManifestEntry(files, "pack.json")
	if entry == nil {
		entry = findManifestEntry(files, "pack.meta.json")
	}
	if entry == nil {
		return Manifest{}, false, nil
	}
	const limit = 4 << 20
	if entry.UncompressedSize64 > limit {
		return Manifest{}, true, fmt.Errorf("%s exceeds the 4 MiB manifest limit", entry.Name)
	}
	reader, err := entry.Open()
	if err != nil {
		return Manifest{}, true, fmt.Errorf("open %s: %w", entry.Name, err)
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return Manifest{}, true, fmt.Errorf("read %s: %w", entry.Name, err)
	}
	if len(data) > limit {
		return Manifest{}, true, fmt.Errorf("%s exceeds the 4 MiB manifest limit", entry.Name)
	}
	manifest, err := decodeManifest(data)
	if err != nil {
		return Manifest{}, true, fmt.Errorf("parse %s: %w", entry.Name, err)
	}
	if manifest.Schema != 1 {
		return Manifest{}, true, fmt.Errorf("%s: schema %d, want 1", entry.Name, manifest.Schema)
	}
	return manifest, true, nil
}

func findManifestEntry(files []*zip.File, name string) *zip.File {
	var best *zip.File
	bestDepth := 1 << 30
	for _, file := range files {
		if file.FileInfo().IsDir() || isMacMetadata(file.Name) {
			continue
		}
		clean := path.Clean(zipEntryName(file))
		if clean != name && !strings.HasSuffix(clean, "/"+name) {
			continue
		}
		depth := strings.Count(clean, "/")
		if best == nil || depth < bestDepth {
			best = file
			bestDepth = depth
		}
	}
	return best
}
