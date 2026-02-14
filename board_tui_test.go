package main

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestBoardModel_Init_SmokeTest ensures the Init function doesn't panic
func TestBoardModel_Init_SmokeTest(t *testing.T) {
	cfg := &Config{
		JiraURL:  "https://test.atlassian.net",
		Email:    "test@example.com",
		APIToken: "test-token",
		Projects: []string{"TEST"},
	}

	model := initialBoardModel(cfg)

	// Test that Init() doesn't panic
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Init() panicked: %v", r)
		}
	}()

	cmd := model.Init()

	// Verify that Init returns a command
	if cmd == nil {
		t.Error("Init() should return a command")
	}

	// Verify model has expected initial state
	if model.cfg != cfg {
		t.Error("Model should store the provided config")
	}

	if len(model.columns) != 3 {
		t.Errorf("Expected 3 columns, got %d", len(model.columns))
	}

	expectedColumns := []string{"To Do", "In Progress", "Done"}
	for i, expected := range expectedColumns {
		if model.columns[i].title != expected {
			t.Errorf("Column %d: expected title '%s', got '%s'", i, expected, model.columns[i].title)
		}
	}

	if !model.loading {
		t.Error("Model should be in loading state initially")
	}
}

// TestBoardModel_Update_SmokeTest ensures the Update function handles basic messages without panicking
func TestBoardModel_Update_SmokeTest(t *testing.T) {
	cfg := &Config{
		JiraURL:  "https://test.atlassian.net",
		Email:    "test@example.com",
		APIToken: "test-token",
		Projects: []string{"TEST"},
	}

	model := initialBoardModel(cfg)

	testCases := []struct {
		name string
		msg  tea.Msg
	}{
		{
			name: "Key message - quit",
			msg:  tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}},
		},
		{
			name: "Key message - refresh",
			msg:  tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}},
		},
		{
			name: "Key message - left arrow",
			msg:  tea.KeyMsg{Type: tea.KeyLeft},
		},
		{
			name: "Key message - right arrow",
			msg:  tea.KeyMsg{Type: tea.KeyRight},
		},
		{
			name: "Key message - up arrow",
			msg:  tea.KeyMsg{Type: tea.KeyUp},
		},
		{
			name: "Key message - down arrow",
			msg:  tea.KeyMsg{Type: tea.KeyDown},
		},
		{
			name: "Key message - tab",
			msg:  tea.KeyMsg{Type: tea.KeyTab},
		},
		{
			name: "Window size message",
			msg:  tea.WindowSizeMsg{Width: 80, Height: 24},
		},
		{
			name: "Invalid key message",
			msg:  tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'@'}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test that Update() doesn't panic with various messages
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("Update() panicked with message %v: %v", tc.msg, r)
				}
			}()

			// Call Update
			updatedModel, cmd := model.Update(tc.msg)

			// Verify we got a model back (even if it's the same one)
			if updatedModel == nil {
				t.Error("Update() should return a model")
			}

			// cmd can be nil, that's fine for some messages
			_ = cmd
		})
	}
}

// TestBoardModel_Update_LoadingMessages tests handling of loading-related messages
func TestBoardModel_Update_LoadingMessages(t *testing.T) {
	cfg := &Config{
		JiraURL:  "https://test.atlassian.net",
		Email:    "test@example.com",
		APIToken: "test-token",
		Projects: []string{"TEST"},
	}

	model := initialBoardModel(cfg)

	// Test data loaded message
	loadingMsg := dataLoadedMsg{
		columns: []kanbanColumnView{
			{
				title:          "To Do",
				statusCategory: "To Do",
				issues: []JiraIssue{
					{Key: "TEST-123"},
				},
			},
		},
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Update() panicked with dataLoadedMsg: %v", r)
		}
	}()

	updatedModel, cmd := model.Update(loadingMsg)
	if updatedModel == nil {
		t.Error("Update() should return a model")
	}

	// cmd can be nil or not, both are valid
	_ = cmd
}

// TestBoardModel_Update_ErrorMessages tests handling of error messages
func TestBoardModel_Update_ErrorMessages(t *testing.T) {
	cfg := &Config{
		JiraURL:  "https://test.atlassian.net",
		Email:    "test@example.com",
		APIToken: "test-token",
		Projects: []string{"TEST"},
	}

	model := initialBoardModel(cfg)

	// Test error message
	testErr := errors.New("Test error message")
	errorMsg := errMsg{
		err: testErr,
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Update() panicked with errMsg: %v", r)
		}
	}()

	updatedModel, cmd := model.Update(errorMsg)
	if updatedModel == nil {
		t.Error("Update() should return a model")
	}

	// Verify error was stored
	boardModel, ok := updatedModel.(boardModel)
	if !ok {
		t.Error("Expected boardModel type")
	}

	if boardModel.err.Error() != testErr.Error() {
		t.Errorf("Expected error 'Test error message', got '%s'", boardModel.err.Error())
	}

	// cmd can be nil or not
	_ = cmd
}

// TestBoardModel_Navigation_SmokeTest tests basic navigation doesn't panic
func TestBoardModel_Navigation_SmokeTest(t *testing.T) {
	cfg := &Config{
		JiraURL:  "https://test.atlassian.net",
		Email:    "test@example.com",
		APIToken: "test-token",
		Projects: []string{"TEST"},
	}

	model := initialBoardModel(cfg)
	
	// Add some mock issues to the model for navigation testing
	model.columns[0].allByScope = make(map[scopeFilter][]JiraIssue)
	model.columns[0].allByScope[scopeMine] = []JiraIssue{
		{Key: "TEST-1"},
		{Key: "TEST-2"},
	}
	model.columns[0].issues = model.columns[0].allByScope[scopeMine]

	navigationKeys := []tea.KeyMsg{
		{Type: tea.KeyUp},
		{Type: tea.KeyDown},
		{Type: tea.KeyLeft},
		{Type: tea.KeyRight},
		{Type: tea.KeyTab},
		{Type: tea.KeyShiftTab},
	}

	for _, key := range navigationKeys {
		defer func(k tea.KeyMsg) {
			if r := recover(); r != nil {
				t.Fatalf("Navigation panicked with key %v: %v", k, r)
			}
		}(key)

		updatedModel, _ := model.Update(key)
		model = updatedModel.(boardModel)
	}
}

// TestBoardModel_View_SmokeTest ensures the View function doesn't panic
func TestBoardModel_View_SmokeTest(t *testing.T) {
	cfg := &Config{
		JiraURL:  "https://test.atlassian.net",
		Email:    "test@example.com",
		APIToken: "test-token",
		Projects: []string{"TEST"},
	}

	model := initialBoardModel(cfg)
	model.width = 80
	model.height = 24

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("View() panicked: %v", r)
		}
	}()

	view := model.View()

	// Verify we get some output
	if len(view) == 0 {
		t.Error("View() should return non-empty string")
	}

	// Test various states
	model.loading = false
	view = model.View()
	if len(view) == 0 {
		t.Error("View() should return non-empty string when not loading")
	}

	model.err = errors.New("Test error")
	view = model.View()
	if len(view) == 0 {
		t.Error("View() should return non-empty string when showing error")
	}
}