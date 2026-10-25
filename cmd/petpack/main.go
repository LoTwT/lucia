// Command petpack imports an animated-WebP pet pack into Lucia's data
// directory. It is a development tool; the desktop app never ships the
// naiwa artwork itself.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/LoTwT/lucia/internal/apppaths"
	"github.com/LoTwT/lucia/internal/petpack"
	"github.com/LoTwT/lucia/internal/settings"
)

func main() {
	zipPath := flag.String("zip", "", "animated-WebP zip to import")
	appName := flag.String("app", "", "application name used to resolve the data directory; empty auto-detects Lucia Dev, lucia or Lucia")
	dataDir := flag.String("data", "", "explicit data directory; overrides -app")
	packID := flag.String("id", "naiwa", "pack id below <data>/packs")
	packName := flag.String("name", "naiwa", "display name")
	force := flag.Bool("force", false, "replace an existing pack with the same id")
	activate := flag.Bool("activate", true, "select this pack in settings.json")
	checkDir := flag.String("check", "", "validate a pack directory and exit")
	strict := flag.Bool("strict", false, "with -check, require schema 2 interaction data")
	flag.Parse()

	if *zipPath == "" && *checkDir == "" {
		fmt.Fprintln(os.Stderr, "usage: petpack (-zip <pack.zip> | -check <pack-dir>) [-app <name> | -data <dir>] [-force] [-strict]")
		os.Exit(2)
	}
	data := *dataDir
	if data == "" {
		name := *appName
		if name == "" {
			name = detectAppName()
		}
		paths, err := apppaths.Resolve(name)
		if err != nil {
			log.Fatal(err)
		}
		data = paths.Data
	}
	if *checkDir != "" {
		checkPack(*checkDir, *strict)
		return
	}
	if err := os.MkdirAll(data, 0o755); err != nil {
		log.Fatal(err)
	}
	store := petpack.NewStore(data, "")
	if err := store.Recover(); err != nil {
		log.Fatal(err)
	}
	pack, err := petpack.ImportZip(*zipPath, filepath.Join(data, "packs"), petpack.ImportOptions{
		ID:   *packID,
		Name: *packName,
		Base: petpack.Base{
			Window: petpack.Size{Width: 248, Height: 310},
			Sprite: petpack.Size{Width: 240, Height: 300},
			Margin: 4,
		},
		Face:         petpack.Face{X: 0.34, Y: 0.06, Width: 0.32, Height: 0.20},
		BubbleAnchor: petpack.Anchor{X: 0.5, Y: 0.05},
		Force:        *force,
	})
	if err != nil {
		log.Fatal(err)
	}
	if _, err := store.List(); err != nil {
		log.Fatalf("refresh pack lockfile: %v", err)
	}
	if *activate {
		prefs, err := settings.Load(data)
		if err != nil {
			log.Fatalf("settings: %v (the pack was imported but not activated; fix the settings file and retry)", err)
		}
		settingsStore := settings.NewStore(data, prefs)
		updated, err := settingsStore.Update(func(s *settings.Settings) {
			s.PetPack = pack.Manifest.ID
		})
		if err != nil {
			log.Fatal(err)
		}
		prefs = updated
		if err := store.SetActive(pack.Manifest.ID); err != nil {
			log.Fatal(err)
		}
	}
	var frames int
	for _, clip := range pack.Clips {
		frames += clip.FrameCount()
	}
	fmt.Printf("data=%s\n", data)
	fmt.Printf("imported %s to %s\n", pack.Manifest.ID, pack.Dir)
	fmt.Printf("clips=%d frames=%d crop=%dx%d cache=%dx%d\n",
		len(pack.Clips), frames, pack.Manifest.Crop.Width, pack.Manifest.Crop.Height,
		pack.PixelWidth, pack.PixelHeight)
	if *activate {
		fmt.Printf("settings.json now selects %q; restart Lucia to use it\n", pack.Manifest.ID)
	}
}

// detectAppName chooses the data directory touched most recently among the
// names used by the development, bare-binary and packaged apps. That keeps
// the default import target aligned with the build the user actually ran.
func detectAppName() string {
	candidates := []string{"Lucia Dev", "lucia", "Lucia"}
	type found struct {
		name string
		mod  time.Time
	}
	var existing []found
	for _, name := range candidates {
		paths, err := apppaths.Resolve(name)
		if err != nil {
			continue
		}
		info, err := os.Stat(filepath.Join(paths.Data, settings.File))
		if err != nil {
			continue
		}
		existing = append(existing, found{name: name, mod: info.ModTime()})
	}
	if len(existing) == 0 {
		return "Lucia Dev"
	}
	sort.Slice(existing, func(i, j int) bool { return existing[i].mod.After(existing[j].mod) })
	return existing[0].name
}

// checkPack validates one installed directory without importing it. The
// strict mode is aimed at schema 2 authoring: it requires hotzones and
// presets so a pack exercised in the console is not silently unclickable.
func checkPack(dir string, strict bool) {
	clean := filepath.Clean(dir)
	id := filepath.Base(clean)
	if strict {
		raw, err := petpack.ReadRawManifest(clean, id)
		if err != nil {
			log.Fatal(err)
		}
		if err := strictManifestError(id, raw); err != nil {
			log.Fatal(err)
		}
	}
	if _, err := petpack.ReadManifest(clean, id); err != nil {
		log.Fatal(err)
	}
	pack, err := petpack.LoadDir(clean, id, 1)
	if err != nil {
		log.Fatal(err)
	}
	var frames int
	for _, clip := range pack.Clips {
		frames += clip.FrameCount()
	}
	fmt.Printf("pack=%s schema=%d clips=%d frames=%d hotzones=%d presets=%d\n",
		pack.Manifest.ID, pack.Manifest.Schema, len(pack.Clips), frames,
		len(pack.Manifest.Hotzones), len(pack.Manifest.Presets))
}

func strictManifestError(id string, raw petpack.Manifest) error {
	if raw.Schema != 2 {
		return fmt.Errorf("petpack %q: strict check requires schema 2 on disk, got %d", id, raw.Schema)
	}
	if len(raw.Hotzones) == 0 {
		return fmt.Errorf("petpack %q: strict check requires at least one explicit hotzone", id)
	}
	if len(raw.Presets) == 0 {
		return fmt.Errorf("petpack %q: strict check requires at least one explicit preset", id)
	}
	return nil
}
