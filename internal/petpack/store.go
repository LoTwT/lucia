package petpack

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// LockFile records the installed pack metadata and the active id.
	LockFile = "packs.lock.json"
	// PendingFile records an import in flight so a crash can be recovered.
	PendingFile = "packs.pending.json"
)

// Summary is the console-facing description of one pack. Preview is loaded
// separately because decoding a poster is comparatively expensive.
type Summary struct {
	ID              string
	Name            string
	Active          bool
	Builtin         bool
	Valid           bool
	Error           string
	Clips           int
	Frames          int
	Hotzones        []HotzoneEntry
	Interactions    int
	InteractionList []InteractionDetail
	ActionList      []ActionDetail
	License         string
	InstalledAt     time.Time
}

// InteractionDetail is the console-facing description of one interaction.
type InteractionDetail struct {
	ID     string
	Label  string
	Bubble string
	Motion string
	Steps  []Step
}

// ActionDetail exposes the authored timing without decoding a clip.
type ActionDetail struct {
	ID        string
	Label     string
	LoopCount int
	Timing    Timing
}

type lockEntry struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Clips        int       `json:"clips"`
	Frames       int       `json:"frames"`
	Hotzones     int       `json:"hotzones"`
	Interactions int       `json:"interactions"`
	License      string    `json:"license,omitempty"`
	Digest       string    `json:"digest,omitempty"`
	InstalledAt  time.Time `json:"installedAt"`
}

// PackExistsError reports that Install found an installed pack with the same
// id. Nothing has been written when this error is returned.
type PackExistsError struct {
	ID   string
	Name string
}

func (e *PackExistsError) Error() string {
	return fmt.Sprintf("素材包 %q 已存在", e.ID)
}

type lockfile struct {
	Schema int                  `json:"schema"`
	Active string               `json:"active,omitempty"`
	Packs  map[string]lockEntry `json:"packs"`
}

