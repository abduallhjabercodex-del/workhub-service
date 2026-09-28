// internal/infrastructure/version/version.go
package version

import (
	"fmt"
	"runtime"
	"runtime/debug"
)

// These values are injected at build time via -ldflags:
//
//	go build -ldflags "\
//	  -X github.com/org/workhub/internal/version.Version=$(VERSION) \
//	  -X github.com/org/workhub/internal/version.Commit=$(COMMIT) \
//	  -X github.com/org/workhub/internal/version.BuildTime=$(BUILD_TIME)"
var (
	Version   = "dev"     // e.g. v0.3.0
	Commit    = "none"    // git SHA
	BuildTime = "unknown" // RFC3339
)

// Info holds all version information for display/reporting.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"build_time"`
	GoVersion string `json:"go_version"`
	Platform  string `json:"platform"`
}

// Get returns the current build info.
func Get() Info {
	return Info{
		Version:   Version,
		Commit:    Commit,
		BuildTime: BuildTime,
		GoVersion: runtime.Version(),
		Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}

// String returns a compact single-line representation.
func String() string {
	return fmt.Sprintf("%s (commit=%s, built=%s, %s, %s)",
		Version, shortCommit(), BuildTime, runtime.Version(), runtime.GOOS+"/"+runtime.GOARCH)
}

// shortCommit returns the first 7 chars of Commit, or "none".
func shortCommit() string {
	if len(Commit) >= 7 {
		return Commit[:7]
	}
	if Commit == "" {
		return "none"
	}
	return Commit
}

// FullCommit returns Commit, falling back to debug.ReadBuildInfo if unset.
func FullCommit() string {
	if Commit != "" {
		return Commit
	}
	// Fallback: read VCS info baked in by Go 1.18+
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" {
				return s.Value
			}
		}
	}
	return "unknown"
}
