// Package buildinfo reports the identity of the running binary: its
// application version, the source revision Go stamped into it, and the
// platform. The first diagnostic log line carries all three so a user's
// report can be tied to an exact build.
package buildinfo

import (
	"runtime"
	"runtime/debug"
	"strings"
)

// Info identifies one build of the application.
type Info struct {
	// Version is the application version, from MyGo's packaged metadata or
	// the development configuration.
	Version string
	// Commit is the full VCS revision Go stamped into the binary, empty when
	// the binary was built outside a checkout.
	Commit string
	// Modified is whether the checkout had uncommitted or untracked changes
	// when the binary was built.
	Modified bool
}

// Current returns the build identity of this process.
func Current(appVersion string) Info {
	info := Info{Version: appVersion}
	if info.Version == "" {
		info.Version = "dev"
	}
	build, ok := debug.ReadBuildInfo()
	if !ok {
		return info
	}
	for _, setting := range build.Settings {
		switch setting.Key {
		case "vcs.revision":
			info.Commit = setting.Value
		case "vcs.modified":
			info.Modified = setting.Value == "true"
		}
	}
	return info
}

// CommitLabel is the short revision shown in the window and logs, marked
// when the build came from a modified checkout.
func (i Info) CommitLabel() string {
	if i.Commit == "" {
		return "unknown"
	}
	short := i.Commit
	if len(short) > 12 {
		short = short[:12]
	}
	if i.Modified {
		short += "-modified"
	}
	return short
}

// Platform is GOOS/GOARCH of the running binary.
func Platform() string {
	return runtime.GOOS + "/" + runtime.GOARCH
}

// LogLine is the first line every run writes to the diagnostic log.
func (i Info) LogLine() string {
	return strings.Join([]string{
		"version=" + i.Version,
		"commit=" + i.CommitLabel(),
		"platform=" + Platform(),
	}, " ")
}
