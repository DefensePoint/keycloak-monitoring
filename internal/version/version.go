package version

import (
	"fmt"
	"runtime"
)

var (
	// Version is the current version of the Keycloak Monitoring Tool
	Version = "0.1.0"

	// GitCommit is the git commit hash (set during build)
	GitCommit = "unknown"

	// BuildDate is the build date (set during build)
	BuildDate = "unknown"
)

// Info represents detailed version information.
// Sensitive fields (commit, build date, runtime/platform) must not be returned
// to unauthenticated clients — see PublicInfo for the safe response.
type Info struct {
	Version   string `json:"version"`
	GitCommit string `json:"git_commit"`
	BuildDate string `json:"build_date"`
	GoVersion string `json:"go_version"`
	Platform  string `json:"platform"`
}

// PublicInfo is the version payload safe to return without authentication.
// It exposes only the semantic version so attackers cannot fingerprint the
// exact commit or runtime to correlate with known CVEs.
type PublicInfo struct {
	Version string `json:"version"`
}

// Get returns the full version information (authenticated callers only).
func Get() Info {
	return Info{
		Version:   Version,
		GitCommit: GitCommit,
		BuildDate: BuildDate,
		GoVersion: runtime.Version(),
		Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}

// GetPublic returns version information safe to expose without authentication.
func GetPublic() PublicInfo {
	return PublicInfo{Version: Version}
}

// String returns a formatted version string
func (i Info) String() string {
	return fmt.Sprintf("kmt %s (commit: %s, built: %s, %s, %s)",
		i.Version, i.GitCommit, i.BuildDate, i.GoVersion, i.Platform)
}
