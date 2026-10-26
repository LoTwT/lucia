// Command petpack imports, validates, builds, and previews animated-WebP pet
// packs. It is a development tool; the desktop app never ships the naiwa
// artwork itself.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/LoTwT/lucia/internal/apppaths"
	"github.com/LoTwT/lucia/internal/petpack"
	"github.com/LoTwT/lucia/internal/settings"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "build":
			runBuild(os.Args[2:])
			return
		case "preview":
			runPreview(os.Args[2:])
			return
		}
	}
	zipPath := flag.String("zip", "", "animated-WebP zip to import")
	appName := flag.String("app", "", "application name used to resolve the data directory; empty auto-detects Lucia Dev, lucia or Lucia")
	dataDir := flag.String("data", "", "explicit data directory; overrides -app")
	packID := flag.String("id", "", "pack id below <data>/packs; empty uses the manifest or zip name")
	packName := flag.String("name", "", "display name; empty uses the manifest")
	force := flag.Bool("force", false, "replace an existing pack with the same id")
	activate := flag.Bool("activate", true, "select this pack in settings.json")
	checkDir := flag.String("check", "", "validate a pack directory and exit")
	flag.Parse()

	if *zipPath == "" && *checkDir == "" {
		fmt.Fprintln(os.Stderr, "usage:\n  petpack (-zip <pack.zip> | -check <pack-dir>) [-app <name> | -data <dir>] [-force]\n  petpack build <source-dir> -o <pack.zip> [-force]\n  petpack preview <source-dir> -o <preview.png> [-clip <id>] [-frame <index>]")
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
		checkPack(*checkDir)
		return
	}
	if err := os.MkdirAll(data, 0o755); err != nil {
		log.Fatal(err)
	}
	store := petpack.NewStore(data, "")
	if err := store.Recover(); err != nil {
		log.Fatal(err)
	}
	_, hasManifest, err := petpack.ReadZipManifest(*zipPath)
	if err != nil {
		log.Fatal(err)
	}
	opts := petpack.ImportOptions{
		ID:    *packID,
		Name:  *packName,
		Force: *force,
	}
	if !hasManifest {
		opts.Base = petpack.Base{
			Window: petpack.Size{Width: 248, Height: 310},
			Sprite: petpack.Size{Width: 240, Height: 300},
			Margin: 4,
		}
		opts.BubbleAnchor = petpack.Anchor{X: 0.5, Y: 0.05}
	}
	pack, err := petpack.ImportZip(*zipPath, filepath.Join(data, "packs"), opts)
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

func leadingSourceArg(args []string) (string, []string) {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		return args[0], args[1:]
	}
	return "", args
}

func runBuild(args []string) {
	source, args := leadingSourceArg(args)
	flags := flag.NewFlagSet("petpack build", flag.ExitOnError)
	output := flags.String("o", "", "output zip path")
	force := flags.Bool("force", false, "replace an existing output zip")
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "usage: petpack build <source-dir> -o <pack.zip> [-force]")
	}
	if err := flags.Parse(args); err != nil {
		os.Exit(2)
	}
	if source == "" && flags.NArg() == 1 {
		source = flags.Arg(0)
	}
	if source == "" || *output == "" || flags.NArg() > 0 && flags.Arg(0) != source {
		flags.Usage()
		os.Exit(2)
	}
	result, err := petpack.Build(source, *output, *force)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("pack=%s clips=%d frames=%d actions=%d hotzones=%d interactions=%d canvas=%dx%d crop=%d,%d,%dx%d\n",
		result.ID, result.Clips, result.Frames, result.Actions, result.Hotzones, result.Interactions,
		result.Canvas.Width, result.Canvas.Height,
		result.Crop.X, result.Crop.Y, result.Crop.Width, result.Crop.Height)
	for _, warning := range result.Warnings {
		fmt.Printf("warning: %s\n", warning)
	}
	fmt.Printf("zip=%s\n", result.Path)
}

func runPreview(args []string) {
	source, args := leadingSourceArg(args)
	flags := flag.NewFlagSet("petpack preview", flag.ExitOnError)
	output := flags.String("o", "", "output PNG path")
	clip := flags.String("clip", "", "clip id; empty uses the idle poster")
	frame := flags.Int("frame", 0, "frame index when -clip is set")
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "usage: petpack preview <source-dir> -o <preview.png> [-clip <id>] [-frame <index>]")
	}
	if err := flags.Parse(args); err != nil {
		os.Exit(2)
	}
	if source == "" && flags.NArg() == 1 {
		source = flags.Arg(0)
	}
	if source == "" || *output == "" || flags.NArg() > 0 && flags.Arg(0) != source {
		flags.Usage()
		os.Exit(2)
	}
	result, err := petpack.PreviewSource(source, *output, *clip, *frame)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("preview=%s pack=%s canvas=%dx%d crop=%d,%d,%dx%d\n",
		*output, result.ID, result.Canvas.Width, result.Canvas.Height,
		result.Crop.X, result.Crop.Y, result.Crop.Width, result.Crop.Height)
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

// checkPack validates one installed directory without importing it.
func checkPack(dir string) {
	clean := filepath.Clean(dir)
	id := filepath.Base(clean)
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
	fmt.Printf("pack=%s clips=%d frames=%d hotzones=%d interactions=%d\n",
		pack.Manifest.ID, len(pack.Clips), frames,
		len(pack.Manifest.Hotzones), len(pack.Manifest.Interactions))
}
