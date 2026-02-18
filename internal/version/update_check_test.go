package version

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIsNewerThan(t *testing.T) {
	tests := []struct {
		latest, current string
		want            bool
	}{
		{"1.2.0", "1.1.0", true},
		{"1.0.0", "1.1.0", false},
		{"1.1.0", "1.1.0", false},
		{"2.0.0", "1.9.9", true},
		{"0.1.0", "0.0.9", true},
		{"invalid", "1.0.0", false},
		{"1.0.0", "invalid", false},
		{"", "", false},
	}
	for _, tt := range tests {
		if got := isNewerThan(tt.latest, tt.current); got != tt.want {
			t.Errorf("isNewerThan(%q, %q) = %v, want %v", tt.latest, tt.current, got, tt.want)
		}
	}
}

func TestLoadSaveCache(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "update_check.json")

	// No file yet — should return false
	if _, ok := loadUpdateCacheFrom(path); ok {
		t.Fatal("expected cache miss for nonexistent file")
	}

	// Write cache
	saveUpdateCacheTo(path, "1.2.0")

	// Read it back
	ver, ok := loadUpdateCacheFrom(path)
	if !ok {
		t.Fatal("expected cache hit after save")
	}
	if ver != "1.2.0" {
		t.Errorf("got cached version %q, want %q", ver, "1.2.0")
	}
}

func TestCacheExpiry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "update_check.json")

	// Write a cache entry with an old timestamp
	cache := updateCache{
		LatestVersion: "1.2.0",
		Timestamp:     time.Now().Add(-25 * time.Hour),
	}
	data, _ := json.Marshal(cache)
	os.WriteFile(path, data, 0644)

	if _, ok := loadUpdateCacheFrom(path); ok {
		t.Fatal("expected cache miss for stale entry")
	}
}

func TestCheckForUpdate_DevBuild(t *testing.T) {
	result := checkForUpdate("dev")
	if result != "" {
		t.Errorf("expected empty result for dev build, got %q", result)
	}
}

func TestLoadCacheFrom_EmptyPath(t *testing.T) {
	if _, ok := loadUpdateCacheFrom(""); ok {
		t.Fatal("expected cache miss for empty path")
	}
}

func TestSaveCacheTo_EmptyPath(t *testing.T) {
	// Should not panic
	saveUpdateCacheTo("", "1.0.0")
}

func TestLoadCacheFrom_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "update_check.json")
	os.WriteFile(path, []byte("not json"), 0644)

	if _, ok := loadUpdateCacheFrom(path); ok {
		t.Fatal("expected cache miss for invalid JSON")
	}
}
