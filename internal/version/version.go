package version

import (
	"fmt"
	"runtime"
)

// Build-time variables set via ldflags
var (
	// Version is the semantic version of the application
	Version = "dev"
	
	// Commit is the git commit hash
	Commit = "unknown"
	
	// Date is the build date
	Date = "unknown"
	
	// GoVersion is the Go version used to build
	GoVersion = runtime.Version()
)

// BuildInfo contains version and build information
type BuildInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	Date      string `json:"date"`
	GoVersion string `json:"go_version"`
	Platform  string `json:"platform"`
}

// GetBuildInfo returns structured build information
func GetBuildInfo() BuildInfo {
	return BuildInfo{
		Version:   Version,
		Commit:    Commit,
		Date:      Date,
		GoVersion: GoVersion,
		Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}

// GetVersionString returns a formatted version string
func GetVersionString() string {
	info := GetBuildInfo()
	if info.Version == "dev" {
		return fmt.Sprintf("gci %s (%s) built with %s on %s", 
			info.Version, info.Commit, info.GoVersion, info.Platform)
	}
	return fmt.Sprintf("gci %s (%s) built on %s with %s for %s", 
		info.Version, info.Commit, info.Date, info.GoVersion, info.Platform)
}

// GetShortVersion returns just the version number
func GetShortVersion() string {
	return Version
}