package updates

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo"
)

type fakeSource struct {
	enabled bool

	checkFn   func(context.Context) (*Release, error)
	installFn func(context.Context, *Release, func(int64, int64)) error

	mu      sync.Mutex
	checked int
}

func (f *fakeSource) Enabled() bool { return f.enabled }

func (f *fakeSource) Check(ctx context.Context) (*Release, error) {
	f.mu.Lock()
	f.checked++
	f.mu.Unlock()
	if f.checkFn == nil {
		return nil, nil
	}
	return f.checkFn(ctx)
}

func (f *fakeSource) Install(ctx context.Context, r *Release, progress func(int64, int64)) error {
	if f.installFn == nil {
		return nil
	}
	return f.installFn(ctx, r, progress)
}

func (f *fakeSource) checks() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.checked
}

func TestDisabledBuildNeverChecks(t *testing.T) {
	src := &fakeSource{enabled: false}
	m := New(src, "0.0.1")
	if got := m.Snapshot().Phase; got != PhaseUnavailable {
		t.Fatalf("phase %v, want unavailable", got)
	}
	if err := m.Check(context.Background()); !errors.Is(err, mygo.ErrUpdatesDisabled) {
		t.Fatalf("Check error %v, want ErrUpdatesDisabled", err)
	}
	if src.checks() != 0 {
		t.Errorf("source checked %d times, want 0", src.checks())
	}
}

func TestCheckFindsRelease(t *testing.T) {
	date := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	src := &fakeSource{
		enabled: true,
		checkFn: func(context.Context) (*Release, error) {
			return &Release{Version: "0.2.0", Notes: "新版本", Date: date}, nil
		},
	}
	m := New(src, "0.0.1")
	var notices int
	m.SetNotify(func() { notices++ })
	if err := m.Check(context.Background()); err != nil {
		t.Fatalf("Check: %v", err)
	}
	snap := m.Snapshot()
	if snap.Phase != PhaseAvailable || snap.Release == nil {
		t.Fatalf("snapshot %+v, want an available release", snap)
	}
	if snap.Release.Version != "0.2.0" || snap.Release.Notes != "新版本" {
		t.Errorf("release %+v, want 0.2.0 with notes", snap.Release)
	}
	if snap.CurrentVersion != "0.0.1" {
		t.Errorf("current version %q", snap.CurrentVersion)
	}
	if snap.LastChecked.IsZero() {
		t.Error("LastChecked was not set")
	}
	if notices < 2 {
		t.Errorf("notify ran %d times, want at least the start and the result", notices)
	}
}

func TestCheckUpToDate(t *testing.T) {
	src := &fakeSource{enabled: true}
	m := New(src, "0.0.1")
	if err := m.Check(context.Background()); err != nil {
		t.Fatalf("Check: %v", err)
	}
	snap := m.Snapshot()
	if snap.Phase != PhaseUpToDate || snap.Release != nil {
		t.Fatalf("snapshot %+v, want up to date without a release", snap)
	}
}

func TestCheckFailureIsVisible(t *testing.T) {
	src := &fakeSource{
		enabled: true,
		checkFn: func(context.Context) (*Release, error) {
			return nil, errors.New("network down")
		},
	}
	m := New(src, "0.0.1")
	if err := m.Check(context.Background()); err == nil {
		t.Fatal("Check did not report the failure")
	}
	snap := m.Snapshot()
	if snap.Phase != PhaseFailed {
		t.Fatalf("phase %v, want failed", snap.Phase)
	}
	if !strings.Contains(snap.Err, "检查更新失败") || !strings.Contains(snap.Err, "network down") {
		t.Errorf("error %q does not explain the failure", snap.Err)
	}
	if snap.Busy {
		t.Error("the manager stays busy after a failure")
	}
}

func TestCheckFailureKeepsPreviousRelease(t *testing.T) {
	var failing bool
	src := &fakeSource{
		enabled: true,
		checkFn: func(context.Context) (*Release, error) {
			if failing {
				return nil, errors.New("network down")
			}
			return &Release{Version: "0.2.0"}, nil
		},
	}
	m := New(src, "0.0.1")
	if err := m.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	failing = true
	if err := m.Check(context.Background()); err == nil {
		t.Fatal("the second check did not report the failure")
	}
	snap := m.Snapshot()
	if snap.Phase != PhaseFailed || snap.Release == nil || snap.Release.Version != "0.2.0" {
		t.Errorf("snapshot %+v, want the previous 0.2.0 release kept after a failed re-check", snap)
	}
}

