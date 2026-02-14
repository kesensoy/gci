package errors

import (
	"fmt"
	"strings"
)

// UserError represents an error with user-friendly messaging and remediation hints
type UserError struct {
	Title       string // Brief title of the error
	Message     string // Detailed error message
	Remediation string // What the user can do to fix it
	Cause       error  // Underlying error, if any
}

func (e *UserError) Error() string {
	var parts []string
	
	if e.Title != "" {
		parts = append(parts, e.Title)
	}
	
	if e.Message != "" {
		parts = append(parts, e.Message)
	}
	
	if e.Remediation != "" {
		parts = append(parts, fmt.Sprintf("💡 %s", e.Remediation))
	}
	
	return strings.Join(parts, "\n")
}

func (e *UserError) Unwrap() error {
	return e.Cause
}

// Common error constructors with built-in remediation

func NewGitConfigError(err error) *UserError {
	return &UserError{
		Title:       "❌ Git Configuration Error",
		Message:     "Failed to get git user email configuration.",
		Remediation: "Run: git config --global user.email \"your.email@example.com\"",
		Cause:       err,
	}
}

func NewOnePasswordError() *UserError {
	return &UserError{
		Title:       "Authentication Error",
		Message:     "No JIRA API token found.",
		Remediation: "Set JIRA_API_TOKEN env var, or configure op_jira_token_path in ~/.config/gci/config.toml and run: op signin",
		Cause:       nil,
	}
}

func NewInvalidProjectError(project string, available []string) *UserError {
	return &UserError{
		Title:       "❌ Invalid Project",
		Message:     fmt.Sprintf("Project '%s' is not available.", project),
		Remediation: fmt.Sprintf("Available projects: %s. Use 'gci setup' to configure projects", strings.Join(available, ", ")),
		Cause:       nil,
	}
}

func NewJiraConnectionError(err error) *UserError {
	errStr := err.Error()
	var remediation string
	
	if strings.Contains(errStr, "401") || strings.Contains(errStr, "Unauthorized") {
		remediation = "Check your API token in 1Password. Run: op signin && gci config doctor"
	} else if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "no such host") {
		remediation = "Check your internet connection and JIRA URL. Run: gci config doctor"
	} else if strings.Contains(errStr, "403") || strings.Contains(errStr, "Forbidden") {
		remediation = "Your API token lacks permission for this operation. Contact your JIRA administrator"
	} else {
		remediation = "Run: gci config doctor to diagnose the issue"
	}
	
	return &UserError{
		Title:       "❌ JIRA Connection Error",
		Message:     "Failed to connect to JIRA. " + errStr,
		Remediation: remediation,
		Cause:       err,
	}
}

func NewJQLPresetError(preset string, err error) *UserError {
	return &UserError{
		Title:       "❌ JQL Preset Error",
		Message:     fmt.Sprintf("JQL preset '%s' failed to execute.", preset),
		Remediation: "Check your JQL syntax in the config file. Run: gci config get jql_presets",
		Cause:       err,
	}
}

func NewJQLPresetNotFoundError(preset string) *UserError {
	return &UserError{
		Title:       "❌ JQL Preset Not Found",
		Message:     fmt.Sprintf("JQL preset '%s' is not configured.", preset),
		Remediation: "Run: gci config print to see available presets, or gci setup to configure them",
		Cause:       nil,
	}
}

func NewConfigError(operation string, err error) *UserError {
	var remediation string
	errStr := err.Error()
	
	switch {
	case strings.Contains(errStr, "permission denied"):
		remediation = "Check file permissions. Run: chmod 644 ~/.config/gci/config.toml"
	case strings.Contains(errStr, "no such file"):
		remediation = "Run: gci setup to create a configuration file"
	case strings.Contains(errStr, "decode") || strings.Contains(errStr, "parse"):
		remediation = "Configuration file format is invalid. Run: gci config doctor"
	default:
		remediation = "Run: gci config doctor to diagnose configuration issues"
	}
	
	return &UserError{
		Title:       "❌ Configuration Error",
		Message:     fmt.Sprintf("Failed to %s configuration: %s", operation, errStr),
		Remediation: remediation,
		Cause:       err,
	}
}

func NewBoardDiscoveryError(err error) *UserError {
	return &UserError{
		Title:       "❌ Board Discovery Error",
		Message:     "Failed to discover JIRA boards from your instance.",
		Remediation: "Check your JIRA permissions and API token. Some boards may be restricted",
		Cause:       err,
	}
}

func NewHttpError(statusCode int, body string) *UserError {
	var title, remediation string
	
	switch {
	case statusCode == 401:
		title = "❌ Authentication Failed"
		remediation = "Check your API token. Run: op signin && gci config doctor"
	case statusCode == 403:
		title = "❌ Access Forbidden" 
		remediation = "Your account lacks permission for this operation. Contact your JIRA administrator"
	case statusCode == 404:
		title = "❌ Resource Not Found"
		remediation = "The requested JIRA resource was not found. Check your project configuration"
	case statusCode >= 500:
		title = "❌ Server Error"
		remediation = "JIRA server is experiencing issues. Try again later or contact your administrator"
	default:
		title = "❌ HTTP Error"
		remediation = "An unexpected HTTP error occurred. Run: gci --verbose to see detailed logs"
	}
	
	return &UserError{
		Title:       title,
		Message:     fmt.Sprintf("HTTP %d: %s", statusCode, body),
		Remediation: remediation,
		Cause:       nil,
	}
}

// Helper function to wrap existing errors with better messaging
func WrapWithContext(err error, context string) error {
	if userErr, ok := err.(*UserError); ok {
		// Already a user error, just return it
		return userErr
	}
	
	// Try to create a more specific error based on context and content
	errStr := err.Error()
	
	switch context {
	case "git_config":
		return NewGitConfigError(err)
	case "jira_connection":
		return NewJiraConnectionError(err)
	case "config_load", "config_save":
		return NewConfigError(context, err)
	case "board_discovery":
		return NewBoardDiscoveryError(err)
	default:
		// Generic wrapper that at least adds some structure
		return &UserError{
			Title:       "❌ Error",
			Message:     errStr,
			Remediation: "Run with --verbose flag for more details",
			Cause:       err,
		}
	}
}