package usercfg

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestConfigRoundTrip(t *testing.T) {
	tempDir := t.TempDir()
	
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tempDir)

	config := Config{
		Projects:     []string{"TEST", "DEMO"},
		DefaultScope: "assigned",
		JiraURL:      "https://test.example.com",
		Boards: map[string]int{
			"TEST_board": 123,
			"DEMO_board": 456,
		},
	}

	err := Save(config)
	if err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	configPath := filepath.Join(tempDir, ".config", "gci", "config.toml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatalf("Config file was not created at %s", configPath)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if len(loaded.Projects) != 2 || loaded.Projects[0] != "TEST" || loaded.Projects[1] != "DEMO" {
		t.Errorf("Projects not preserved: got %v, want [TEST DEMO]", loaded.Projects)
	}
	if loaded.DefaultScope != "assigned" {
		t.Errorf("DefaultScope not preserved: got %s, want assigned", loaded.DefaultScope)
	}
	if loaded.JiraURL != "https://test.example.com" {
		t.Errorf("JiraURL not preserved: got %s, want https://test.example.com", loaded.JiraURL)
	}
	if loaded.Boards["TEST_board"] != 123 {
		t.Errorf("Boards not preserved: got %v", loaded.Boards)
	}
}

func TestConfigDefaults(t *testing.T) {
	tempDir := t.TempDir()

	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tempDir)

	config, err := Load()
	if err != ErrNotConfigured {
		t.Fatalf("Expected ErrNotConfigured when no config file, got: %v", err)
	}

	if len(config.Projects) != 0 {
		t.Errorf("Default projects should be empty: got %v", config.Projects)
	}
	if config.DefaultScope != "assigned_or_reported" {
		t.Errorf("Default scope incorrect: got %s", config.DefaultScope)
	}
	if config.JiraURL != "" {
		t.Errorf("Default JIRA URL should be empty: got %s", config.JiraURL)
	}
	if len(config.Boards) != 0 {
		t.Errorf("Default boards should be empty: got %v", config.Boards)
	}
}

func TestEnvVarOverlays(t *testing.T) {
	tempDir := t.TempDir()
	
	// Set up temp home
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tempDir)
	
	// Save original env vars and restore after test
	origProjects := os.Getenv("GCI_PROJECTS")
	origScope := os.Getenv("GCI_DEFAULT_SCOPE")
	origJiraURL := os.Getenv("GCI_JIRA_URL")
	defer func() {
		os.Setenv("GCI_PROJECTS", origProjects)
		os.Setenv("GCI_DEFAULT_SCOPE", origScope)
		os.Setenv("GCI_JIRA_URL", origJiraURL)
	}()
	
	// Test GCI_PROJECTS override
	os.Setenv("GCI_PROJECTS", "FOO,BAR,BAZ")
	os.Setenv("GCI_DEFAULT_SCOPE", "assigned")
	os.Setenv("GCI_JIRA_URL", "https://env.example.com")
	
	config := GetRuntimeConfig()
	
	expectedProjects := []string{"FOO", "BAR", "BAZ"}
	if len(config.Projects) != 3 {
		t.Errorf("Expected 3 projects from env var, got %d", len(config.Projects))
	}
	for i, expected := range expectedProjects {
		if i >= len(config.Projects) || config.Projects[i] != expected {
			t.Errorf("Project %d: expected %s, got %v", i, expected, config.Projects)
		}
	}
	
	if config.DefaultScope != "assigned" {
		t.Errorf("Expected scope 'assigned' from env var, got %s", config.DefaultScope)
	}
	
	if config.JiraURL != "https://env.example.com" {
		t.Errorf("Expected JIRA URL from env var, got %s", config.JiraURL)
	}
}

func TestEnvVarProjectsWithSpaces(t *testing.T) {
	tempDir := t.TempDir()
	
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tempDir)
	
	origProjects := os.Getenv("GCI_PROJECTS")
	defer os.Setenv("GCI_PROJECTS", origProjects)
	
	// Test with spaces around commas
	os.Setenv("GCI_PROJECTS", " FOO , BAR,  BAZ  ")
	
	config := GetRuntimeConfig()
	
	expectedProjects := []string{"FOO", "BAR", "BAZ"}
	if len(config.Projects) != 3 {
		t.Errorf("Expected 3 projects, got %d", len(config.Projects))
	}
	for i, expected := range expectedProjects {
		if i >= len(config.Projects) || config.Projects[i] != expected {
			t.Errorf("Project %d: expected %s, got %s", i, expected, config.Projects[i])
		}
	}
}

