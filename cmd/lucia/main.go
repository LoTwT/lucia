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
	"sync/atomic"
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

	model := appui.New(appui.Config{
		Version:       info.Version,
		Commit:        info.CommitLabel(),
		Platform:      buildinfo.Platform(),
		DataDir:       paths.Data,
		Updates:       manager,
		Settings:      prefs,
		SaveSettings:  func(s settings.Settings) error { return settings.Save(paths.Data, s) },
		OpenDataDir:   func() error { return mygo.Shell.OpenPath(paths.Data) },
		RepositoryURL: repositoryURL,
		UpdateSource:  "LoTwT/lucia",
	})

	var (
		win      *mygo.Window
		petWin   *mygo.Window
		tray     *mygo.Tray
		quitting bool
	)
	showWindow := func() {
		if win == nil || win.IsDestroyed() {
			return
		}
		win.Show()
		win.Focus()
	}
	togglePet := func() {
		if petWin == nil || petWin.IsDestroyed() {
			return
		}
		if petWin.IsVisible() {
			petWin.Hide()
			return
		}
		petWin.ShowInactive()
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
		area := mygo.Screen.PrimaryDisplay().WorkArea
		petWin = mygo.NewWindow(mygo.WindowOptions{
			Title:             "Lucia 桌宠",
			Width:             pet.Size,
			Height:            pet.Size,
			X:                 area.X + area.Width - pet.Size - 24,
			Y:                 area.Y + area.Height - pet.Size - 24,
			Frameless:         true,
			Transparent:       true,
			AlwaysOnTop:       true,
			Panel:             true,
			SkipTaskbar:       true,
			DisableShadow:     true,
			DisableResize:     true,
			DisableMinimize:   true,
			DisableMaximize:   true,
			DisableFullScreen: true,
			// Opens where the user left it, keeping the size across launches.
			StateKey: "pet",
			Content:  mgui.View(petModel.View),
		})
		// Closing the pet hides it; only the tray's quit closes the app.
		petWin.OnClose(func(e *mygo.CloseEvent) {
			if quitting {
				return
			}
			e.PreventDefault()
			petWin.Hide()
		})
		// DragWindow's double click asks macOS to zoom the window, even
		// though a pet has no meaningful maximized state. Undo it at once.
		petWin.OnMaximize(func() { petWin.Unmaximize() })
		// Native DragWindow crosses displays without fighting the window
		// server, but it lets the pet hang past an outer edge. Correct that
		// after the mouse is released instead of calling SetPosition during
		// the drag: a position change in the native drag loop is what caused
		// the bounce at the edge and at the display seam.
		clampPetOutside := func() {
			if petWin == nil || petWin.IsDestroyed() {
				return
			}
			x, y := petWin.Position()
			next, changed := pet.KeepOnScreen(pet.Rect{
				X: x, Y: y, Width: pet.Size, Height: pet.Size,
			}, currentPetDisplays())
			if changed {
				petWin.SetPosition(next.X, next.Y)
			}
		}
		var clampWatching atomic.Bool
		petWin.OnMove(func() {
			x, y := petWin.Position()
			if _, changed := pet.KeepOnScreen(pet.Rect{
				X: x, Y: y, Width: pet.Size, Height: pet.Size,
			}, currentPetDisplays()); !changed {
				return
			}
			if !clampWatching.CompareAndSwap(false, true) {
				return
			}
			go func() {
				defer clampWatching.Store(false)
				down, ok := leftMouseButtonDown()
				if !ok {
					// No native button state: give the drag time to settle.
					time.Sleep(300 * time.Millisecond)
				} else {
					for down {
						time.Sleep(25 * time.Millisecond)
						down, ok = leftMouseButtonDown()
						if !ok {
							break
						}
					}
				}
				clampPetOutside()
			}()
		})
		// Startup and display changes are a recovery path, not a drag: put
		// the whole window back inside one work area so a position saved on a
		// disconnected display cannot leave the pet unreachable.
		clampPet := func() {
			if petWin == nil || petWin.IsDestroyed() {
				return
			}
			if petWin.IsMaximized() {
				petWin.Unmaximize()
			}
			if w, h := petWin.Size(); w != pet.Size || h != pet.Size {
				petWin.SetSize(pet.Size, pet.Size)
			}
			x, y := petWin.Position()
			next, changed := pet.KeepOnScreen(pet.Rect{
				X: x, Y: y, Width: pet.Size, Height: pet.Size,
			}, currentPetDisplays())
			if changed {
				petWin.SetPosition(next.X, next.Y)
			}
		}
		clampPet()
		mygo.Screen.OnDisplaysChanged(clampPet)

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

// currentPetDisplays snapshots the monitor geometry for the pet placement
// rules. Display changes are rare, so reading it on each move keeps drag
// behavior correct when a monitor is plugged in or rearranged mid-session.
func currentPetDisplays() []pet.Display {
	displays := mygo.Screen.Displays()
	out := make([]pet.Display, 0, len(displays))
	for _, display := range displays {
		out = append(out, pet.Display{
			Bounds: pet.Rect{
				X:      display.Bounds.X,
				Y:      display.Bounds.Y,
				Width:  display.Bounds.Width,
				Height: display.Bounds.Height,
			},
			WorkArea: pet.Rect{
				X:      display.WorkArea.X,
				Y:      display.WorkArea.Y,
				Width:  display.WorkArea.Width,
				Height: display.WorkArea.Height,
			},
		})
	}
	return out
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
