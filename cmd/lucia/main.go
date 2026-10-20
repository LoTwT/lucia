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

	manager := updates.New(updates.MyGo{}, info.Version)
	manager.SetAutoCheck(prefs.CheckUpdatesOnLaunch)

	petModel, err := pet.New()
	if err != nil {
		log.Fatalf("lucia: pet assets: %v", err)
	}

	var (
		win        *mygo.Window
		petWin     *petWindow
		bubbleWin  *bubbleWindow
		tray       *mygo.Tray
		quitting   bool
		lastBubble time.Time
	)
	petSound := newPetSound()
	model := appui.New(appui.Config{
		Version:  info.Version,
		Commit:   info.CommitLabel(),
		Platform: buildinfo.Platform(),
		DataDir:  paths.Data,
		Updates:  manager,
		Settings: prefs,
		SaveSettings: func(s settings.Settings) error {
			if err := settings.Save(paths.Data, s); err != nil {
				return err
			}
			prefs = s
			return nil
		},
		ApplyPetVisible: func(visible bool) { petWin.SetVisible(visible) },
		OpenAtLogin:     func() bool { return mygo.App.OpenAtLogin() },
		SetOpenAtLogin:  func(open bool) error { return mygo.App.SetOpenAtLogin(open) },
		ResetPetPosition: func() {
			bubbleWin.Hide()
			petWin.ResetPosition()
		},
		OpenDataDir:   func() error { return mygo.Shell.OpenPath(paths.Data) },
		RepositoryURL: repositoryURL,
		UpdateSource:  "LoTwT/lucia",
	})

	showWindow := func() {
		if win == nil || win.IsDestroyed() {
			return
		}
		win.Show()
		win.Focus()
	}
	togglePet := func() {
		if petWin == nil {
			return
		}
		if petWin.IsVisible() {
			bubbleWin.Hide()
		}
		petWin.Toggle()
		visible := petWin.IsVisible()
		prefs.PetVisible = visible
		model.SetPetVisible(visible)
		if win != nil && !win.IsDestroyed() {
			win.Update(func() {})
		}
	}
	resetPet := func() {
		if petWin == nil {
			return
		}
		bubbleWin.Hide()
		petWin.ResetPosition()
		prefs.PetVisible = true
		model.SetPetVisible(true)
		if win != nil && !win.IsDestroyed() {
			win.Update(func() {})
		}
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
			{Label: "检查更新…", Accelerator: "CmdOrCtrl+U", Click: func(*mygo.MenuItem, *mygo.Window) {
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
		bubbleWin = newBubbleWindow()
		petWin.SetOnMove(func() { bubbleWin.Hide() })
		petWin.SetOnHide(func() { bubbleWin.Hide() })
		petWin.SetOnDragStart(func() {
			petModel.DragStarted()
			bubbleWin.Hide()
		})
		petWin.SetOnDragEnd(func() { petModel.DragEnded() })
		petModel.SetReactionHandler(func(reaction pet.Reaction) {
			if petWin == nil || !petWin.IsVisible() {
				return
			}
			if prefs.SoundEnabled {
				playPetSound(petSound)
			}
			cooldown := prefs.BubbleCooldownSeconds
			if cooldown < 0 {
				return
			}
			if cooldown > 0 && time.Since(lastBubble) < time.Duration(cooldown)*time.Second {
				return
			}
			lastBubble = time.Now()
			bubbleWin.Show(reaction, petWin.Rect(), currentPetDisplays())
		})

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
			if win != nil && !win.IsDestroyed() {
				win.Update(func() {})
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
