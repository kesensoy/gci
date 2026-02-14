package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"gci/internal/jira"
)

// Mock JIRA response structures for testing
type mockJiraResponse struct {
	Issues     []JiraIssue `json:"issues"`
	StartAt    int         `json:"startAt"`
	MaxResults int         `json:"maxResults"`
	Total      int         `json:"total"`
}

// TestFetchColumnIssues_IntegrationWithMockServer tests fetchColumnIssues with a test server
func TestFetchColumnIssues_IntegrationWithMockServer(t *testing.T) {
	// Create mock JIRA issues
	mockIssues := []JiraIssue{
		{
			Key: "TEST-123",
			Fields: struct {
				Summary     string `json:"summary"`
				Description *struct {
					Content []struct {
						Type    string `json:"type"`
						Content []struct {
							Type string `json:"type"`
							Text string `json:"text,omitempty"`
						} `json:"content,omitempty"`
					} `json:"content,omitempty"`
				} `json:"description"`
				Project struct {
					Key string `json:"key"`
				} `json:"project"`
				IssueType struct {
					Name    string `json:"name"`
					Subtask bool   `json:"subtask"`
				} `json:"issuetype"`
				Parent struct {
					Key string `json:"key"`
				} `json:"parent"`
				Status struct {
					Name           string `json:"name"`
					StatusCategory struct {
						Name string `json:"name"`
					} `json:"statusCategory"`
				} `json:"status"`
				Assignee struct {
					DisplayName string `json:"displayName"`
					Name        string `json:"name"`
				} `json:"assignee"`
				Priority struct {
					Name string `json:"name"`
				} `json:"priority"`
			}{
				Summary: "Test issue for integration test",
				Project: struct {
					Key string `json:"key"`
				}{Key: "TEST"},
				Status: struct {
					Name           string `json:"name"`
					StatusCategory struct {
						Name string `json:"name"`
					} `json:"statusCategory"`
				}{
					Name: "To Do",
					StatusCategory: struct {
						Name string `json:"name"`
					}{Name: "To Do"},
				},
				Assignee: struct {
					DisplayName string `json:"displayName"`
					Name        string `json:"name"`
				}{
					DisplayName: "Test User",
					Name:        "testuser",
				},
				Priority: struct {
					Name string `json:"name"`
				}{Name: "Medium"},
			},
		},
	}

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the request is properly formed
		if r.Header.Get("Authorization") == "" {
			t.Error("Authorization header missing")
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Error("Accept header incorrect")
		}

		// Return mock response
		response := mockJiraResponse{
			Issues:     mockIssues,
			StartAt:    0,
			MaxResults: 50,
			Total:      1,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Create test config
	config := &Config{
		JiraURL:  server.URL,
		Email:    "test@example.com",
		APIToken: "test-token",
	}

	// Test fetchColumnIssues
	issues, err := fetchColumnIssues(config, "To Do", scopeMine, 50)
	if err != nil {
		t.Fatalf("fetchColumnIssues failed: %v", err)
	}

	if len(issues) != 1 {
		t.Fatalf("Expected 1 issue, got %d", len(issues))
	}

	if issues[0].Key != "TEST-123" {
		t.Errorf("Expected issue key 'TEST-123', got '%s'", issues[0].Key)
	}

	if issues[0].Fields.Summary != "Test issue for integration test" {
		t.Errorf("Expected summary 'Test issue for integration test', got '%s'", issues[0].Fields.Summary)
	}
}

// TestFetchIssuesWithJQL_IntegrationWithMockServer tests fetchIssuesWithJQL with a test server
func TestFetchIssuesWithJQL_IntegrationWithMockServer(t *testing.T) {
	mockIssues := []JiraIssue{
		{
			Key: "PROJ-456",
			Fields: struct {
				Summary     string `json:"summary"`
				Description *struct {
					Content []struct {
						Type    string `json:"type"`
						Content []struct {
							Type string `json:"type"`
							Text string `json:"text,omitempty"`
						} `json:"content,omitempty"`
					} `json:"content,omitempty"`
				} `json:"description"`
				Project struct {
					Key string `json:"key"`
				} `json:"project"`
				IssueType struct {
					Name    string `json:"name"`
					Subtask bool   `json:"subtask"`
				} `json:"issuetype"`
				Parent struct {
					Key string `json:"key"`
				} `json:"parent"`
				Status struct {
					Name           string `json:"name"`
					StatusCategory struct {
						Name string `json:"name"`
					} `json:"statusCategory"`
				} `json:"status"`
				Assignee struct {
					DisplayName string `json:"displayName"`
					Name        string `json:"name"`
				} `json:"assignee"`
				Priority struct {
					Name string `json:"name"`
				} `json:"priority"`
			}{
				Summary: "JQL test issue",
				Project: struct {
					Key string `json:"key"`
				}{Key: "PROJ"},
				Status: struct {
					Name           string `json:"name"`
					StatusCategory struct {
						Name string `json:"name"`
					} `json:"statusCategory"`
				}{
					Name: "In Progress",
					StatusCategory: struct {
						Name string `json:"name"`
					}{Name: "In Progress"},
				},
			},
		},
	}

	// Track received JQL query
	var receivedJQL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract JQL from query parameters
		receivedJQL = r.URL.Query().Get("jql")

		response := mockJiraResponse{
			Issues:     mockIssues,
			StartAt:    0,
			MaxResults: 25,
			Total:      1,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	config := &Config{
		JiraURL:  server.URL,
		Email:    "test@example.com",
		APIToken: "test-token",
	}

	testJQL := "project = PROJ AND status = 'In Progress'"
	issues, err := fetchIssuesWithJQL(config, testJQL, 25)
	if err != nil {
		t.Fatalf("fetchIssuesWithJQL failed: %v", err)
	}

	if len(issues) != 1 {
		t.Fatalf("Expected 1 issue, got %d", len(issues))
	}

	if issues[0].Key != "PROJ-456" {
		t.Errorf("Expected issue key 'PROJ-456', got '%s'", issues[0].Key)
	}

	if receivedJQL != testJQL {
		t.Errorf("Expected JQL '%s', got '%s'", testJQL, receivedJQL)
	}
}

// TestJiraDiscovery_IntegrationWithMockServer tests JIRA board discovery functions
func TestJiraDiscovery_IntegrationWithMockServer(t *testing.T) {
	// Mock boards response
	mockBoards := []jira.Board{
		{
			ID:   123,
			Name: "Test Board 1",
			Type: "scrum",
		},
		{
			ID:   456,
			Name: "Test Board 2",
			Type: "kanban",
		},
	}

	type mockBoardsResponse struct {
		Values []jira.Board `json:"values"`
	}

	boardsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify this is a boards request
		if r.URL.Path != "/rest/agile/1.0/board" {
			t.Errorf("Expected boards endpoint, got %s", r.URL.Path)
		}

		response := mockBoardsResponse{
			Values: mockBoards,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
	defer boardsServer.Close()

	// Test fetchBoardsFromAPI from internal/jira package
	boards, err := jira.FetchBoardsFromAPI(boardsServer.URL, "test@example.com", "test-token")
	if err != nil {
		t.Fatalf("FetchBoardsFromAPI failed: %v", err)
	}

	if len(boards) != 2 {
		t.Fatalf("Expected 2 boards, got %d", len(boards))
	}

	if boards[0].ID != 123 {
		t.Errorf("Expected first board ID 123, got %d", boards[0].ID)
	}

	if boards[0].Name != "Test Board 1" {
		t.Errorf("Expected first board name 'Test Board 1', got '%s'", boards[0].Name)
	}

	if boards[1].Type != "kanban" {
		t.Errorf("Expected second board type 'kanban', got '%s'", boards[1].Type)
	}
}

// TestHTTPErrorHandling_IntegrationWithMockServer tests error handling with various HTTP error codes
func TestHTTPErrorHandling_IntegrationWithMockServer(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		expectError    bool
		expectRetry    bool
		responseBody   string
	}{
		{
			name:         "401 Unauthorized",
			statusCode:   http.StatusUnauthorized,
			expectError:  true,
			expectRetry:  false,
			responseBody: `{"error": "Invalid credentials"}`,
		},
		{
			name:         "404 Not Found",
			statusCode:   http.StatusNotFound,
			expectError:  true,
			expectRetry:  false,
			responseBody: `{"error": "Resource not found"}`,
		},
		{
			name:         "500 Internal Server Error",
			statusCode:   http.StatusInternalServerError,
			expectError:  true,
			expectRetry:  true,
			responseBody: `{"error": "Server error"}`,
		},
		{
			name:         "503 Service Unavailable",
			statusCode:   http.StatusServiceUnavailable,
			expectError:  true,
			expectRetry:  true,
			responseBody: `{"error": "Service temporarily unavailable"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attempts := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempts++
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			config := &Config{
				JiraURL:  server.URL,
				Email:    "test@example.com",
				APIToken: "test-token",
			}

			_, err := fetchColumnIssues(config, "To Do", scopeMine, 50)

			if tt.expectError && err == nil {
				t.Errorf("Expected error for status %d, but got none", tt.statusCode)
			}

			if !tt.expectError && err != nil {
				t.Errorf("Expected no error for status %d, but got: %v", tt.statusCode, err)
			}

			// For retryable errors, we should see multiple attempts
			if tt.expectRetry && attempts < 2 {
				t.Errorf("Expected retries for status %d, but only saw %d attempts", tt.statusCode, attempts)
			}

			// For non-retryable errors, we should see only one attempt
			if !tt.expectRetry && attempts > 1 {
				t.Errorf("Expected no retries for status %d, but saw %d attempts", tt.statusCode, attempts)
			}
		})
	}
}