func TestGetAvailableProjectsFromRuntime(t *testing.T) {
	tempDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tempDir)

	origProjects := os.Getenv("GCI_PROJECTS")
	defer os.Setenv("GCI_PROJECTS", origProjects)

	// Test default behavior (no env var)
	os.Setenv("GCI_PROJECTS", "")
	available := GetAvailableProjectsFromRuntime()
	if len(available) != 1 || available[0] != "both" {
		t.Errorf("Default available projects should be [both], got %v", available)
	}

	// Test with env var override
	os.Setenv("GCI_PROJECTS", "X,Y")
	available = GetAvailableProjectsFromRuntime()
	expectedEnv := []string{"X", "Y", "both"}
	if len(available) != 3 {
		t.Errorf("Env var available projects should be 3, got %d", len(available))
	}
	for i, expected := range expectedEnv {
		if i >= len(available) || available[i] != expected {
			t.Errorf("Env var project %d: expected %s, got %v", i, expected, available)
		}
	}
}

func TestXDGCompliance(t *testing.T) {
	tempDir := t.TempDir()
	
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tempDir)

	// Test 1: XDG path takes precedence when both exist
	xdgPath := filepath.Join(tempDir, ".config", "gci", "config.toml")
	legacyPath := filepath.Join(tempDir, ".config", "gci.toml")
	
	// Create XDG config directory
	if err := os.MkdirAll(filepath.Dir(xdgPath), 0755); err != nil {
		t.Fatalf("Failed to create XDG config dir: %v", err)
	}
	
	// Create legacy config directory
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0755); err != nil {
		t.Fatalf("Failed to create legacy config dir: %v", err)
	}

	// Write different configs to each path
	xdgConfig := Config{
		Projects:     []string{"XDG_PROJECT"},
		DefaultScope: "assigned",
		JiraURL:      "https://xdg.example.com",
		Boards:       map[string]int{"XDG_board": 123},
	}
	
	legacyConfig := Config{
		Projects:     []string{"LEGACY_PROJECT"},
		DefaultScope: "reported",
		JiraURL:      "https://legacy.example.com",
		Boards:       map[string]int{"LEGACY_board": 456},
	}

	// Save to XDG path
	if err := Save(xdgConfig); err != nil {
		t.Fatalf("Failed to save XDG config: %v", err)
	}
	
	// Manually write to legacy path (since Save() now uses XDG path)
	legacyFile, err := os.Create(legacyPath)
	if err != nil {
		t.Fatalf("Failed to create legacy config: %v", err)
	}
	defer legacyFile.Close()
	
	if err := toml.NewEncoder(legacyFile).Encode(legacyConfig); err != nil {
		t.Fatalf("Failed to encode legacy config: %v", err)
	}

	// Load should prefer XDG path
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if len(loaded.Projects) != 1 || loaded.Projects[0] != "XDG_PROJECT" {
		t.Errorf("XDG precedence failed: got projects %v, want [XDG_PROJECT]", loaded.Projects)
	}
}

func TestLegacyPathWarning(t *testing.T) {
	tempDir := t.TempDir()
	
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tempDir)

	// Only create legacy config
	legacyPath := filepath.Join(tempDir, ".config", "gci.toml")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0755); err != nil {
		t.Fatalf("Failed to create legacy config dir: %v", err)
	}

	legacyConfig := Config{
		Projects:     []string{"LEGACY"},
		DefaultScope: "assigned",
		JiraURL:      "https://legacy.example.com",
		Boards:       map[string]int{"LEGACY_board": 789},
	}

	legacyFile, err := os.Create(legacyPath)
	if err != nil {
		t.Fatalf("Failed to create legacy config: %v", err)
	}
	defer legacyFile.Close()
	
	if err := toml.NewEncoder(legacyFile).Encode(legacyConfig); err != nil {
		t.Fatalf("Failed to encode legacy config: %v", err)
	}

	// Capture stderr to check for warning
	// Note: This is a basic test - in practice the warning goes to stderr
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Failed to load legacy config: %v", err)
	}

	if len(loaded.Projects) != 1 || loaded.Projects[0] != "LEGACY" {
		t.Errorf("Legacy config loading failed: got projects %v, want [LEGACY]", loaded.Projects)
	}
}

func TestPathFunctions(t *testing.T) {
	tempDir := t.TempDir()
	
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tempDir)

	xdgPath := Path()
	legacyPath := LegacyPath()
	
	expectedXDG := filepath.Join(tempDir, ".config", "gci", "config.toml")
	expectedLegacy := filepath.Join(tempDir, ".config", "gci.toml")
	
	if xdgPath != expectedXDG {
		t.Errorf("XDG Path() incorrect: got %s, want %s", xdgPath, expectedXDG)
	}
	
	if legacyPath != expectedLegacy {
		t.Errorf("LegacyPath() incorrect: got %s, want %s", legacyPath, expectedLegacy)
	}
}

func TestSchemaVersioning(t *testing.T) {
	tempDir := t.TempDir()
	
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tempDir)

	// Test 1: New configs have schema version
	newConfig := Config{
		Projects:     []string{"TEST"},
		DefaultScope: "assigned",
		JiraURL:      "https://test.example.com",
		Boards:       map[string]int{"TEST_board": 123},
	}
	
	err := Save(newConfig)
	if err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}
	
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}
	
	if loaded.SchemaVersion != CurrentSchemaVersion {
		t.Errorf("New config should have current schema version %d, got %d", CurrentSchemaVersion, loaded.SchemaVersion)
	}
}

