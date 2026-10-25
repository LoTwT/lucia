// Command lucia is the entry point of the Lucia desktop application. It
// resolves the per-user directories before MyGo starts, keeps a retrievable
// diagnostic log, wires the single-instance and deep-link behavior, and
// assembles the window model and the update manager.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/LoTwT/lucia/internal/apppaths"
	"github.com/LoTwT/lucia/internal/buildinfo"
	"github.com/LoTwT/lucia/internal/diagnostics"
	"github.com/LoTwT/lucia/internal/pet"
	"github.com/LoTwT/lucia/internal/petpack"
	"github.com/LoTwT/lucia/internal/settings"
	appui "github.com/LoTwT/lucia/internal/ui"
	"github.com/LoTwT/lucia/internal/updates"
	"github.com/egoist/mygo"
	mgui "github.com/egoist/mygo/ui"
)

// repositoryURL is the public project page linked from the About page and
// used as the update feed's home.
const repositoryURL = "https://github.com/LoTwT/lucia"

func main() {
	// The packaging self-check: an explicit environment switch turns the
	// process into a one-shot report of this build's identity and updater
	// state, then exits. It runs before directories, logs or the event loop,
	// so a release workflow can assert what the artifact itself carries.
	if os.Getenv("LUCIA_SELFTEST") != "" {
		selfTest()
		return
	}

	version := mygo.App.Version()
	info := buildinfo.Current(version)
	appName := mygo.App.Name()

	paths, err := apppaths.Resolve(appName)
	if err != nil {
		log.Fatal(err)
	}
	if err := paths.Ensure(); err != nil {
		log.Fatal(err)
	}
	// Set the overrides before Run: the framework reads its own state from
	// these paths as it starts, and the model, settings and logs must share
	// the same root.
	mygo.App.SetPath(mygo.PathUserData, paths.Data)
	mygo.App.SetPath(mygo.PathCache, paths.Cache)
	mygo.App.SetPath(mygo.PathLogs, paths.Logs)

	// A second launch hands its command line to the first instance and
	// exits before touching the log or the data files.
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}

	// The diagnostic log is opened after the single-instance lock, so two
	// processes never interleave lines. A failure is not fatal: stderr still
	// carries the log, and the application remains usable.
	logFile, err := diagnostics.Open(paths.Logs)
	if err != nil {
		log.Printf("lucia: diagnostic log unavailable, logging to stderr only: %v", err)
	} else {
		defer logFile.Close()
		log.SetOutput(logFile)
	}
	log.Printf("lucia: %s data=%q logs=%q", info.LogLine(), paths.Data, paths.Logs)

	prefs, err := settings.Load(paths.Data)
	if err != nil {
		log.Printf("lucia: settings: %v (using defaults)", err)
	}
	log.Printf("lucia: settings: check updates on launch=%v", prefs.CheckUpdatesOnLaunch)
	settingsStore := settings.NewStore(paths.Data, prefs)
	packStore := petpack.NewStore(paths.Data, prefs.PetPack)
	if err := packStore.Recover(); err != nil {
		log.Printf("lucia: recover packs: %v", err)
	}
	if err := packStore.SetActive(prefs.PetPack); err != nil {
		log.Printf("lucia: record active pack: %v", err)
	}

	manager := updates.New(updates.MyGo{}, info.Version)
	manager.SetAutoCheck(prefs.CheckUpdatesOnLaunch)

	petModel, err := newPetModel(paths.Data, prefs.PetPack)
	if err != nil {
		log.Fatalf("lucia: pet assets: %v", err)
	}
	if prefs.PetPack != "" && !petModel.Animated() {
		log.Printf("lucia: pet pack %q failed to load; clearing the active selection", prefs.PetPack)
		if err := clearFallbackActive(settingsStore, packStore, &prefs); err != nil {
			log.Printf("lucia: clear active pack: %v", err)
		}
	}
	petModel.SetScale(prefs.PetScale)
	if scale := petModel.Scale(); scale != prefs.PetScale {
		updated, err := settingsStore.Update(func(s *settings.Settings) { s.PetScale = scale })
		if err != nil {
			log.Printf("lucia: normalize pet scale: %v", err)
		} else {
			prefs = updated
		}
	}
	playIntro := prefs.PetVisible && petModel.Animated() && !prefs.PetIntroPlayed

	var (
		win        *mygo.Window
		petWin     *petWindow
		bubbleWin  *bubbleWindow
		tray       *mygo.Tray
		quitting   bool
		lastBubble time.Time
		lastPhase  updates.Phase
	)
	petSound := newPetSound()
	var activatePetPack func(string) error
	model := appui.New(appui.Config{
		Version:  info.Version,
		DataDir:  paths.Data,
		Updates:  manager,
		Settings: settingsStore.Snapshot(),
		SaveSettings: func(s settings.Settings) error {
			updated, err := updateUISettings(settingsStore, s)
			if err != nil {
				return err
			}
			prefs = updated
			return nil
		},
		ApplyPetVisible: func(visible bool) { petWin.SetVisible(visible) },
		ApplyPetScale: func(scale float64) {
			bubbleWin.Hide()
			petWin.ApplyScale(scale)
		},
		OpenAtLogin:    func() bool { return mygo.App.OpenAtLogin() },
		SetOpenAtLogin: func(open bool) error { return mygo.App.SetOpenAtLogin(open) },
		ResetPetPosition: func() {
			bubbleWin.Hide()
			petWin.ResetPosition()
		},
		OpenDataDir: func() error { return mygo.Shell.OpenPath(paths.Data) },
		LoadPacks: func() ([]appui.PackSummary, error) {
			list, err := packStore.List()
			if err != nil {
				return nil, err
			}
			out := make([]appui.PackSummary, 0, len(list))
			for _, summary := range list {
				out = append(out, uiPackSummary(summary))
			}
			return out, nil
		},
		ChoosePackZipAsync: func(done func(string, error)) {
			// Dialog.Open posts the native panel to the main loop and waits.
			// Calling it on the UI thread would block that loop; keep the
			// dialog in a goroutine and return the result through Update.
			parent := win
			go func() {
				paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
					Parent:  parent,
					Title:   "选择素材包",
					Message: "选择包含透明动画 WebP 的 zip",
					Filters: []mygo.FileFilter{{Name: "素材包", Extensions: []string{"zip"}}},
				})
				path := ""
				if len(paths) > 0 {
					path = paths[0]
				}
				if parent == nil || parent.IsDestroyed() {
					return
				}
				parent.Update(func() { done(path, err) })
			}()
		},
		ImportPack: func(zipPath string, acceptLicense bool) (appui.PackSummary, error) {
			summary, err := packStore.Import(zipPath, acceptLicense)
			if err != nil {
				return appui.PackSummary{}, err
			}
			return uiPackSummary(summary), nil
		},
		ReplacePack: func(zipPath string, acceptLicense bool) (appui.PackSummary, error) {
			summary, err := packStore.Replace(zipPath, acceptLicense)
			if err != nil {
				return appui.PackSummary{}, err
			}
			return uiPackSummary(summary), nil
		},
		ActivatePack: func(id string) error {
			if activatePetPack == nil {
				return fmt.Errorf("应用尚未就绪")
			}
			return activatePetPack(id)
		},
		DeletePack: func(id string) error { return packStore.Delete(id) },
		ConfirmDeletePackAsync: func(id, name string, done func(bool)) {
			// Dialog.Message has the same main-loop contract as Open.
			parent := win
			go func() {
				res, err := mygo.Dialog.Message(mygo.MessageOptions{
					Parent:        parent,
					Type:          mygo.MessageWarning,
					Title:         "删除素材包",
					Message:       fmt.Sprintf("删除“%s”（%s）？", name, id),
					Detail:        "只会删除本机数据目录中的素材包，不会影响内置鲸鱼。",
					Buttons:       []string{"删除", "取消"},
					DefaultButton: 1,
					CancelButton:  1,
				})
				if parent == nil || parent.IsDestroyed() {
					return
				}
				parent.Update(func() { done(err == nil && res.Button == 0) })
			}()
		},
		LoadPackPreview: func(id string) ([]byte, error) { return packStore.Preview(id) },
		OpenPacksDir: func() error {
			if err := os.MkdirAll(packStore.PacksDir(), 0o755); err != nil {
				return err
			}
			return mygo.Shell.OpenPath(packStore.PacksDir())
		},
		RepositoryURL: repositoryURL,
		UpdateSource:  "LoTwT/lucia",
	})

	syncPetVisible := func(visible bool) {
		prefs.PetVisible = visible
		model.SetPetVisible(visible)
		if win != nil && !win.IsDestroyed() {
			win.Update(func() {})
		}
	}
	showWindow := func() {
		if win == nil || win.IsDestroyed() {
			return
		}
		model.RefreshOpenAtLogin()
		win.Show()
		win.Focus()
		win.Update(func() {})
	}
	togglePet := func() {
		if petWin == nil {
			return
		}
		if petWin.IsVisible() {
			bubbleWin.Hide()
		}
		petWin.Toggle()
		syncPetVisible(petWin.IsVisible())
	}
	resetPet := func() {
		if petWin == nil {
			return
		}
		bubbleWin.Hide()
		petWin.ResetPosition()
		petModel.PlayAction("wave")
		syncPetVisible(true)
	}

	configurePetModel := func(m *pet.Model) {
		if m == nil {
			return
		}
		m.SetMenuActions(pet.MenuActions{
			OpenSettings: func() {
				model.ShowOverview()
				showWindow()
			},
			Hide:  togglePet,
			Reset: resetPet,
		})
		m.SetReactionHandler(func(reaction pet.Reaction) {
			if petWin == nil || !petWin.IsVisible() {
				return
			}
			if prefs.SoundEnabled {
				playPetSound(petSound)
			}
			now := time.Now()
			if !bubbleAllowed(now, lastBubble, prefs.BubbleCooldownSeconds) {
				return
			}
			lastBubble = now
			bubbleWin.Show(reaction, petWin.BubbleRect(), currentPetDisplays())
		})
	}
	configurePetModel(petModel)

	activatePetPack = func(id string) error {
		var next *pet.Model
		var err error
		if id == "" {
			next, err = pet.New()
		} else {
			pack, loadErr := petpack.Load(paths.Data, id, 2)
			if loadErr != nil {
				return fmt.Errorf("加载素材包 %q：%w", id, loadErr)
			}
			next, err = pet.NewAnimated(pack)
		}
		if err != nil {
			return fmt.Errorf("创建桌宠：%w", err)
		}
		next.SetScale(prefs.PetScale)
		configurePetModel(next)
		oldID := prefs.PetPack
		if err := packStore.SetActive(id); err != nil {
			return err
		}
		updated, err := settingsStore.Update(func(s *settings.Settings) {
			s.PetPack = id
		})
		if err != nil {
			_ = packStore.SetActive(oldID)
			return err
		}
		prefs = updated
		oldModel := petModel
		petModel = next
		if petWin != nil {
			petWin.SetModel(next)
		}
		if oldModel != nil {
			oldModel.Close()
		}
		if bubbleWin != nil {
			bubbleWin.Hide()
		}
		if next.Animated() && id != "" {
			next.PlayAction("wave")
		}
		return nil
	}

	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { quitting = true })
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.OnSecondInstance(func([]string, string) { showWindow() })
	mygo.App.OnActivate(func(hasVisibleWindows bool) {
		// Keep the pet as the default face of the app: a click on the Dock
		// icon opens the console only when neither the pet nor another
		// window is visible.
		if !hasVisibleWindows && (petWin == nil || !petWin.IsVisible()) {
			showWindow()
		}
	})
	mygo.App.OnOpenURL(func(raw string) {
		// lucia://open is the registered deep link. Any URL of the scheme
		// brings the running window forward; routing state is intentionally
		// out of scope until the app has pages that need it.
		log.Printf("lucia: open url %q", raw)
		showWindow()
	})

	mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu},
		{Label: "文件", Submenu: []*mygo.MenuItem{
			{Label: "打开数据目录", Accelerator: "CmdOrCtrl+Shift+O", Click: func(*mygo.MenuItem, *mygo.Window) {
				if err := mygo.Shell.OpenPath(paths.Data); err != nil {
					log.Printf("lucia: open data directory: %v", err)
				}
			}},
			mygo.Separator(),
			{Role: mygo.RoleClose},
		}},
		{Role: mygo.RoleEditMenu},
		{Role: mygo.RoleWindowMenu},
		{Label: "帮助", Submenu: []*mygo.MenuItem{
			{Label: "检查更新", Accelerator: "CmdOrCtrl+U", Click: func(*mygo.MenuItem, *mygo.Window) {
				showWindow()
				model.ShowUpdates()
				if win != nil && !win.IsDestroyed() {
					win.Update(func() {})
				}
			}},
			mygo.Separator(),
			{Label: "MyGo 文档", Click: func(*mygo.MenuItem, *mygo.Window) {
				if err := mygo.Shell.OpenExternal("https://mygo.egoist.dev/docs/"); err != nil {
					log.Printf("lucia: open MyGo docs: %v", err)
				}
			}},
		}},
	}))

	autoCtx, stopAutoChecks := context.WithCancel(context.Background())
	defer stopAutoChecks()

	mygo.App.WhenReady(func() {
		petWin = newPetWindow(petModel, func() bool { return quitting }, prefs.PetVisible)
		if playIntro {
			petWin.SetOnFirstFrame(func() {
				if updated, err := settingsStore.Update(func(s *settings.Settings) {
					s.PetIntroPlayed = true
				}); err != nil {
					log.Printf("lucia: save pet intro state: %v", err)
				} else {
					prefs = updated
				}
			})
			petModel.PlayAction("wave")
		}
		bubbleWin = newBubbleWindow(func() bool { return quitting })
		petWin.SetOnMove(func() { bubbleWin.Hide() })
		petWin.SetOnHide(func() {
			bubbleWin.Hide()
			syncPetVisible(false)
		})
		petWin.SetOnDragStart(func() {
			petModel.DragStarted()
			bubbleWin.Hide()
		})
		petWin.SetOnDragEnd(func() { petModel.DragEnded() })

		win = mygo.NewWindow(mygo.WindowOptions{
			Title:     "Lucia",
			Width:     1000,
			Height:    700,
			MinWidth:  760,
			MinHeight: 520,
			// The console starts hidden; the tray and Dock icon open it.
			Hidden: true,
			// Opens where the user left it, keeping the size across launches.
			StateKey: "main",
			Content:  mgui.View(model.View),
		})
		win.OnClose(func(e *mygo.CloseEvent) {
			if quitting {
				return
			}
			e.PreventDefault()
			win.Hide()
		})

		tray, err = mygo.NewTray(mygo.TrayOptions{
			Icon:    pet.TrayIcon(),
			ToolTip: "Lucia 桌宠",
			Menu: mygo.NewMenu([]*mygo.MenuItem{
				{Label: "显示/隐藏宠物", Click: func(*mygo.MenuItem, *mygo.Window) { togglePet() }},
				{Label: "重置宠物位置", Click: func(*mygo.MenuItem, *mygo.Window) { resetPet() }},
				{Label: "打开 Lucia", Click: func(*mygo.MenuItem, *mygo.Window) { showWindow() }},
				mygo.Separator(),
				{Label: "退出", Click: func(*mygo.MenuItem, *mygo.Window) {
					quitting = true
					mygo.App.Quit()
				}},
			}),
		})
		if err != nil {
			log.Printf("lucia: tray: %v", err)
		} else {
			// Keep the tray handle reachable while the app runs; the menu
			// callbacks above are the user's only way back to the console.
			_ = tray
		}

		// Background state changes redraw the window; Update runs the
		// closure on the UI thread and is safe from the manager's goroutines.
		manager.SetNotify(func() {
			snapshot := manager.Snapshot()
			if win != nil && !win.IsDestroyed() {
				win.Update(func() {
					if snapshot.Phase == lastPhase {
						return
					}
					lastPhase = snapshot.Phase
					if petWin == nil || !petWin.IsVisible() {
						return
					}
					switch snapshot.Phase {
					case updates.PhaseChecking, updates.PhaseDownloading:
						petModel.PlayBackgroundAction("think")
					case updates.PhaseFailed:
						petModel.PlayBackgroundAction("sad")
					}
				})
			}
		})
		manager.SetInstalled(func() {
			// Relaunch must start on the UI thread. The update is installed;
			// the running process keeps working until the restart.
			if win != nil && !win.IsDestroyed() {
				win.Update(func() {
					log.Printf("lucia: update installed, restarting")
					mygo.App.Relaunch()
				})
			}
		})
		manager.StartAutoChecks(autoCtx, appui.CheckInterval)
	})

	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
	log.Printf("lucia: exited")
}

