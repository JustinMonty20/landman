package taxsource

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestGet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		limit      int64
		cancel     bool
		handler    func(http.ResponseWriter, int32)
		wantBody   string
		wantErr    error
		wantsError bool
		wantHits   int32
	}{
		{
			name:     "returns a successful body",
			limit:    10,
			wantBody: "ok",
			wantHits: 1,
			handler: func(w http.ResponseWriter, _ int32) {
				_, _ = io.WriteString(w, "ok")
			},
		},
		{
			name:       "does not retry not found",
			limit:      10,
			wantsError: true,
			wantHits:   1,
			handler: func(w http.ResponseWriter, _ int32) {
				http.NotFound(w, nil)
			},
		},
		{
			name:     "retries server error before body",
			limit:    10,
			wantBody: "ok",
			wantHits: 2,
			handler: func(w http.ResponseWriter, hit int32) {
				if hit == 1 {
					http.Error(w, "temporary", http.StatusInternalServerError)
					return
				}
				_, _ = io.WriteString(w, "ok")
			},
		},
		{
			name:     "rejects body one byte over limit",
			limit:    2,
			wantErr:  errBodyTooLarge,
			wantHits: 1,
			handler: func(w http.ResponseWriter, _ int32) {
				_, _ = io.WriteString(w, "too")
			},
		},
		{
			name:     "does not start a canceled request",
			limit:    10,
			cancel:   true,
			wantErr:  context.Canceled,
			wantHits: 0,
			handler: func(http.ResponseWriter, int32) {
				t.Fatal("handler must not run")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var hits atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				test.handler(w, hits.Add(1))
			}))
			defer server.Close()

			ctx := context.Background()
			if test.cancel {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			body, err := Get(ctx, server.URL, test.limit, time.Second)
			if err == nil {
				defer body.Close()
				bytes, readErr := io.ReadAll(body)
				if readErr != nil {
					err = readErr
				} else if string(bytes) != test.wantBody {
					t.Fatalf("body = %q, want %q", bytes, test.wantBody)
				}
			}
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("error = %v, want an error matching %v", err, test.wantErr)
				}
			} else if test.wantsError {
				if err == nil {
					t.Fatal("Get() error = nil")
				}
			} else if err != nil {
				t.Fatalf("Get() error = %v", err)
			}
			if got := hits.Load(); got != test.wantHits {
				t.Fatalf("hits = %d, want %d", got, test.wantHits)
			}
		})
	}
}

func TestGetDoesNotRetryStatusAbove599(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(600)
	}))
	defer server.Close()

	_, err := Get(context.Background(), server.URL, 10, time.Second)
	if err == nil {
		t.Fatal("Get() error = nil")
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("hits = %d, want 1", got)
	}
}

func TestGetStopsAfterSecondServerError(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		http.Error(w, "temporary", http.StatusInternalServerError)
	}))
	defer server.Close()

	_, err := Get(context.Background(), server.URL, 10, time.Second)
	if err == nil {
		t.Fatal("Get() error = nil")
	}
	if got := hits.Load(); got != 2 {
		t.Fatalf("hits = %d, want 2", got)
	}
}

func TestLimitedReadCloserAllowsZeroLengthRead(t *testing.T) {
	reader := &limitedReadCloser{body: io.NopCloser(strings.NewReader("ok")), remaining: 2}
	defer reader.Close()

	n, err := reader.Read([]byte{})
	if n != 0 || err != nil {
		t.Fatalf("Read([]byte{}) = %d, %v; want 0, nil", n, err)
	}
}

func TestGetReadsMeasuredDelinquentFileSizeUnderOneSecond(t *testing.T) {
	body := strings.Repeat("x", 300_371)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, body)
	}))
	defer server.Close()

	started := time.Now()
	response, err := Get(context.Background(), server.URL, int64(len(body)), time.Second)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	defer response.Close()
	got, err := io.ReadAll(response)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if len(got) != len(body) {
		t.Fatalf("body length = %d, want %d", len(got), len(body))
	}
	if elapsed := time.Since(started); elapsed >= time.Second {
		t.Fatalf("Get() and ReadAll() took %s, want less than one second", elapsed)
	}
}

func TestGetRejectsInvalidLimits(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name    string
		limit   int64
		timeout time.Duration
	}{
		{name: "zero limit", timeout: time.Second},
		{name: "zero timeout", limit: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := Get(context.Background(), "http://example.invalid", test.limit, test.timeout)
			if err == nil {
				t.Fatal("Get() error = nil")
			}
		})
	}
}

type timeoutError struct{}

func (timeoutError) Error() string   { return "timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return false }

func TestRetryableTransportErrorRejectsNetworkTimeout(t *testing.T) {
	if retryableTransportError(timeoutError{}) {
		t.Fatal("retryableTransportError(timeoutError{}) = true, want false")
	}
}

func TestGetTimeoutDoesNotRetry(t *testing.T) {
	var hits atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		hits.Add(1)
		close(started)
		<-release
	}))
	defer server.Close()

	done := make(chan error, 1)
	go func() {
		_, err := Get(context.Background(), server.URL, 10, 10*time.Millisecond)
		done <- err
	}()
	<-started
	err := <-done
	close(release)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want an error matching context deadline exceeded", err)
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("hits = %d, want 1", got)
	}
}
