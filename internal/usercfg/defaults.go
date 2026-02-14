package usercfg

func getDefaults() Config {
	t := true
	f := false
	return Config{
		SchemaVersion:   CurrentSchemaVersion,
		Projects:        nil,
		DefaultScope:    "assigned_or_reported",
		JiraURL:         "",
		Boards:          nil,
		EnableClaude:    &f,
		EnableWorktrees: &t,
	}
}

func GetAvailableProjects() []string {
	return GetAvailableProjectsFromRuntime()
}

func GetAvailableProjectsFromRuntime() []string {
	// Get the runtime config to include any env var overrides
	config := GetRuntimeConfig()
	projects := make([]string, len(config.Projects))
	copy(projects, config.Projects)
	projects = append(projects, "both")
	return projects
}