type pendingFile struct {
	Operation string    `json:"operation"`
	ID        string    `json:"id,omitempty"`
	Stage     string    `json:"stage,omitempty"`
	Backup    string    `json:"backup,omitempty"`
	Error     string    `json:"error,omitempty"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Store owns pack installation and the lockfile. All mutating operations are
// serialized; the player only sees complete directories because ImportZip
// installs through a temporary directory and rename.
type Store struct {
	mu      sync.Mutex
	dataDir string
	active  string
}

// NewStore returns a store rooted at dataDir. The active id is normally
// settings.PetPack; callers should call Recover before loading the player.
func NewStore(dataDir, active string) *Store {
	if IsReservedPackID(active) {
		active = ""
	}
	return &Store{dataDir: dataDir, active: active}
}

func (s *Store) PacksDir() string { return filepath.Join(s.dataDir, "packs") }

// Active returns the selected pack id, or "" for the built-in pet.
func (s *Store) Active() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active
}

// SetActive records the selection in the lockfile. It does not write
// settings.json; the application keeps that as the user-facing source of
// truth and updates both through its activation callback.
func (s *Store) SetActive(id string) error {
	if IsReservedPackID(id) {
		id = ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := s.loadLocked()
	if err != nil {
		return err
	}
	lock.Active = id
	if err := s.saveLockLocked(lock); err != nil {
		return err
	}
	s.active = id
	return nil
}

// Recover cleans interrupted imports and rebuilds the lockfile from the
// directories that are actually installed.
func (s *Store) Recover() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.PacksDir(), 0o755); err != nil {
		return err
	}
	pending, pendingErr := s.readPendingLocked()
	if pendingErr != nil {
		// A corrupt pending record cannot identify a recoverable backup;
		// keep it for diagnosis and let the directory scan repair the rest.
		_ = os.Rename(s.pendingPath(), s.pendingPath()+".bad")
	} else if pending != nil {
		s.recoverPendingLocked(pending)
		if err := os.Remove(s.pendingPath()); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	entries, err := os.ReadDir(s.PacksDir())
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		path := filepath.Join(s.PacksDir(), name)
		switch {
		case strings.HasPrefix(name, ".import-"), strings.HasPrefix(name, ".trash-"):
			_ = os.RemoveAll(path)
		case strings.HasSuffix(name, ".backup"):
			base := strings.TrimSuffix(name, ".backup")
			basePath := filepath.Join(s.PacksDir(), base)
			if _, err := LoadDir(basePath, base, 1); err == nil {
				_ = os.RemoveAll(path)
			} else {
				_ = os.RemoveAll(basePath)
				_ = os.Rename(path, basePath)
			}
		}
	}
	lock, err := s.loadLocked()
	if err != nil {
		return err
	}
	lock.Active = s.active
	return s.refreshLockLocked(lock)
}

// List returns every installed directory, including invalid packs. Invalid
// packs are shown rather than hidden so the user can delete or replace them.
func (s *Store) List() ([]Summary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := s.loadLocked()
	if err != nil {
		return nil, err
	}
	lock.Active = s.active
	if err := s.refreshLockLocked(lock); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.PacksDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	summaries := make([]Summary, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") ||
			strings.HasSuffix(entry.Name(), ".backup") {
			continue
		}
		id := entry.Name()
		if IsReservedPackID(id) {
			continue
		}
		manifest, err := ReadManifest(filepath.Join(s.PacksDir(), id), id)
		if err != nil {
			summaries = append(summaries, Summary{
				ID: id, Active: id == s.active, Error: err.Error(),
			})
			continue
		}
		summary := summaryFromManifest(manifest, id == s.active)
		if summary.ID == "" {
			summary.ID = id
		}
		if entry, ok := lock.Packs[id]; ok {
			summary.InstalledAt = entry.InstalledAt
		}
		summaries = append(summaries, summary)
	}
	if s.active != "" && !containsSummary(summaries, s.active) {
		summaries = append(summaries, Summary{
			ID: s.active, Active: true,
			Error: "选中的素材包未安装；启动时会回退内置鲸鱼",
		})
	}
	sort.SliceStable(summaries, func(i, j int) bool {
		if summaries[i].Active != summaries[j].Active {
			return summaries[i].Active
		}
		if summaries[i].Name != summaries[j].Name {
			return summaries[i].Name < summaries[j].Name
		}
		return summaries[i].ID < summaries[j].ID
	})
	return summaries, nil
}

func summaryFromManifest(manifest Manifest, active bool) Summary {
	var license struct {
		Status string `json:"status"`
	}
	if len(manifest.License) > 0 {
		_ = json.Unmarshal(manifest.License, &license)
	}
	if license.Status == "" {
		license.Status = "未声明"
	}
	frames := 0
	for _, clip := range manifest.Clips {
		frames += clip.Frames
	}
	interactionIDs := make([]string, 0, len(manifest.Interactions))
	for id := range manifest.Interactions {
		interactionIDs = append(interactionIDs, id)
	}
	sort.Strings(interactionIDs)
	interactions := make([]InteractionDetail, 0, len(interactionIDs))
	for _, id := range interactionIDs {
		interaction := manifest.Interactions[id]
		interactions = append(interactions, InteractionDetail{
			ID: id, Label: interaction.Label, Bubble: interaction.Bubble,
			Motion: interaction.Motion, Steps: append([]Step(nil), interaction.Steps...),
		})
	}
	actionIDs := make([]string, 0, len(manifest.Actions))
	for id := range manifest.Actions {
		actionIDs = append(actionIDs, id)
	}
	sort.Strings(actionIDs)
	actions := make([]ActionDetail, 0, len(actionIDs))
	for _, id := range actionIDs {
		action := manifest.Actions[id]
		actions = append(actions, ActionDetail{
			ID: id, Label: action.Label, LoopCount: action.LoopCount, Timing: action.Timing,
		})
	}
	return Summary{
		ID: manifest.ID, Name: manifest.Name,
		Active: active, Valid: true, Clips: len(manifest.Clips), Frames: frames,
		Hotzones: manifest.HotzoneList(), Interactions: len(manifest.Interactions),
		InteractionList: interactions, ActionList: actions,
		License: license.Status,
	}
}

func containsSummary(summaries []Summary, id string) bool {
	for _, summary := range summaries {
		if summary.ID == id {
			return true
		}
	}
	return false
}

// Install validates a zip, installs it below packs/<id>, and records it in
// the lockfile. It never activates the pack; the console asks the
// application to activate it after the install succeeds. A same-id pack
// returns *PackExistsError before anything is written unless replace is true.
func (s *Store) Install(zipPath string, replace bool) (Summary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.PacksDir(), 0o755); err != nil {
		return Summary{}, err
	}
	manifest, hasManifest, err := ReadZipManifest(zipPath)
	if err != nil {
		return Summary{}, err
	}
	id := manifest.ID
	if id == "" {
		id = idFromZipName(zipPath)
	}
	if !validPackDirID(id) {
		return Summary{}, fmt.Errorf("无法从压缩包确定合法的素材包 id（%q）", id)
	}
	if IsReservedPackID(id) {
		return Summary{}, fmt.Errorf("素材包 id %q 保留给内置素材包", id)
	}
	final := filepath.Join(s.PacksDir(), id)
	if _, err := os.Stat(final); err == nil {
		if !replace {
			name := manifest.Name
			if name == "" {
				name = id
			}
			return Summary{}, &PackExistsError{ID: id, Name: name}
		}
	} else if !os.IsNotExist(err) {
		return Summary{}, err
	}

	pending := pendingFile{
		Operation: "import", ID: id, Stage: "reading", UpdatedAt: time.Now().UTC(),
	}
	if err := s.writePendingLocked(pending); err != nil {
		return Summary{}, err
	}
	opts := ImportOptions{
		ID:    id,
		Force: replace,
		OnStage: func(stage string) {
			pending.Stage = stage
			pending.UpdatedAt = time.Now().UTC()
			_ = s.writePendingLocked(pending)
		},
	}
	if !hasManifest {
		opts.Base = Base{
			Window: Size{Width: 248, Height: 310},
			Sprite: Size{Width: 240, Height: 300},
			Margin: 4,
		}
		opts.BubbleAnchor = Anchor{X: 0.5, Y: 0.05}
	}
	_, importErr := ImportZip(zipPath, s.PacksDir(), opts)
	if importErr != nil {
		pending.Error = importErr.Error()
		pending.UpdatedAt = time.Now().UTC()
		_ = s.writePendingLocked(pending)
		return Summary{}, importErr
	}
	_ = os.Remove(s.pendingPath())
	lock, err := s.loadLocked()
	if err != nil {
		return Summary{}, err
	}
	lock.Active = s.active
	if err := s.refreshLockLocked(lock); err != nil {
		return Summary{}, err
	}
	manifest, err = ReadManifest(final, id)
	if err != nil {
		return Summary{}, err
	}
	summary := summaryFromManifest(manifest, id == s.active)
	summary.ID = id
	if entry, ok := lock.Packs[id]; ok {
		summary.InstalledAt = entry.InstalledAt
	}
	return summary, nil
}

// Delete removes a pack directory. The active pack cannot be deleted because
// that would turn a working selection into a startup fallback.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id == s.active {
		return errors.New("正在使用的素材包不能删除")
	}
	if IsReservedPackID(id) {
		return errors.New("内置素材包不能删除")
	}
	if id == "" || id == "." || id == ".." || strings.HasPrefix(id, ".") ||
		strings.HasSuffix(id, ".backup") || strings.ContainsAny(id, `/\`) || filepath.Base(id) != id {
		return fmt.Errorf("invalid pack id %q", id)
	}
	dir := filepath.Join(s.PacksDir(), id)
	if _, err := os.Stat(dir); err != nil {
		return err
	}
	trash := filepath.Join(s.PacksDir(), fmt.Sprintf(".trash-%s-%d", id, time.Now().UnixNano()))
	if err := os.Rename(dir, trash); err != nil {
		return err
	}
	if err := os.RemoveAll(trash); err != nil {
		return err
	}
	lock, err := s.loadLocked()
	if err != nil {
		return err
	}
	delete(lock.Packs, id)
	lock.Active = s.active
	return s.saveLockLocked(lock)
}

// Preview returns a PNG poster for the console. It uses the idle poster when
// one exists, otherwise the first action or clip with a poster.
func (s *Store) Preview(id string) ([]byte, error) {
	pack, err := Load(s.dataDir, id, 2)
	if err != nil {
		return nil, err
	}
	action, ok := pack.Action(pack.Manifest.States.Idle)
	if !ok || action.Poster == "" {
		for _, candidate := range sortedActions(pack.Manifest.Actions) {
			if candidate.Poster != "" {
				action = candidate
				ok = true
				break
			}
		}
	}
	if !ok || action.Poster == "" {
		return nil, errors.New("素材包没有可用作预览的海报帧")
	}
	img, err := pack.Poster(action)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func sortedActions(actions map[string]Action) []Action {
	ids := make([]string, 0, len(actions))
	for id := range actions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]Action, 0, len(ids))
	for _, id := range ids {
		out = append(out, actions[id])
	}
	return out
}

func idFromZipName(zipPath string) string {
	base := strings.TrimSuffix(filepath.Base(zipPath), filepath.Ext(zipPath))
	var b strings.Builder
	for _, r := range strings.ToLower(base) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		case r == ' ' || r == '.':
			b.WriteRune('-')
		}
	}
	id := strings.Trim(b.String(), "-_")
	if len(id) > 64 {
		id = id[:64]
	}
	return id
}

func (s *Store) lockPath() string    { return filepath.Join(s.dataDir, LockFile) }
func (s *Store) pendingPath() string { return filepath.Join(s.dataDir, PendingFile) }

func (s *Store) loadLocked() (*lockfile, error) {
	lock := &lockfile{Schema: 1, Active: s.active, Packs: make(map[string]lockEntry)}
	data, err := os.ReadFile(s.lockPath())
	if err != nil {
		if os.IsNotExist(err) {
			return lock, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, lock); err != nil {
		// A corrupt lockfile is a cache, not the source of installed data.
		// Keep a copy for diagnosis and rebuild from the pack directories.
		_ = os.Rename(s.lockPath(), s.lockPath()+".bad")
		return &lockfile{Schema: 1, Active: s.active, Packs: make(map[string]lockEntry)}, nil
	}
	if lock.Packs == nil {
		lock.Packs = make(map[string]lockEntry)
	}
	if lock.Active == "" {
		lock.Active = s.active
	}
	return lock, nil
}

func (s *Store) saveLockLocked(lock *lockfile) error {
	lock.Schema = 1
	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeFileAtomic(s.lockPath(), data)
}

func (s *Store) refreshLockLocked(lock *lockfile) error {
	entries, err := os.ReadDir(s.PacksDir())
	if err != nil {
		if os.IsNotExist(err) {
			lock.Packs = make(map[string]lockEntry)
			return s.saveLockLocked(lock)
		}
		return err
	}
	next := make(map[string]lockEntry, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id := entry.Name()
		if strings.HasPrefix(id, ".") || strings.HasSuffix(id, ".backup") || !validPackDirID(id) ||
			IsReservedPackID(id) {
			continue
		}
		dir := filepath.Join(s.PacksDir(), id)
		manifest, err := ReadManifest(dir, id)
		if err != nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, "pack.json"))
		if err != nil {
			continue
		}
		sum := sha256.Sum256(data)
		var license struct {
			Status string `json:"status"`
		}
		if len(manifest.License) > 0 {
			_ = json.Unmarshal(manifest.License, &license)
		}
		var frames int
		for _, clip := range manifest.Clips {
			frames += clip.Frames
		}
		installedAt := time.Now().UTC()
		if old, ok := lock.Packs[id]; ok && !old.InstalledAt.IsZero() {
			installedAt = old.InstalledAt
		} else if info, err := entry.Info(); err == nil {
			installedAt = info.ModTime().UTC()
		}
		next[id] = lockEntry{
			ID: id, Name: manifest.Name,
			Clips: len(manifest.Clips), Frames: frames,
			Hotzones: len(manifest.Hotzones), Interactions: len(manifest.Interactions),
			License: license.Status, Digest: hex.EncodeToString(sum[:]),
			InstalledAt: installedAt,
		}
	}
	lock.Packs = next
	return s.saveLockLocked(lock)
}

func (s *Store) readPendingLocked() (*pendingFile, error) {
	data, err := os.ReadFile(s.pendingPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var pending pendingFile
	if err := json.Unmarshal(data, &pending); err != nil {
		return nil, err
	}
	return &pending, nil
}

func (s *Store) writePendingLocked(pending pendingFile) error {
	data, err := json.MarshalIndent(pending, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeFileAtomic(s.pendingPath(), data)
}

func (s *Store) recoverPendingLocked(pending *pendingFile) {
	if pending == nil || !validPackDirID(pending.ID) || IsReservedPackID(pending.ID) {
		return
	}
	final := filepath.Join(s.PacksDir(), pending.ID)
	backup := pending.Backup
	if backup == "" {
		backup = final + ".backup"
	} else {
		rel, err := filepath.Rel(s.PacksDir(), backup)
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) ||
			!strings.HasSuffix(backup, ".backup") {
			backup = final + ".backup"
		}
	}
	if _, err := LoadDir(final, pending.ID, 1); err == nil {
		_ = os.RemoveAll(backup)
		return
	}
	if _, err := os.Stat(backup); err == nil {
		_ = os.RemoveAll(final)
		_ = os.Rename(backup, final)
	}
}

func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
