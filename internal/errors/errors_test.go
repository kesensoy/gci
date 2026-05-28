package errors

import (
	"fmt"
	"strings"
	"testing"
)

func TestUserError_Error(t *testing.T) {
	tests := []struct {
		name     string
		userErr  *UserError
		expected []string // Substrings that should be present
	}{
		{
			name: "complete error with all fields",
			userErr: &UserError{
				Title:       "❌ Test Error",
				Message:     "Something went wrong",
				Remediation: "Try running the fix",
				Cause:       fmt.Errorf("underlying cause"),
			},
			expected: []string{"❌ Test Error", "Something went wrong", "💡 Try running the fix"},
		},
		{
			name: "error without title",
			userErr: &UserError{
				Message:     "Just a message",
				Remediation: "Just a fix",
			},
			expected: []string{"Just a message", "💡 Just a fix"},
		},
		{
			name: "error without remediation",
			userErr: &UserError{
				Title:   "❌ Simple Error",
				Message: "Something failed",
			},
			expected: []string{"❌ Simple Error", "Something failed"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.userErr.Error()
			for _, expected := range tt.expected {
				if !strings.Contains(result, expected) {
					t.Errorf("Expected error message to contain %q, but got: %s", expected, result)
				}
			}
		})
	}
}

func TestNewGitConfigError(t *testing.T) {
	cause := fmt.Errorf("exit status 1")
	err := NewGitConfigError(cause)
	
	result := err.Error()
	
	// Check for expected components
	expectedParts := []string{
		"❌ Git Configuration Error",
		"Failed to get git user email configuration",
		"💡 Run: git config --global user.email",
	}
	
	for _, part := range expectedParts {
		if !strings.Contains(result, part) {
			t.Errorf("Expected error message to contain %q, but got: %s", part, result)
		}
	}
	
	// Check that it unwraps correctly
	if err.Unwrap() != cause {
		t.Errorf("Expected Unwrap() to return %v, got %v", cause, err.Unwrap())
	}
}

func TestNewOnePasswordError(t *testing.T) {
	err := NewOnePasswordError()

	result := err.Error()

	expectedParts := []string{
		"Authentication Error",
		"No JIRA API token found",
		"💡 Set JIRA_API_TOKEN env var",
		"op_jira_token_path",
	}

	for _, part := range expectedParts {
		if !strings.Contains(result, part) {
			t.Errorf("Expected error message to contain %q, but got: %s", part, result)
		}
	}
}

func TestNewInvalidProjectError(t *testing.T) {
	err := NewInvalidProjectError("BADPROJ", []string{"GOOD1", "GOOD2"})
	
	result := err.Error()
	
	expectedParts := []string{
		"❌ Invalid Project",
		"Project 'BADPROJ' is not available",
		"💡 Available projects: GOOD1, GOOD2",
		"gci setup",
	}
	
	for _, part := range expectedParts {
		if !strings.Contains(result, part) {
			t.Errorf("Expected error message to contain %q, but got: %s", part, result)
		}
	}
}

func TestNewJiraConnectionError(t *testing.T) {
	tests := []struct {
		name           string
		cause          error
		expectedRemediation string
	}{
		{
			name:           "401 unauthorized",
			cause:          fmt.Errorf("HTTP 401: Unauthorized"),
			expectedRemediation: "rejected your API token",
		},
		{
			name:           "timeout error",
			cause:          fmt.Errorf("timeout occurred"),
			expectedRemediation: "Check your internet connection",
		},
		{
			name:           "403 forbidden",
			cause:          fmt.Errorf("HTTP 403: Forbidden"),
			expectedRemediation: "Your API token lacks permission",
		},
		{
			name:           "generic error",
			cause:          fmt.Errorf("some other error"),
			expectedRemediation: "gci config doctor",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewJiraConnectionError(tt.cause)
			result := err.Error()
			
			if !strings.Contains(result, "❌ JIRA Connection Error") {
				t.Errorf("Expected error to contain JIRA Connection Error, got: %s", result)
			}
			
			if !strings.Contains(result, tt.expectedRemediation) {
				t.Errorf("Expected error to contain %q, got: %s", tt.expectedRemediation, result)
			}
		})
	}
}

func TestNewHttpError(t *testing.T) {
	tests := []struct {
		statusCode       int
		expectedTitle    string
		expectedRemediation string
	}{
		{401, "❌ Authentication Failed", "rejected your API token"},
		{403, "❌ Access Forbidden", "Your account lacks permission"},
		{404, "❌ Resource Not Found", "The requested JIRA resource was not found"},
		{500, "❌ Server Error", "JIRA server is experiencing issues"},
		{418, "❌ HTTP Error", "An unexpected HTTP error occurred"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("status_%d", tt.statusCode), func(t *testing.T) {
			err := NewHttpError(tt.statusCode, "test body")
			result := err.Error()
			
			if !strings.Contains(result, tt.expectedTitle) {
				t.Errorf("Expected error to contain %q, got: %s", tt.expectedTitle, result)
			}
			
			if !strings.Contains(result, tt.expectedRemediation) {
				t.Errorf("Expected error to contain %q, got: %s", tt.expectedRemediation, result)
			}
		})
	}
}

func TestWrapWithContext(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		context  string
		expected string
	}{
		{
			name:     "git_config context",
			err:      fmt.Errorf("command failed"),
			context:  "git_config",
			expected: "❌ Git Configuration Error",
		},
		{
			name:     "jira_connection context",
			err:      fmt.Errorf("connection failed"),
			context:  "jira_connection",
			expected: "❌ JIRA Connection Error",
		},
		{
			name:     "config_load context",
			err:      fmt.Errorf("file not found"),
			context:  "config_load",
			expected: "❌ Configuration Error",
		},
		{
			name:     "generic context",
			err:      fmt.Errorf("unknown error"),
			context:  "unknown",
			expected: "❌ Error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wrapped := WrapWithContext(tt.err, tt.context)
			result := wrapped.Error()
			
			if !strings.Contains(result, tt.expected) {
				t.Errorf("Expected wrapped error to contain %q, got: %s", tt.expected, result)
			}
		})
	}
}

func TestWrapWithContext_AlreadyUserError(t *testing.T) {
	// Test that wrapping a UserError returns it unchanged
	original := NewOnePasswordError()
	wrapped := WrapWithContext(original, "some_context")
	
	if wrapped != original {
		t.Error("Expected WrapWithContext to return the same UserError unchanged")
	}
}