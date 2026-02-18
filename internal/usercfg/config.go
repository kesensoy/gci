package usercfg

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gci/internal/errors"
	"github.com/BurntSushi/toml"
)

// ErrNotConfigured is returned when no config file exists and no env vars are set.
var ErrNotConfigured = fmt.Errorf("gci is not configured; run: gci setup")

// IsConfigured returns true if a config file exists or essential env vars are set.
func IsConfigured() bool {
	if os.Getenv("GCI_JIRA_URL") != "" && os.Getenv("GCI_PROJECTS") != "" {
		return true
	}
	configPath := Path()
	legacyPath := LegacyPath()
	if configPath != "" {
		if _, err := os.Stat(configPath); err == nil {
			return true
		}
	}
	if legacyPath != "" {
		if _, err := os.Stat(legacyPath); err == nil {
			return true
		}
	}
	return false
}

type Config struct {
	SchemaVersion     int               `toml:"schema_version,omitempty"`
	Projects          []string          `toml:"projects"`
	DefaultScope      string            `toml:"default_scope"`
	JiraURL           string            `toml:"jira_url"`
	Boards            map[string]int    `toml:"boards"`
	UIPrefs           UIPreferences     `toml:"ui_prefs,omitempty"`
	EnableClaude      *bool             `toml:"enable_claude"`
	EnableWorktrees   *bool             `toml:"enable_worktrees"`
	OPJiraTokenPath   string            `toml:"op_jira_token_path,omitempty"`
	EmailDomainMap    map[string]string `toml:"email_domain_map,omitempty"`
}

type UIPreferences struct {
	LastScope       string `toml:"last_scope,omitempty"`
	LastFilter      string `toml:"last_filter,omitempty"`
	ColumnWidths    []int  `toml:"column_widths,omitempty"`
	LastSelectedCol int    `toml:"last_selected_col,omitempty"`
	FuzzySearch     bool   `toml:"fuzzy_search,omitempty"`
	ShowExtraFields bool   `toml:"show_extra_fields,omitempty"`
}

const CurrentSchemaVersion = 1

func Path() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	// Prefer XDG-compliant path: ~/.config/gci/config.toml
	return filepath.Join(homeDir, ".config", "gci", "config.toml")
}

func LegacyPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	// Legacy path for backward compatibility
	return filepath.Join(homeDir, ".config", "gci.toml")
}

func Load() (Config, error) {
	configPath := Path()
	legacyPath := LegacyPath()

	if configPath == "" || legacyPath == "" {
		return getDefaults(), errors.NewConfigError("load", fmt.Errorf("unable to determine home directory"))
	}

	var actualPath string
	var warnLegacy bool

	// Check XDG-compliant path first
	if _, err := os.Stat(configPath); err == nil {
		actualPath = configPath
	} else if _, err := os.Stat(legacyPath); err == nil {
		// Fall back to legacy path if XDG path doesn't exist
		actualPath = legacyPath
		warnLegacy = true
	} else {
		// Neither path exists -- not configured
		return getDefaults(), ErrNotConfigured
	}

	var config Config
	if _, err := toml.DecodeFile(actualPath, &config); err != nil {
		return getDefaults(), errors.NewConfigError("load", fmt.Errorf("failed to decode config file: %v", err))
	}

	// Warn about legacy path usage (once per load)
	if warnLegacy {
		fmt.Fprintf(os.Stderr, "Warning: Using legacy config path %s. Consider moving to %s\n", legacyPath, configPath)
	}

	// Apply migrations if needed
	migratedConfig := migrateConfig(config)

	return mergeWithDefaults(migratedConfig), nil
}

func Save(config Config) error {
	configPath := Path()
	if configPath == "" {
		return fmt.Errorf("unable to determine home directory")
	}

	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %v", err)
	}

	file, err := os.Create(configPath)
	if err != nil {
		return fmt.Errorf("failed to create config file: %v", err)
	}
	defer file.Close()

	encoder := toml.NewEncoder(file)
	if err := encoder.Encode(config); err != nil {
		return fmt.Errorf("failed to encode config: %v", err)
	}

	return nil
}

func GetRuntimeConfig() Config {
	config, err := Load()
	if err != nil && err != ErrNotConfigured {
		fmt.Fprintf(os.Stderr, "Warning: %v, using defaults\n", err)
		config = getDefaults()
	}

	// Apply environment variable overlays
	return applyEnvOverlays(config)
}

func mergeWithDefaults(config Config) Config {
	// Always ensure we have the current schema version
	config.SchemaVersion = CurrentSchemaVersion

	// DefaultScope has a sensible generic default
	if config.DefaultScope == "" {
		config.DefaultScope = "assigned_or_reported"
	}

	// EnableWorktrees defaults to true when not explicitly set
	if config.EnableWorktrees == nil {
		t := true
		config.EnableWorktrees = &t
	}

	// EnableClaude defaults to false (nil is equivalent to false)
	if config.EnableClaude == nil {
		f := false
		config.EnableClaude = &f
	}

	// Projects, JiraURL, Boards: left empty if not in config file.
	// The caller must handle empty values (e.g. prompt for gci setup).

	return config
}