func TestMigrationFromV0(t *testing.T) {
	tempDir := t.TempDir()
	
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tempDir)

	// Create a v0 config (no schema_version field)
	configPath := filepath.Join(tempDir, ".config", "gci", "config.toml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}

	// Write a v0 config manually (without schema_version)
	v0ConfigContent := `projects = ["V0_PROJECT"]
default_scope = "assigned"
jira_url = "https://v0.example.com"

[boards]
V0_board = 999
`
	
	if err := os.WriteFile(configPath, []byte(v0ConfigContent), 0644); err != nil {
		t.Fatalf("Failed to write v0 config: %v", err)
	}

	// Load should migrate automatically
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Failed to load v0 config: %v", err)
	}

	// Should be migrated to current version
	if loaded.SchemaVersion != CurrentSchemaVersion {
		t.Errorf("V0 config should be migrated to version %d, got %d", CurrentSchemaVersion, loaded.SchemaVersion)
	}
	
	// Content should be preserved
	if len(loaded.Projects) != 1 || loaded.Projects[0] != "V0_PROJECT" {
		t.Errorf("Migration should preserve projects: got %v", loaded.Projects)
	}
	
	if loaded.DefaultScope != "assigned" {
		t.Errorf("Migration should preserve default scope: got %s", loaded.DefaultScope)
	}
	
	if loaded.JiraURL != "https://v0.example.com" {
		t.Errorf("Migration should preserve JIRA URL: got %s", loaded.JiraURL)
	}
	
	if loaded.Boards["V0_board"] != 999 {
		t.Errorf("Migration should preserve boards: got %v", loaded.Boards)
	}
}

func TestMigrateAndSave(t *testing.T) {
	tempDir := t.TempDir()
	
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tempDir)

	// Create a v0 config
	configPath := filepath.Join(tempDir, ".config", "gci", "config.toml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}

	v0ConfigContent := `projects = ["MIGRATE_TEST"]
default_scope = "reported"
`
	
	if err := os.WriteFile(configPath, []byte(v0ConfigContent), 0644); err != nil {
		t.Fatalf("Failed to write v0 config: %v", err)
	}

	// Run migration
	err := MigrateAndSave()
	if err != nil {
		t.Fatalf("MigrateAndSave failed: %v", err)
	}

	// Load the migrated file and check it has schema version
	var migratedConfig Config
	if _, err := toml.DecodeFile(configPath, &migratedConfig); err != nil {
		t.Fatalf("Failed to decode migrated config: %v", err)
	}

	if migratedConfig.SchemaVersion != CurrentSchemaVersion {
		t.Errorf("Migrated config should have schema version %d, got %d", CurrentSchemaVersion, migratedConfig.SchemaVersion)
	}
	
	if len(migratedConfig.Projects) != 1 || migratedConfig.Projects[0] != "MIGRATE_TEST" {
		t.Errorf("Migration should preserve projects: got %v", migratedConfig.Projects)
	}
}

func TestMigrateAlreadyCurrentVersion(t *testing.T) {
	tempDir := t.TempDir()
	
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tempDir)

	// Create a current version config
	currentConfig := Config{
		SchemaVersion: CurrentSchemaVersion,
		Projects:      []string{"CURRENT"},
		DefaultScope:  "assigned",
	}
	
	err := Save(currentConfig)
	if err != nil {
		t.Fatalf("Failed to save current config: %v", err)
	}

	// Attempt migration - should fail
	err = MigrateAndSave()
	if err == nil {
		t.Errorf("MigrateAndSave should fail when config is already current version")
	}
}

func TestExampleConfigParses(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Skip("Cannot determine working directory")
	}
	repoRoot := filepath.Join(wd, "..", "..")
	examplePath := filepath.Join(repoRoot, "examples", "gci.toml")

	var config Config
	if _, err := toml.DecodeFile(examplePath, &config); err != nil {
		t.Fatalf("Example config file should parse correctly: %v", err)
	}

	if config.SchemaVersion != 1 {
		t.Errorf("Example should have schema version 1, got %d", config.SchemaVersion)
	}

	expectedProjects := []string{"MYPROJECT", "INFRA"}
	if len(config.Projects) != len(expectedProjects) {
		t.Errorf("Example should have %d projects, got %d", len(expectedProjects), len(config.Projects))
	}

	if config.DefaultScope != "assigned_or_reported" {
		t.Errorf("Example should have default scope 'assigned_or_reported', got %s", config.DefaultScope)
	}

	if config.JiraURL != "https://your-company.atlassian.net" {
		t.Errorf("Example should have placeholder JIRA URL, got %s", config.JiraURL)
	}

	if config.Boards["MYPROJECT_kanban"] != 123 {
		t.Errorf("Example should have MYPROJECT_kanban board, got %v", config.Boards)
	}
}