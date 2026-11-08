// Package buildinfo reports build identity without loading content or dependencies.
package buildinfo

import "runtime"

// These strings are set together in both executables by packaging/build.py.
var (
	Version   = "development"
	SourceSHA = "unknown"
	Target    = ""
	Dirty     = "false"
	Mode      = "development"
)

type Identity struct {
	Version     string `json:"version"`
	SourceSHA   string `json:"source_sha"`
	Target      string `json:"target"`
	Development bool   `json:"development"`
	Dirty       bool   `json:"dirty"`
}

func Current() Identity {
	target := Target
	if target == "" {
		target = runtime.GOOS + "/" + runtime.GOARCH
	}
	return Identity{Version: Version, SourceSHA: SourceSHA, Target: target, Development: Mode != "release" || Dirty == "true", Dirty: Dirty == "true"}
}
