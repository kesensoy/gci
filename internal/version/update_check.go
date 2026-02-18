package version

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	semver "github.com/Masterminds/semver/v3"
	selfupdate "github.com/creativeprojects/go-selfupdate"
)

const (
	updateCheckTTL  = 24 * time.Hour
	updateCacheFile = "update_check.json"
	githubSlug      = "kesensoy/gci"
)

// UpdateCheckResult holds the outcome of a background update check.
type UpdateCheckResult struct {
	NewVersion string // empty means no update available (or check skipped/failed)
}

type updateCache struct {
	LatestVersion  string    `json:"latest_version"`
	CheckedVersion string    `json:"checked_version"` // version that was running when we last checked
	Timestamp      time.Time `json:"timestamp"`
}

// StartUpdateCheck launches a background goroutine that checks for updates.
// Returns a channel that will receive exactly one result.
func StartUpdateCheck() <-chan UpdateCheckResult {
	ch := make(chan UpdateCheckResult, 1)
	go func() {
		defer close(ch)
		newVer := checkForUpdate(GetShortVersion())
		ch <- UpdateCheckResult{NewVersion: newVer}
	}()
	return ch
}

func checkForUpdate(current string) string {
	if current == "dev" {
		return ""
	}

	// Try cache first — but invalidate if user has updated since last check
	if cached, checkedVer, ok := loadUpdateCache(); ok && checkedVer == current {
		if cached != "" && isNewerThan(cached, current) {
			return cached
		}
		return ""
	}

	// Cache miss, stale, or user updated — query GitHub
	source, err := selfupdate.NewGitHubSource(selfupdate.GitHubConfig{})
	if err != nil {
		return ""
	}

	updater, err := selfupdate.NewUpdater(selfupdate.Config{
		Source:    source,
		Validator: &selfupdate.ChecksumValidator{UniqueFilename: "checksums.txt"},
	})
	if err != nil {
		return ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	latest, found, err := updater.DetectLatest(ctx, selfupdate.ParseSlug(githubSlug))
	if err != nil || !found {
		// Cache current version so we don't hammer GitHub when offline
		saveUpdateCache(current, current)
		return ""
	}

	latestVer := latest.Version()
	saveUpdateCache(latestVer, current)

	if latest.LessOrEqual(current) {
		return ""
	}
	return latestVer
}

func isNewerThan(latest, current string) bool {
	lv, err := semver.NewVersion(latest)
	if err != nil {
		return false
	}
	cv, err := semver.NewVersion(current)
	if err != nil {
		return false
	}
	return lv.GreaterThan(cv)
}

// Cache helpers — inner functions take a path for testability.

func updateCachePath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(homeDir, ".config", "gci", updateCacheFile)
}

func loadUpdateCache() (string, string, bool) {
	return loadUpdateCacheFrom(updateCachePath())
}

func saveUpdateCache(latestVersion, checkedVersion string) {
	saveUpdateCacheTo(updateCachePath(), latestVersion, checkedVersion)
}

func loadUpdateCacheFrom(path string) (string, string, bool) {
	if path == "" {
		return "", "", false
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", false
	}

	var cache updateCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return "", "", false
	}

	if time.Since(cache.Timestamp) > updateCheckTTL {
		return "", "", false
	}

	return cache.LatestVersion, cache.CheckedVersion, true
}

func saveUpdateCacheTo(path string, latestVersion, checkedVersion string) {
	if path == "" {
		return
	}

	cache := updateCache{
		LatestVersion:  latestVersion,
		CheckedVersion: checkedVersion,
		Timestamp:      time.Now(),
	}

	data, err := json.Marshal(cache)
	if err != nil {
		return
	}

	os.MkdirAll(filepath.Dir(path), 0755)
	os.WriteFile(path, data, 0644)
}
