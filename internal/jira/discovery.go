package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gci/internal/httputil"
)

type Board struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Location struct {
		ProjectKey string `json:"projectKey"`
	} `json:"location"`
}

type BoardWithActivity struct {
	Board
	RecentActivity int `json:"recent_activity,omitempty"` // Number of recent issues
}

type BoardsResponse struct {
	Values []Board `json:"values"`
	Total  int     `json:"total"`
}

type DiscoveryCache struct {
	Boards    []BoardWithActivity `json:"boards"`
	Timestamp time.Time           `json:"timestamp"`
}

func DiscoverBoards(jiraURL, email, apiToken string) ([]Board, error) {
	cacheFile := getCacheFilePath()
	
	if cached, ok := loadFromCache(cacheFile); ok {
		// Convert BoardWithActivity back to Board
		result := make([]Board, len(cached))
		for i, bwa := range cached {
			result[i] = bwa.Board
		}
		return result, nil
	}

	boards, err := fetchBoardsFromAPI(jiraURL, email, apiToken)
	if err != nil {
		return nil, err
	}
	
	// Enhance boards with activity data
	boardsWithActivity := enhanceBoardsWithActivity(boards, jiraURL, email, apiToken)
	
	saveToCache(cacheFile, boardsWithActivity)
	
	// Convert back to Board slice for return
	result := make([]Board, len(boardsWithActivity))
	for i, bwa := range boardsWithActivity {
		result[i] = bwa.Board
	}
	return result, nil
}

func getCacheFilePath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(homeDir, ".config", "gci_boards_cache.json")
}

func loadFromCache(cacheFile string) ([]BoardWithActivity, bool) {
	if cacheFile == "" {
		return nil, false
	}

	data, err := os.ReadFile(cacheFile)
	if err != nil {
		return nil, false
	}

	var cache DiscoveryCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, false
	}

	if time.Since(cache.Timestamp) > 24*time.Hour {
		return nil, false
	}

	return cache.Boards, true
}

func saveToCache(cacheFile string, boards []BoardWithActivity) {
	if cacheFile == "" {
		return
	}

	cache := DiscoveryCache{
		Boards:    boards,
		Timestamp: time.Now(),
	}

	data, err := json.Marshal(cache)
	if err != nil {
		return
	}

	os.MkdirAll(filepath.Dir(cacheFile), 0755)
	os.WriteFile(cacheFile, data, 0644)
}

// FetchBoardsFromAPI is an exported wrapper for testing
func FetchBoardsFromAPI(jiraURL, email, apiToken string) ([]Board, error) {
	return fetchBoardsFromAPI(jiraURL, email, apiToken)
}

func fetchBoardsFromAPI(jiraURL, email, apiToken string) ([]Board, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	client := httputil.NewRetryableClient(10*time.Second, 2)

	url := fmt.Sprintf("%s/rest/agile/1.0/board?maxResults=50", jiraURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	req.SetBasicAuth(email, apiToken)
	req.Header.Set("Accept", "application/json")

	var boardsResp BoardsResponse
	if err := client.DoJSONRequest(ctx, req, &boardsResp); err != nil {
		return nil, fmt.Errorf("failed to fetch boards: %w", err)
	}

	return boardsResp.Values, nil
}

// enhanceBoardsWithActivity adds recent activity data to boards
// This operation is designed to complete within a few seconds total
func enhanceBoardsWithActivity(boards []Board, jiraURL, email, apiToken string) []BoardWithActivity {
	enhanced := make([]BoardWithActivity, len(boards))
	
	// Use a channel to limit concurrent requests to avoid overwhelming JIRA
	concurrency := 3
	semaphore := make(chan struct{}, concurrency)
	results := make(chan struct {
		index int
		activity int
	}, len(boards))
	
	// Start activity fetching for each board
	for i, board := range boards {
		go func(idx int, b Board) {
			semaphore <- struct{}{} // Acquire semaphore
			defer func() { <-semaphore }() // Release semaphore
			
			activity := fetchBoardActivity(b.ID, jiraURL, email, apiToken)
			results <- struct {
				index int
				activity int
			}{idx, activity}
		}(i, board)
	}
	
	// Initialize enhanced boards
	for i, board := range boards {
		enhanced[i] = BoardWithActivity{
			Board: board,
			RecentActivity: 0, // Default to 0 if activity fetch fails
		}
	}
	
	// Wait for all activity fetches to complete (with timeout)
	timeout := time.After(8 * time.Second) // Leave 2s buffer for other operations
	collected := 0
	
	for collected < len(boards) {
		select {
		case result := <-results:
			enhanced[result.index].RecentActivity = result.activity
			collected++
		case <-timeout:
			// Timeout reached, use what we have
			goto done
		}
	}
	
done:
	return enhanced
}

// fetchBoardActivity gets the count of recent issues for a board
// Returns 0 if unable to fetch (graceful degradation)
func fetchBoardActivity(boardID int, jiraURL, email, apiToken string) int {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	
	client := httputil.NewRetryableClient(2*time.Second, 1) // Quick timeout, minimal retries
	
	// Query for issues updated in the last 30 days
	jql := "updated >= -30d ORDER BY updated DESC"
	url := fmt.Sprintf("%s/rest/agile/1.0/board/%d/issue?jql=%s&maxResults=50", 
		jiraURL, boardID, jql)
	
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0
	}
	
	req.SetBasicAuth(email, apiToken)
	req.Header.Set("Accept", "application/json")
	
	var issuesResp struct {
		Total int `json:"total"`
	}
	
	if err := client.DoJSONRequest(ctx, req, &issuesResp); err != nil {
		return 0 // Graceful degradation on any error
	}
	
	return issuesResp.Total
}

func RankBoards(boards []Board, currentProjects []string) []Board {
	// Load cached activity data if available
	activityMap := make(map[int]int) // boardID -> activity count
	cacheFile := getCacheFilePath()
	if cached, ok := loadFromCache(cacheFile); ok {
		for _, bwa := range cached {
			activityMap[bwa.Board.ID] = bwa.RecentActivity
		}
	}
	
	scored := make([]struct {
		board Board
		score int
	}, len(boards))

	for i, board := range boards {
		score := 0
		
		// Project affinity (highest weight)
		for _, project := range currentProjects {
			if board.Location.ProjectKey == project {
				score += 100
			}
		}
		
		// Recent activity (medium weight)
		if activity, ok := activityMap[board.ID]; ok {
			// Cap activity bonus at 50 points to prevent overwhelming other factors
			activityBonus := activity
			if activityBonus > 50 {
				activityBonus = 50
			}
			score += activityBonus
		}
		
		// Board type preference (low weight)
		if board.Type == "scrum" {
			score += 5
		} else if board.Type == "kanban" {
			score += 3
		}
		
		// Name-based heuristics for relevance
		boardName := strings.ToLower(board.Name)
		if strings.Contains(boardName, "active") || strings.Contains(boardName, "current") {
			score += 2
		}
		if strings.Contains(boardName, "deprecated") || strings.Contains(boardName, "old") {
			score -= 5
		}

		scored[i] = struct {
			board Board
			score int
		}{board, score}
	}

	// Sort by score (deterministic - uses board ID as tiebreaker for consistency)
	for i := 0; i < len(scored)-1; i++ {
		for j := i + 1; j < len(scored); j++ {
			if scored[i].score < scored[j].score || 
			   (scored[i].score == scored[j].score && scored[i].board.ID > scored[j].board.ID) {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}

	result := make([]Board, len(scored))
	for i, s := range scored {
		result[i] = s.board
	}
	return result
}