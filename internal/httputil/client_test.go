package httputil

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRetryableClient_DoWithRetry_Success(t *testing.T) {
	// Create a test server that returns OK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success": true}`))
	}))
	defer server.Close()

	client := NewRetryableClient(5*time.Second, 2)
	req, err := http.NewRequest("GET", server.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	ctx := context.Background()
	resp, err := client.DoWithRetry(ctx, req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestRetryableClient_DoWithRetry_Timeout(t *testing.T) {
	// Create a test server that delays response beyond timeout
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second) // Longer than timeout
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewRetryableClient(500*time.Millisecond, 0) // Short timeout, no retries
	req, err := http.NewRequest("GET", server.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	ctx := context.Background()
	_, err = client.DoWithRetry(ctx, req)
	if err == nil {
		t.Error("Expected timeout error, but got none")
	}
}

func TestRetryableClient_DoWithRetry_RetryOn500(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success": true}`))
	}))
	defer server.Close()

	client := NewRetryableClient(5*time.Second, 3)
	req, err := http.NewRequest("GET", server.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	ctx := context.Background()
	resp, err := client.DoWithRetry(ctx, req)
	if err != nil {
		t.Fatalf("Request failed after retries: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	if attempts != 3 {
		t.Errorf("Expected 3 attempts, got %d", attempts)
	}
}

func TestRetryableClient_DoWithRetry_NoRetryOn400(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadRequest) // 400 should not be retried
	}))
	defer server.Close()

	client := NewRetryableClient(5*time.Second, 3)
	req, err := http.NewRequest("GET", server.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	ctx := context.Background()
	resp, err := client.DoWithRetry(ctx, req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", resp.StatusCode)
	}

	if attempts != 1 {
		t.Errorf("Expected 1 attempt (no retries), got %d", attempts)
	}
}

func TestRetryableClient_DoJSONRequest(t *testing.T) {
	// Create a test server that returns JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"message": "hello", "count": 42}`))
	}))
	defer server.Close()

	client := NewDefaultClient()
	req, err := http.NewRequest("GET", server.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	ctx := context.Background()
	var result struct {
		Message string `json:"message"`
		Count   int    `json:"count"`
	}

	err = client.DoJSONRequest(ctx, req, &result)
	if err != nil {
		t.Fatalf("JSON request failed: %v", err)
	}

	if result.Message != "hello" {
		t.Errorf("Expected message 'hello', got '%s'", result.Message)
	}

	if result.Count != 42 {
		t.Errorf("Expected count 42, got %d", result.Count)
	}
}