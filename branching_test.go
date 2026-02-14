package main

import (
	"testing"
)

// TestCreateBranchName verifies the hardcoded kebab-case branch naming
func TestCreateBranchName(t *testing.T) {
	tests := []struct {
		name        string
		key         string
		summary     string
		expected    string
		description string
	}{
		{
			name:        "basic issue",
			key:         "PROJ-123",
			summary:     "Fix login bug",
			expected:    "PROJ-123_fix-login-bug",
			description: "Basic kebab-case conversion",
		},
		{
			name:        "special characters",
			key:         "CHANGE-456",
			summary:     "Add user's profile @settings",
			expected:    "CHANGE-456_add-user-s-profile-settings",
			description: "Special characters replaced with hyphens",
		},
		{
			name:        "multiple spaces",
			key:         "INF-789",
			summary:     "Update    documentation    for   API",
			expected:    "INF-789_update-documentation-for-api",
			description: "Multiple spaces collapsed to single hyphen",
		},
		{
			name:        "mixed case",
			key:         "TASK-101",
			summary:     "Fix SSO Integration Bug",
			expected:    "TASK-101_fix-sso-integration-bug",
			description: "Mixed case converted to lowercase",
		},
		{
			name:        "unicode characters",
			key:         "PROJ-202",
			summary:     "Add café menu feature",
			expected:    "PROJ-202_add-caf-menu-feature",
			description: "Unicode characters handled",
		},
		{
			name:        "long summary",
			key:         "EPIC-303",
			summary:     "This is a very long summary that exceeds the fifty character limit and should be truncated appropriately",
			expected:    "EPIC-303_this-is-a-very-long-summary-that-exceeds-the-fifty",
			description: "Long summaries truncated to 50 chars",
		},
		{
			name:        "empty summary",
			key:         "TASK-404",
			summary:     "",
			expected:    "TASK-404_",
			description: "Empty summary handled",
		},
		{
			name:        "only special chars",
			key:         "BUG-505",
			summary:     "@#$%^&*()",
			expected:    "BUG-505_",
			description: "Only special characters removed",
		},
		{
			name:        "numbers in summary",
			key:         "FEAT-606",
			summary:     "Add iOS 14 support",
			expected:    "FEAT-606_add-ios-14-support",
			description: "Numbers preserved in summary",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issue := JiraIssue{
				Key: tt.key,
			}
			issue.Fields.Summary = tt.summary

			result := createBranchName(issue)

			if result != tt.expected {
				t.Errorf("createBranchName() = %v, want %v\nDescription: %s",
					result, tt.expected, tt.description)
			}
		})
	}
}

// TestCreateBranchName_Truncation specifically tests the truncation logic
func TestCreateBranchName_Truncation(t *testing.T) {
	tests := []struct {
		name        string
		key         string
		summary     string
		maxLength   int // expected summary portion length (not including KEY_)
		description string
	}{
		{
			name:        "exactly 50 chars",
			key:         "TEST-1",
			summary:     "abcdefghij klmnopqrst uvwxyz abcdefghij klmnopqrst",
			maxLength:   50,
			description: "Summary exactly 50 chars after conversion",
		},
		{
			name:        "over 50 chars",
			key:         "TEST-2",
			summary:     "This is a very long summary that will definitely exceed fifty characters",
			maxLength:   50,
			description: "Summary truncated to 50 chars",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issue := JiraIssue{
				Key: tt.key,
			}
			issue.Fields.Summary = tt.summary

			result := createBranchName(issue)

			// Extract summary part (after KEY_)
			summaryPart := result[len(tt.key)+1:]

			if len(summaryPart) > tt.maxLength {
				t.Errorf("Summary portion too long: got %d chars, want max %d\nResult: %s\nDescription: %s",
					len(summaryPart), tt.maxLength, result, tt.description)
			}
		})
	}
}
