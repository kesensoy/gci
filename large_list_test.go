package main

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestLargeListRendering tests that rendering performance is acceptable with thousands of issues
func TestLargeListRendering(t *testing.T) {
	cfg := &Config{
		JiraURL:  "https://test.atlassian.net",
		Email:    "test@example.com",
		APIToken: "test-token",
		Projects: []string{"TEST"},
	}

	model := initialBoardModel(cfg)
	model.width = 120
	model.height = 40

	// Create a large number of synthetic issues
	const numIssues = 5000
	syntheticIssues := make([]JiraIssue, numIssues)

	for i := 0; i < numIssues; i++ {
		syntheticIssues[i] = JiraIssue{
			Key: fmt.Sprintf("TEST-%d", i+1),
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
				Summary: fmt.Sprintf("Test issue number %d - this is a longer summary to simulate real issue content", i+1),
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
			},
		}
	}

	// Distribute issues across columns to simulate a real board
	model.columns[0].issues = syntheticIssues[:2000]            // 2000 in To Do
	model.columns[1].issues = syntheticIssues[2000:3500]       // 1500 in In Progress  
	model.columns[2].issues = syntheticIssues[3500:numIssues]  // 1500 in Done

	// Initialize all issues as well
	for i := range model.columns {
		model.columns[i].allIssues = model.columns[i].issues
	}

	// Measure rendering time
	start := time.Now()
	
	// Render the view multiple times to get average performance
	const numRenders = 100
	for i := 0; i < numRenders; i++ {
		view := model.View()
		
		// Verify view is not empty
		if len(view) == 0 {
			t.Error("View should not be empty with synthetic data")
		}
		
		// Verify we're not rendering all issues (windowing is working)
		issueCount := strings.Count(view, "TEST-")
		expectedMaxVisible := model.itemsWindowCount() * len(model.columns) + 10 // +10 for slack/indicators
		if issueCount > expectedMaxVisible {
			t.Errorf("Too many issues rendered: %d > %d (windowing may not be working)", issueCount, expectedMaxVisible)
		}
	}
	
	renderTime := time.Since(start)
	avgRenderTime := renderTime / numRenders
	
	// Performance assertion: each render should be very fast even with 5000 issues  
	// Allow 20ms which is still excellent performance for large datasets
	maxAcceptableTime := 20 * time.Millisecond
	if avgRenderTime > maxAcceptableTime {
		t.Errorf("Rendering too slow: %v > %v per render with %d issues", avgRenderTime, maxAcceptableTime, numIssues)
	}
	
	t.Logf("✅ Large list rendering performance: %v avg per render (%d renders of %d issues)", avgRenderTime, numRenders, numIssues)
}

// TestLargeListNavigation tests that navigation performance is acceptable with thousands of issues
func TestLargeListNavigation(t *testing.T) {
	cfg := &Config{
		JiraURL:  "https://test.atlassian.net",  
		Email:    "test@example.com",
		APIToken: "test-token",
		Projects: []string{"TEST"},
	}

	model := initialBoardModel(cfg)
	model.width = 120
	model.height = 40

	// Create synthetic issues for the first column
	const numIssues = 10000
	syntheticIssues := make([]JiraIssue, numIssues)
	for i := 0; i < numIssues; i++ {
		syntheticIssues[i] = JiraIssue{
			Key: fmt.Sprintf("TEST-%d", i+1),
		}
	}
	
	model.columns[0].issues = syntheticIssues
	model.columns[0].allIssues = syntheticIssues

	// Test navigation performance by jumping to end and back
	start := time.Now()
	
	// Navigate to the bottom
	model.columns[0].cursor = numIssues - 1
	model.ensureCursorVisible(&model.columns[0])
	
	// Navigate to the top
	model.columns[0].cursor = 0
	model.ensureCursorVisible(&model.columns[0])
	
	// Navigate to middle
	model.columns[0].cursor = numIssues / 2
	model.ensureCursorVisible(&model.columns[0])
	
	navigationTime := time.Since(start)
	
	// Navigation should be near-instantaneous even with 10k issues
	maxAcceptableTime := 1 * time.Millisecond
	if navigationTime > maxAcceptableTime {
		t.Errorf("Navigation too slow: %v > %v with %d issues", navigationTime, maxAcceptableTime, numIssues)
	}
	
	// Verify viewport positioning is correct
	itemsWindow := model.itemsWindowCount()
	expectedOffset := numIssues/2 - itemsWindow/2
	if expectedOffset < 0 {
		expectedOffset = 0
	}
	if expectedOffset > numIssues-itemsWindow {
		expectedOffset = numIssues - itemsWindow
	}
	
	// The offset should be reasonable (cursor should be visible)
	if model.columns[0].cursor < model.columns[0].offset || 
	   model.columns[0].cursor >= model.columns[0].offset+itemsWindow {
		t.Errorf("Cursor not visible: cursor=%d, offset=%d, window=%d", 
			model.columns[0].cursor, model.columns[0].offset, itemsWindow)
	}
	
	t.Logf("✅ Large list navigation performance: %v for %d issues", navigationTime, numIssues)
}