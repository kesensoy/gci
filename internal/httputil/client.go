package httputil

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
	
	"gci/internal/errors"
)

// DefaultTimeout is the standard timeout for HTTP requests
const DefaultTimeout = 30 * time.Second

// RetryableClient provides HTTP operations with consistent timeout and retry behavior
type RetryableClient struct {
	client  *http.Client
	timeout time.Duration
	retries int
}

// NewRetryableClient creates a new HTTP client with timeout and retry configuration
func NewRetryableClient(timeout time.Duration, retries int) *RetryableClient {
	return &RetryableClient{
		client: &http.Client{
			Timeout: timeout,
		},
		timeout: timeout,
		retries: retries,
	}
}

// NewDefaultClient creates a client with standard timeout and retry settings
func NewDefaultClient() *RetryableClient {
	return NewRetryableClient(DefaultTimeout, 2)
}

// DoWithRetry executes an HTTP request with retry logic for transient errors
func (c *RetryableClient) DoWithRetry(ctx context.Context, req *http.Request) (*http.Response, error) {
	// Set context with timeout if not already set
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	var lastErr error
	
	for attempt := 0; attempt <= c.retries; attempt++ {
		// Clone request with context
		reqWithCtx := req.Clone(ctx)
		
		resp, err := c.client.Do(reqWithCtx)
		if err != nil {
			lastErr = fmt.Errorf("HTTP request failed (attempt %d/%d): %w", attempt+1, c.retries+1, err)
			if attempt < c.retries {
				// Wait before retry with exponential backoff
				waitTime := time.Duration(attempt+1) * 500 * time.Millisecond
				select {
				case <-time.After(waitTime):
					continue
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			continue
		}

		// Check if we should retry based on status code
		if shouldRetry(resp.StatusCode) && attempt < c.retries {
			resp.Body.Close()
			lastErr = fmt.Errorf("HTTP request returned retryable status %d (attempt %d/%d)", resp.StatusCode, attempt+1, c.retries+1)
			
			// Wait before retry
			waitTime := time.Duration(attempt+1) * 500 * time.Millisecond
			select {
			case <-time.After(waitTime):
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		return resp, nil
	}

	return nil, lastErr
}

// DoJSONRequest executes a JSON request with retry logic and decodes the response
func (c *RetryableClient) DoJSONRequest(ctx context.Context, req *http.Request, result interface{}) error {
	resp, err := c.DoWithRetry(ctx, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Read error body for debugging
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return errors.NewHttpError(resp.StatusCode, string(body))
	}

	return json.NewDecoder(resp.Body).Decode(result)
}

// shouldRetry determines if a status code indicates a retryable error
func shouldRetry(statusCode int) bool {
	switch statusCode {
	case http.StatusInternalServerError,     // 500
		http.StatusBadGateway,               // 502  
		http.StatusServiceUnavailable,       // 503
		http.StatusGatewayTimeout,           // 504
		http.StatusInsufficientStorage,      // 507
		http.StatusNetworkAuthenticationRequired: // 511
		return true
	default:
		return false
	}
}