// clearFallbackActive removes a selected pack after the startup load failed
// and the built-in pet took over. Keeping it active would make the console
// claim a pack is in use while the pet window shows the whale.
func clearFallbackActive(settingsStore *settings.Store, packStore *petpack.Store, prefs *settings.Settings) error {
	old := prefs.PetPack
	if err := packStore.SetActive(""); err != nil {
		return err
	}
	updated, err := settingsStore.Update(func(s *settings.Settings) {
		s.PetPack = ""
	})
	if err != nil {
		_ = packStore.SetActive(old)
		return err
	}
	*prefs = updated
	return nil
}

// updateUISettings merges the presentation fields owned by the console into
// the process-wide settings store. Runtime-only fields such as petIntroPlayed
// and petPack must survive a console save.
func updateUISettings(store *settings.Store, next settings.Settings) (settings.Settings, error) {
	return store.Update(func(cur *settings.Settings) {
		cur.CheckUpdatesOnLaunch = next.CheckUpdatesOnLaunch
		cur.PetVisible = next.PetVisible
		cur.PetScale = next.PetScale
		cur.SoundEnabled = next.SoundEnabled
		cur.BubbleCooldownSeconds = next.BubbleCooldownSeconds
	})
}

// newPetModel loads the configured external pack, falling back to the
// embedded whale when the pack is missing or invalid. A broken optional pack
// must never keep the app from starting.
func newPetModel(dataDir, packID string) (*pet.Model, error) {
	if packID != "" {
		pack, err := petpack.Load(dataDir, packID, 2)
		if err != nil {
			log.Printf("lucia: pet pack %q: %v (using the built-in pet)", packID, err)
		} else {
			model, err := pet.NewAnimated(pack)
			if err != nil {
				log.Printf("lucia: pet pack %q: %v (using the built-in pet)", packID, err)
			} else {
				log.Printf("lucia: pet pack %q loaded: %d clips", packID, len(pack.Clips))
				return model, nil
			}
		}
	}
	return pet.New()
}

// selfTest reports the identity and updater state of the built artifact.
// Development runs print updates=disabled; a release build must print
// updates=enabled. The output goes to stderr because Windows GUI builds
// have no console.
func selfTest() {
	info := buildinfo.Current(mygo.App.Version())
	state := "disabled"
	if mygo.Updater.Enabled() {
		state = "enabled"
	}
	fmt.Fprintf(os.Stderr, "lucia: self-test: %s updates=%s\n", info.LogLine(), state)
}