func TestCheckRejectsConcurrentRuns(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	src := &fakeSource{
		enabled: true,
		checkFn: func(context.Context) (*Release, error) {
			close(started)
			<-release
			return nil, nil
		},
	}
	m := New(src, "0.0.1")
	done := make(chan error, 1)
	go func() { done <- m.Check(context.Background()) }()
	<-started
	if err := m.Check(context.Background()); !errors.Is(err, ErrBusy) {
		t.Fatalf("concurrent Check error %v, want ErrBusy", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("first Check: %v", err)
	}
}

func TestInstallReportsProgressAndRestarts(t *testing.T) {
	src := &fakeSource{
		enabled: true,
		checkFn: func(context.Context) (*Release, error) {
			return &Release{Version: "0.2.0"}, nil
		},
		installFn: func(_ context.Context, r *Release, progress func(int64, int64)) error {
			if r.Version != "0.2.0" {
				t.Errorf("installed %q, want 0.2.0", r.Version)
			}
			progress(50, 100)
			return nil
		},
	}
	m := New(src, "0.0.1")
	if err := m.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	var restarted bool
	m.SetInstalled(func() { restarted = true })
	if err := m.Install(context.Background()); err != nil {
		t.Fatalf("Install: %v", err)
	}
	snap := m.Snapshot()
	if snap.Phase != PhaseInstalled || snap.Progress != 1 {
		t.Fatalf("snapshot %+v, want installed with full progress", snap)
	}
	if !restarted {
		t.Error("the installed callback did not run")
	}
}

func TestInstallFailureKeepsTheReleaseForRetry(t *testing.T) {
	src := &fakeSource{
		enabled: true,
		checkFn: func(context.Context) (*Release, error) {
			return &Release{Version: "0.2.0"}, nil
		},
		installFn: func(context.Context, *Release, func(int64, int64)) error {
			return errors.New("signature mismatch")
		},
	}
	m := New(src, "0.0.1")
	if err := m.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := m.Install(context.Background()); err == nil {
		t.Fatal("Install did not report the failure")
	}
	snap := m.Snapshot()
	if snap.Phase != PhaseFailed {
		t.Fatalf("phase %v, want failed", snap.Phase)
	}
	if snap.Release == nil || snap.Release.Version != "0.2.0" {
		t.Errorf("release %+v, want the found version kept for a retry", snap.Release)
	}
	if !strings.Contains(snap.Err, "安装更新失败") {
		t.Errorf("error %q does not explain the install failure", snap.Err)
	}
	if snap.Busy {
		t.Error("the manager stays busy after a failed install")
	}
}

func TestInstallWithoutRelease(t *testing.T) {
	m := New(&fakeSource{enabled: true}, "0.0.1")
	if err := m.Install(context.Background()); !errors.Is(err, ErrNoUpdate) {
		t.Fatalf("Install error %v, want ErrNoUpdate", err)
	}
}

func TestAutoCheckPreference(t *testing.T) {
	m := New(&fakeSource{enabled: true}, "0.0.1")
	if !m.AutoCheck() {
		t.Error("automatic checks should start on")
	}
	m.SetAutoCheck(false)
	if m.AutoCheck() {
		t.Error("automatic checks did not turn off")
	}
}

func TestStartAutoChecksChecksImmediately(t *testing.T) {
	var mu sync.Mutex
	count := 0
	src := &fakeSource{
		enabled: true,
		checkFn: func(context.Context) (*Release, error) {
			mu.Lock()
			count++
			mu.Unlock()
			return nil, nil
		},
	}
	m := New(src, "0.0.1")
	ctx, cancel := context.WithCancel(context.Background())
	m.StartAutoChecks(ctx, time.Hour)
	t.Cleanup(cancel)
	deadline := time.After(2 * time.Second)
	for {
		mu.Lock()
		n := count
		mu.Unlock()
		if n == 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("checks %d after startup, want 1", n)
		case <-time.After(10 * time.Millisecond):
		}
	}
	m.SetAutoCheck(false)
	cancel()
}
