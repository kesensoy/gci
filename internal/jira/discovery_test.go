package jira

import (
	"testing"
)

func TestRankBoards(t *testing.T) {
	boards := []Board{
		{ID: 1, Name: "CHANGE Board", Type: "scrum", Location: struct{ ProjectKey string `json:"projectKey"` }{ProjectKey: "CHANGE"}},
		{ID: 2, Name: "INF Board", Type: "kanban", Location: struct{ ProjectKey string `json:"projectKey"` }{ProjectKey: "INF"}},
		{ID: 3, Name: "OTHER Board", Type: "simple", Location: struct{ ProjectKey string `json:"projectKey"` }{ProjectKey: "OTHER"}},
		{ID: 4, Name: "CHANGE Active Board", Type: "scrum", Location: struct{ ProjectKey string `json:"projectKey"` }{ProjectKey: "CHANGE"}},
		{ID: 5, Name: "CHANGE Deprecated Board", Type: "scrum", Location: struct{ ProjectKey string `json:"projectKey"` }{ProjectKey: "CHANGE"}},
	}

	currentProjects := []string{"CHANGE", "INF"}
	ranked := RankBoards(boards, currentProjects)

	if len(ranked) != 5 {
		t.Fatalf("Expected 5 boards, got %d", len(ranked))
	}

	// First should be CHANGE Active Board (project match + active boost)
	if ranked[0].ID != 4 {
		t.Errorf("Expected CHANGE Active board first, got board %d (%s)", ranked[0].ID, ranked[0].Name)
	}

	// INF board should rank higher than CHANGE boards without "active" or with "deprecated"
	// But project match is heavily weighted, so CHANGE boards still come first
	if ranked[1].Location.ProjectKey != "CHANGE" {
		t.Errorf("Expected second board to be from CHANGE project, got %s", ranked[1].Location.ProjectKey)
	}

	// Last should be non-matching project
	if ranked[len(ranked)-1].Location.ProjectKey == "CHANGE" || ranked[len(ranked)-1].Location.ProjectKey == "INF" {
		t.Errorf("Expected last board to be from OTHER project, got %s", ranked[len(ranked)-1].Location.ProjectKey)
	}
}

func TestRankBoardsDeterministic(t *testing.T) {
	// Test that ranking is deterministic by running multiple times
	boards := []Board{
		{ID: 100, Name: "Board A", Type: "scrum", Location: struct{ ProjectKey string `json:"projectKey"` }{ProjectKey: "PROJ"}},
		{ID: 50, Name: "Board B", Type: "scrum", Location: struct{ ProjectKey string `json:"projectKey"` }{ProjectKey: "PROJ"}},
		{ID: 75, Name: "Board C", Type: "scrum", Location: struct{ ProjectKey string `json:"projectKey"` }{ProjectKey: "PROJ"}},
	}

	currentProjects := []string{"PROJ"}
	
	// Run ranking multiple times
	var results [][]Board
	for i := 0; i < 5; i++ {
		ranked := RankBoards(boards, currentProjects)
		results = append(results, ranked)
	}
	
	// All results should be identical (deterministic)
	for i := 1; i < len(results); i++ {
		for j := 0; j < len(results[i]); j++ {
			if results[0][j].ID != results[i][j].ID {
				t.Errorf("Ranking is not deterministic. Run 0 had board %d at position %d, run %d had board %d", 
					results[0][j].ID, j, i, results[i][j].ID)
			}
		}
	}
	
	// For equal scores, lower ID should come first (tiebreaker)
	if results[0][0].ID != 50 { // Lowest ID
		t.Errorf("Expected board with ID 50 first (lowest ID tiebreaker), got %d", results[0][0].ID)
	}
}

func TestBoardWithActivity(t *testing.T) {
	// Test that BoardWithActivity correctly preserves Board data
	board := Board{
		ID:   123,
		Name: "Test Board",
		Type: "scrum",
		Location: struct{ ProjectKey string `json:"projectKey"` }{ProjectKey: "TEST"},
	}
	
	bwa := BoardWithActivity{
		Board:          board,
		RecentActivity: 42,
	}
	
	if bwa.Board.ID != board.ID {
		t.Errorf("BoardWithActivity lost Board.ID: expected %d, got %d", board.ID, bwa.Board.ID)
	}
	
	if bwa.Board.Name != board.Name {
		t.Errorf("BoardWithActivity lost Board.Name: expected %s, got %s", board.Name, bwa.Board.Name)
	}
	
	if bwa.RecentActivity != 42 {
		t.Errorf("BoardWithActivity lost RecentActivity: expected 42, got %d", bwa.RecentActivity)
	}
}

func TestGetCacheFilePath(t *testing.T) {
	path := getCacheFilePath()
	if path == "" {
		t.Skip("No home directory available")
	}
	
	if len(path) < 21 || path[len(path)-21:] != "gci_boards_cache.json" {
		t.Errorf("Cache file path should end with gci_boards_cache.json, got %s", path)
	}
}