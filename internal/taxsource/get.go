// Package taxsource defines shared tax-file retrieval and streaming contracts.
package taxsource

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

var errBodyTooLarge = errors.New("response body exceeds limit")

// Get retrieves one file body. It retries one transient failure before any body is read.
func Get(ctx context.Context, rawURL string, limit int64, timeout time.Duration) (io.ReadCloser, error) {
	if limit <= 0 {
		return nil, errors.New("response body limit must be greater than zero")
	}
	if timeout <= 0 {
		return nil, errors.New("request timeout must be greater than zero")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: timeout}
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		body, retry, err := getOnce(ctx, client, rawURL)
		if err == nil {
			return &limitedReadCloser{body: body, remaining: limit}, nil
		}
		if !retry || !retryableTransportError(err) {
			return nil, err
		}
		lastErr = err
	}
	return nil, lastErr
}

func retryableTransportError(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var networkError net.Error
	return !errors.As(err, &networkError) || !networkError.Timeout()
}

func getOnce(ctx context.Context, client *http.Client, rawURL string) (io.ReadCloser, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, fmt.Errorf("create tax source request: %w", err)
	}

	response, err := client.Do(req)
	if err != nil {
		return nil, true, fmt.Errorf("get tax source: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		response.Body.Close()
		retry := response.StatusCode >= http.StatusInternalServerError && response.StatusCode <= 599
		return nil, retry, fmt.Errorf("get tax source: unexpected HTTP status %s", response.Status)
	}
	return response.Body, false, nil
}

type limitedReadCloser struct {
	body      io.ReadCloser
	remaining int64
}

func (r *limitedReadCloser) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.remaining > 0 {
		if int64(len(p)) > r.remaining {
			p = p[:r.remaining]
		}
		n, err := r.body.Read(p)
		r.remaining -= int64(n)
		return n, err
	}

	var extra [1]byte
	n, err := r.body.Read(extra[:])
	if n > 0 {
		_ = r.body.Close()
		return 0, errBodyTooLarge
	}
	return 0, err
}

func (r *limitedReadCloser) Close() error {
	return r.body.Close()
}