// ClaudeEnabled returns whether Claude AI integration is enabled.
func (c Config) ClaudeEnabled() bool {
	return c.EnableClaude != nil && *c.EnableClaude
}

// WorktreesEnabled returns whether git worktrees are enabled for Interactive Mode.
func (c Config) WorktreesEnabled() bool {
	return c.EnableWorktrees == nil || *c.EnableWorktrees
}

// applyEnvOverlays applies environment variable overlays to the config
func applyEnvOverlays(config Config) Config {
	// GCI_PROJECTS: comma-separated project list
	if envProjects := os.Getenv("GCI_PROJECTS"); envProjects != "" {
		projects := strings.Split(envProjects, ",")
		for i := range projects {
			projects[i] = strings.TrimSpace(projects[i])
		}
		// Filter out empty strings
		var validProjects []string
		for _, p := range projects {
			if p != "" {
				validProjects = append(validProjects, p)
			}
		}
		if len(validProjects) > 0 {
			config.Projects = validProjects
		}
	}

	// GCI_DEFAULT_SCOPE: override default scope
	if envScope := os.Getenv("GCI_DEFAULT_SCOPE"); envScope != "" {
		config.DefaultScope = envScope
	}

	// GCI_JIRA_URL: override JIRA URL
	if envJiraURL := os.Getenv("GCI_JIRA_URL"); envJiraURL != "" {
		config.JiraURL = envJiraURL
	}

	// GCI_OP_JIRA_TOKEN_PATH: override 1Password JIRA token path
	if v := os.Getenv("GCI_OP_JIRA_TOKEN_PATH"); v != "" {
		config.OPJiraTokenPath = v
	}

	return config
}

// migrateConfig performs in-memory migration of config from older schema versions
func migrateConfig(config Config) Config {
	originalVersion := config.SchemaVersion

	// Migration from version 0 (no schema_version field) to version 1
	if originalVersion == 0 {
		// Version 0 configs don't have schema_version field
		// Current structure is already compatible, just need to set version
		config.SchemaVersion = 1

		// Log migration if needed (could be made conditional)
		if config.Projects != nil || config.DefaultScope != "" || config.JiraURL != "" || config.Boards != nil {
			fmt.Fprintf(os.Stderr, "Info: Migrated config from schema version 0 to %d\n", config.SchemaVersion)
		}
	}

	// Future migrations would go here:
	// if originalVersion < 2 { ... }

	return config
}

// MigrateAndSave loads the config, applies migrations, and saves it back to disk
// This is used by the `gci config migrate` command
func MigrateAndSave() error {
	// Load the raw config without going through the full Load() process
	configPath := Path()
	legacyPath := LegacyPath()

	if configPath == "" || legacyPath == "" {
		return fmt.Errorf("unable to determine home directory")
	}

	var actualPath string

	// Check XDG-compliant path first
	if _, err := os.Stat(configPath); err == nil {
		actualPath = configPath
	} else if _, err := os.Stat(legacyPath); err == nil {
		actualPath = legacyPath
	} else {
		return fmt.Errorf("no config file found to migrate")
	}

	var rawConfig Config
	if _, err := toml.DecodeFile(actualPath, &rawConfig); err != nil {
		return fmt.Errorf("failed to decode config file: %v", err)
	}

	originalVersion := rawConfig.SchemaVersion
	if originalVersion == CurrentSchemaVersion {
		return fmt.Errorf("config is already at current schema version %d", CurrentSchemaVersion)
	}

	// Now apply the full Load() process which includes migration and merging
	config, err := Load()
	if err != nil {
		return fmt.Errorf("failed to load config for migration: %v", err)
	}

	// Save the migrated config
	err = Save(config)
	if err != nil {
		return fmt.Errorf("failed to save migrated config: %v", err)
	}

	fmt.Printf("Successfully migrated config from schema version %d to %d\n", originalVersion, config.SchemaVersion)
	return nil
}

// SaveUIPrefs saves only the UI preferences to the config file
// This is lightweight and can be called frequently without impacting other config values
func SaveUIPrefs(prefs UIPreferences) error {
	config, err := Load()
	if err != nil {
		// Create a minimal config -- don't seed with company defaults
		config = Config{
			SchemaVersion: CurrentSchemaVersion,
			DefaultScope:  "assigned_or_reported",
		}
	}

	config.UIPrefs = prefs
	return Save(config)
}

// GetUIPrefs returns the current UI preferences from the runtime config
func GetUIPrefs() UIPreferences {
	// Allow ignoring UI prefs via env for troubleshooting
	if os.Getenv("GCI_IGNORE_UI_PREFS") == "1" {
		return UIPreferences{}
	}
	config := GetRuntimeConfig()
	return config.UIPrefs
}
