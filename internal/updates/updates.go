// Package updates owns the application's update state: checking the
// signed release feed, downloading and installing a newer version, and
// publishing progress to the window. MyGo performs the network, signature
// and replacement work; this package adds the state machine and the
// friendly behavior around it.
//
// All methods are safe to call from the UI thread or a background
// goroutine. The UI reads Snapshot and is redrawn through the notify
// callback; it never waits on a check or a download.
package updates

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/egoist/mygo"
)

// ErrBusy reports that a check or install is already in progress.
var ErrBusy = errors.New("updates: an update operation is already in progress")

// ErrNoUpdate reports that there is no found release to install.
var ErrNoUpdate = errors.New("updates: no update is available to install")

// checkTimeout bounds one background check: a feed that stops responding
// must not leave the UI "checking" indefinitely. Manual calls of Check keep
// the caller's context, which can carry its own deadline.
const checkTimeout = 60 * time.Second

// Release is a newer version found in the update feed.
type Release struct {
	Version string
	// Notes is the Markdown release notes of the version.
	Notes string
	Date  time.Time
	// update is MyGo's installable handle. It stays inside this package;
	// tests and the UI see only the release's public description.
	update *mygo.Update
}

// Source is the update feed and installer the manager drives. The real
// implementation wraps mygo.Updater; tests provide a fake.
type Source interface {
	// Enabled reports whether this build can update itself at all.
	Enabled() bool
	// Check returns a newer release, or nil when the app is up to date.
	Check(ctx context.Context) (*Release, error)
	// Install downloads, verifies and installs release.
	Install(ctx context.Context, release *Release, progress func(downloaded, total int64)) error
}

// MyGo is the production Source, backed by MyGo's signed updater.
type MyGo struct{}

// Enabled reports whether this build was packaged with an update feed and
// can write to its installation.
func (MyGo) Enabled() bool { return mygo.Updater.Enabled() }

// Check asks MyGo's configured feed for a newer version.
func (MyGo) Check(ctx context.Context) (*Release, error) {
	up, err := mygo.Updater.Check(ctx)
	if err != nil || up == nil {
		return nil, err
	}
	return &Release{Version: up.Version, Notes: up.Notes, Date: up.Date, update: up}, nil
}

// Install downloads and installs the release. MyGo verifies the signature
// against the public key built into the application.
func (MyGo) Install(ctx context.Context, release *Release, progress func(downloaded, total int64)) error {
	if release == nil || release.update == nil {
		return errors.New("updates: the release carries no installable update")
	}
	return release.update.Install(ctx, progress)
}

// Phase is where the update flow stands.
type Phase int

const (
	// PhaseUnavailable means this build cannot update itself: a development
	// run, a build without an update feed, or an installation the user
	// cannot write to.
	PhaseUnavailable Phase = iota
	// PhaseIdle means no check has run yet.
	PhaseIdle
	// PhaseChecking means a check is in flight.
	PhaseChecking
	// PhaseUpToDate means the running version is current.
	PhaseUpToDate
	// PhaseAvailable means a newer version was found.
	PhaseAvailable
	// PhaseDownloading means the found update is being downloaded and
	// installed.
	PhaseDownloading
	// PhaseInstalled means the update was installed and waits for a restart.
	PhaseInstalled
	// PhaseFailed means the last check or install failed.
	PhaseFailed
)

// String returns a stable name for logs and tests.
func (p Phase) String() string {
	switch p {
	case PhaseUnavailable:
		return "unavailable"
	case PhaseIdle:
		return "idle"
	case PhaseChecking:
		return "checking"
	case PhaseUpToDate:
		return "up-to-date"
	case PhaseAvailable:
		return "available"
	case PhaseDownloading:
		return "downloading"
	case PhaseInstalled:
		return "installed"
	case PhaseFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// Snapshot is the update state the window renders.
type Snapshot struct {
	Phase          Phase
	CurrentVersion string
	// Release is the newer version found, while available, downloading or
	// after a failed install. It is nil otherwise.
	Release *Release
	// Downloaded and Total are bytes; Total is 0 until the server reports a
	// size.
	Downloaded, Total int64
	// Progress is downloaded/total between 0 and 1, or 0 when unknown.
	Progress float64
	// Err is a user-facing failure description, empty unless Phase is
	// PhaseFailed.
	Err string
	// LastChecked is when the most recent check finished.
	LastChecked time.Time
	// Busy is whether a check or install is running.
	Busy bool
}

// Manager coordinates update checks and installs for one running app.
type Manager struct {
	mu     sync.Mutex
	source Source
	now    func() time.Time

	snap      Snapshot
	autoCheck bool
	notify    func()
	installed func()
}

// New creates a manager for the running version. A build that cannot
// update itself starts in PhaseUnavailable and stays there; no network
// request is made.
func New(source Source, currentVersion string) *Manager {
	phase := PhaseIdle
	if !source.Enabled() {
		phase = PhaseUnavailable
	}
	return &Manager{
		source: source,
		now:    time.Now,
		snap: Snapshot{
			Phase:          phase,
			CurrentVersion: currentVersion,
		},
		autoCheck: true,
	}
}

// SetNotify sets the callback that asks the window to redraw after a state
// change. It runs outside the manager's lock. Set it before starting a
// check; a nil callback is allowed in tests.
func (m *Manager) SetNotify(fn func()) {
	m.mu.Lock()
	m.notify = fn
	m.mu.Unlock()
}

// SetInstalled sets the callback run after an update is installed. The
// entry point relaunches the app from it, on the UI thread.
func (m *Manager) SetInstalled(fn func()) {
	m.mu.Lock()
	m.installed = fn
	m.mu.Unlock()
}

// SetAutoCheck turns scheduled checks on or off. The preference is
// persisted by the caller.
func (m *Manager) SetAutoCheck(enabled bool) {
	m.mu.Lock()
	m.autoCheck = enabled
	m.mu.Unlock()
}

// AutoCheck reports whether scheduled checks are on.
func (m *Manager) AutoCheck() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.autoCheck
}

// Snapshot returns a copy of the current state.
func (m *Manager) Snapshot() Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.snap
}

