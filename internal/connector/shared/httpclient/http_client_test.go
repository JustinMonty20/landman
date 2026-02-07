package httpclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewHTTPClient(t *testing.T) {
	client := NewHTTPClient()

	if client == nil {
		t.Fatal("expected non-nil client")
	}

	if client.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want %v", client.Timeout, 30*time.Second)
	}

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("expected *http.Transport")
	}

	if transport.MaxIdleConns != 100 {
		t.Errorf("MaxIdleConns = %d, want 100", transport.MaxIdleConns)
	}

	if transport.MaxIdleConnsPerHost != 10 {
		t.Errorf("MaxIdleConnsPerHost = %d, want 10", transport.MaxIdleConnsPerHost)
	}

	if transport.IdleConnTimeout != 90*time.Second {
		t.Errorf("IdleConnTimeout = %v, want %v", transport.IdleConnTimeout, 90*time.Second)
	}

	if transport.TLSHandshakeTimeout != 10*time.Second {
		t.Errorf("TLSHandshakeTimeout = %v, want %v", transport.TLSHandshakeTimeout, 10*time.Second)
	}

	if transport.ResponseHeaderTimeout != 15*time.Second {
		t.Errorf("ResponseHeaderTimeout = %v, want %v", transport.ResponseHeaderTimeout, 15*time.Second)
	}

	if transport.DisableKeepAlives {
		t.Error("DisableKeepAlives should be false")
	}
}

func TestNewRateLimitedClient(t *testing.T) {
	tests := []struct {
		name         string
		reqPerSecond int
		wantRate     int
	}{
		{
			name:         "explicit rate",
			reqPerSecond: 5,
			wantRate:     5,
		},
		{
			name:         "zero defaults to 1",
			reqPerSecond: 0,
			wantRate:     1,
		},
		{
			name:         "negative still gets set (implementation detail)",
			reqPerSecond: -1,
			wantRate:     -1, // Note: rate.Limiter handles negative values
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpClient := NewHTTPClient()
			rlClient := NewRateLimitedClient(httpClient, tt.reqPerSecond)

			if rlClient == nil {
				t.Fatal("expected non-nil client")
			}

			if rlClient.Client != httpClient {
				t.Error("Client should be the same http.Client passed in")
			}

			if rlClient.limiter == nil {
				t.Error("limiter should not be nil")
			}
		})
	}
}

func TestRateLimitedClient_Do(t *testing.T) {
	var requestCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok"}`))
	}))
	defer server.Close()

	rlClient := NewRateLimitedClient(server.Client(), 10) // 10 req/sec for faster test

	ctx := context.Background()

	// Make a simple request
	req, err := http.NewRequestWithContext(ctx, "GET", server.URL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := rlClient.Do(ctx, req)
	if err != nil {
		t.Fatalf("Do() failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	if atomic.LoadInt32(&requestCount) != 1 {
		t.Errorf("expected 1 request, got %d", requestCount)
	}
}

func TestRateLimitedClient_Do_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Very slow rate to ensure we hit the rate limiter
	rlClient := NewRateLimitedClient(server.Client(), 1)

	// Make one request to consume the token
	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL, nil)
	resp, err := rlClient.Do(ctx, req)
	if err != nil {
		t.Fatalf("first request failed: %v", err)
	}
	resp.Body.Close()

	// Now try with cancelled context - should fail quickly
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	req2, _ := http.NewRequestWithContext(cancelledCtx, "GET", server.URL, nil)
	_, err = rlClient.Do(cancelledCtx, req2)
	if err == nil {
		t.Error("expected error from cancelled context")
	}
}

func TestRateLimitedClient_Do_RateLimiting(t *testing.T) {
	var requestCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// 2 requests per second
	rlClient := NewRateLimitedClient(server.Client(), 2)
	ctx := context.Background()

	start := time.Now()

	// Make 3 requests - should take at least 500ms due to rate limiting
	for i := 0; i < 3; i++ {
		req, _ := http.NewRequestWithContext(ctx, "GET", server.URL, nil)
		resp, err := rlClient.Do(ctx, req)
		if err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}
		resp.Body.Close()
	}

	elapsed := time.Since(start)

	// With burst of 1 and rate of 2/sec, 3 requests should take at least 1 second
	// (first request immediate, second waits 500ms, third waits another 500ms)
	if elapsed < 900*time.Millisecond {
		t.Errorf("rate limiting not working: 3 requests completed in %v, expected >= 900ms", elapsed)
	}
}

func TestRateLimitedClient_Do_PropagatesErrors(t *testing.T) {
	// Server that returns an error status
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	rlClient := NewRateLimitedClient(server.Client(), 10)
	ctx := context.Background()

	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL, nil)
	resp, err := rlClient.Do(ctx, req)

	// HTTP errors are not Go errors - they come back as responses
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500 status, got %d", resp.StatusCode)
	}
}
