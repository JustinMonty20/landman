// Package heapprof serves sampled Go heap profiles on a private Unix socket.
package heapprof

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime/pprof"
	"time"
)

// Start serves heap profiles in a new private directory under parent. An empty
// parent disables capture. Call stop to close the socket and remove the directory.
func Start(parent string) (stop func(), err error) {
	if parent == "" {
		return func() {}, nil
	}
	dir, err := os.MkdirTemp(parent, "heap-")
	if err != nil {
		return nil, fmt.Errorf("create heap profile directory: %w", err)
	}
	path := filepath.Join(dir, "heap.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		removeDir(dir)
		return nil, fmt.Errorf("listen on heap profile socket: %w", err)
	}

	active := make(chan struct{}, 1)
	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/heap" || r.URL.RawQuery != "" {
				http.NotFound(w, r)
				return
			}
			if r.Method != http.MethodGet {
				w.Header().Set("Allow", http.MethodGet)
				http.Error(w, "Use GET", http.StatusMethodNotAllowed)
				return
			}
			select {
			case active <- struct{}{}:
				defer func() { <-active }()
			default:
				http.Error(w, "Heap profile capture is active", http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			if err := pprof.Lookup("heap").WriteTo(w, 0); err != nil {
				log.Printf("write heap profile: %v", err)
			}
		}),
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       5 * time.Second,
		MaxHeaderBytes:    4 << 10,
	}
	log.Printf("Heap profile socket: %s (PID %d)", path, os.Getpid())
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("serve heap profiles: %v", err)
		}
	}()
	return func() {
		_ = server.Close()
		_ = listener.Close() // Also close it if Serve has not started.
		removeDir(dir)
	}, nil
}

func removeDir(dir string) {
	if err := os.Remove(dir); err != nil {
		log.Printf("remove heap profile directory: %v", err)
	}
}