// CheckAsync starts a check in the background. It does nothing when the
// build cannot update or another operation is running.
func (m *Manager) CheckAsync() {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
		defer cancel()
		_ = m.Check(ctx)
	}()
}

// InstallAsync downloads and installs the found release in the
// background. It does nothing when there is no release or another
// operation is running.
func (m *Manager) InstallAsync() {
	go func() {
		_ = m.Install(context.Background())
	}()
}

// Check runs one update check synchronously. The manager records the
// outcome in its snapshot and returns the error, including
// mygo.ErrUpdatesDisabled for builds without updates.
func (m *Manager) Check(ctx context.Context) error {
	m.mu.Lock()
	if m.snap.Phase == PhaseUnavailable {
		m.mu.Unlock()
		return mygo.ErrUpdatesDisabled
	}
	if m.snap.Busy {
		m.mu.Unlock()
		return ErrBusy
	}
	m.snap.Busy = true
	m.snap.Phase = PhaseChecking
	m.snap.Err = ""
	current := m.snap.CurrentVersion
	m.mu.Unlock()
	log.Printf("lucia: update check: checking (running %s)", current)
	m.changed()

	release, err := m.source.Check(ctx)

	m.mu.Lock()
	m.snap.Busy = false
	m.snap.LastChecked = m.now()
	switch {
	case err != nil && errors.Is(err, mygo.ErrUpdatesDisabled):
		m.snap.Phase = PhaseUnavailable
		m.snap.Release = nil
	case err != nil:
		// Keep a previously found release: a transient feed failure must not
		// take away an update the user can still install.
		m.snap.Phase = PhaseFailed
		m.snap.Err = fmt.Sprintf("检查更新失败：%v", err)
	case release == nil:
		m.snap.Phase = PhaseUpToDate
		m.snap.Release = nil
	default:
		m.snap.Phase = PhaseAvailable
		m.snap.Release = release
	}
	m.mu.Unlock()
	switch {
	case err != nil && errors.Is(err, mygo.ErrUpdatesDisabled):
		log.Printf("lucia: update check: updates disabled")
	case err != nil:
		log.Printf("lucia: update check failed: %v", err)
	case release == nil:
		log.Printf("lucia: update check: no version newer than %s", current)
	default:
		log.Printf("lucia: update check: found %s (running %s)", release.Version, current)
	}
	m.changed()
	return err
}

// Install runs one download and install synchronously, reporting progress
// through the snapshot. The installed callback runs after a successful
// install, outside the lock.
func (m *Manager) Install(ctx context.Context) error {
	m.mu.Lock()
	if m.snap.Busy {
		m.mu.Unlock()
		return ErrBusy
	}
	release := m.snap.Release
	if release == nil {
		m.mu.Unlock()
		return ErrNoUpdate
	}
	m.snap.Busy = true
	m.snap.Phase = PhaseDownloading
	m.snap.Err = ""
	m.snap.Downloaded, m.snap.Total, m.snap.Progress = 0, 0, 0
	m.mu.Unlock()
	log.Printf("lucia: update install: downloading and verifying %s", release.Version)
	m.changed()

	// MyGo may report the final byte twice (the last chunk and EOF); log the
	// download once per attempted transfer, including a delta-to-archive
	// fallback that starts a new one with another total.
	var lastTotal int64
	var downloadLogged bool
	err := m.source.Install(ctx, release, func(downloaded, total int64) {
		m.mu.Lock()
		m.snap.Downloaded, m.snap.Total = downloaded, total
		if total > 0 {
			m.snap.Progress = min(max(float64(downloaded)/float64(total), 0), 1)
		}
		if total != lastTotal {
			lastTotal = total
			downloadLogged = false
		}
		complete := total > 0 && downloaded >= total && !downloadLogged
		if complete {
			downloadLogged = true
		}
		m.mu.Unlock()
		if complete {
			log.Printf("lucia: update install: downloaded %d bytes", downloaded)
		}
		m.changed()
	})

	m.mu.Lock()
	m.snap.Busy = false
	if err != nil {
		m.snap.Phase = PhaseFailed
		m.snap.Err = fmt.Sprintf("安装更新失败：%v", err)
		m.snap.Progress = 0
		m.mu.Unlock()
		log.Printf("lucia: update install failed: %v", err)
		m.changed()
		return err
	}
	m.snap.Phase = PhaseInstalled
	m.snap.Progress = 1
	installed := m.installed
	m.mu.Unlock()
	log.Printf("lucia: update install: %s installed", release.Version)
	m.changed()
	if installed != nil {
		installed()
	}
	return nil
}

// StartAutoChecks checks now when automatic checks are on, then repeats
// every interval until ctx is canceled. Turning the preference off stops
// the scheduled checks; the window starts a check when the user turns it
// back on.
func (m *Manager) StartAutoChecks(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		return
	}
	if m.AutoCheck() {
		m.CheckAsync()
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if m.AutoCheck() {
					m.CheckAsync()
				}
			}
		}
	}()
}

func (m *Manager) changed() {
	m.mu.Lock()
	notify := m.notify
	m.mu.Unlock()
	if notify != nil {
		notify()
	}
}
