package heapprof_test

import (
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/heapprof"
)

func TestDisabled(t *testing.T) {
	stop, err := heapprof.Start("")
	if err != nil {
		t.Fatal(err)
	}
	stop()
}

func TestHeap(t *testing.T) {
	parent := t.TempDir()
	stop, err := heapprof.Start(parent)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		stop()
		entries, err := os.ReadDir(parent)
		if err != nil || len(entries) != 0 {
			t.Errorf("directory after stop: %v, %v", entries, err)
		}
	}()

	paths, err := filepath.Glob(filepath.Join(parent, "heap-*", "heap.sock"))
	if err != nil || len(paths) != 1 {
		t.Fatalf("heap sockets: %v, %v", paths, err)
	}
	info, err := os.Stat(filepath.Dir(paths[0]))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("heap directory permissions: %o", info.Mode().Perm())
	}

	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", paths[0])
		},
	}}
	defer client.CloseIdleConnections()
	for _, check := range []struct {
		method string
		path   string
		status int
	}{
		{http.MethodGet, "/heap", http.StatusOK},
		{http.MethodPost, "/heap", http.StatusMethodNotAllowed},
		{http.MethodGet, "/heap?gc=1", http.StatusNotFound},
		{http.MethodGet, "/debug/pprof/goroutine", http.StatusNotFound},
	} {
		t.Run(check.method+check.path, func(t *testing.T) {
			req, err := http.NewRequest(check.method, "http://localhost"+check.path, nil)
			if err != nil {
				t.Fatal(err)
			}
			res, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			if res.StatusCode != check.status {
				t.Fatalf("status = %d, want %d", res.StatusCode, check.status)
			}
			if check.status != http.StatusOK {
				return
			}
			profile, err := gzip.NewReader(res.Body)
			if err != nil {
				t.Fatal(err)
			}
			defer profile.Close()
			size, err := io.Copy(io.Discard, profile)
			if err != nil || size == 0 {
				t.Fatalf("profile size = %d, err = %v", size, err)
			}
		})
	}
}

func TestListenFailureRemovesDirectory(t *testing.T) {
	parent := filepath.Join(t.TempDir(), strings.Repeat("a", 110))
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	stop, err := heapprof.Start(parent)
	if err == nil {
		stop()
		t.Fatal("expected error for long Unix socket path")
	}
	var opErr *net.OpError
	if !errors.As(err, &opErr) {
		t.Fatalf("expected listen error, got %v", err)
	}
	entries, err := os.ReadDir(parent)
	if err != nil || len(entries) != 0 {
		t.Fatalf("directory after listen failure: %v, %v", entries, err)
	}
}
