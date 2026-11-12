package soil

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSoilNamesBatchAndSurveyDate(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.ParseForm() != nil || !strings.Contains(r.Form.Get("query"), "'115341','115333'") {
			t.Errorf("invalid USDA request: %s %v", r.Method, r.Form)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Table":[["115341","ChA","Chewacla silt loam","NC179","Union County, North Carolina","9/3/2026 1:43:46 PM"],["115333","BaB","Badin channery silt loam","NC179","Union County, North Carolina","9/3/2026 1:43:46 PM"]]}`))
	}))
	defer server.Close()
	names, err := fetchNames(context.Background(), server.Client(), server.URL, []string{"115341", "115333"})
	if err != nil || calls != 1 || len(names) != 2 {
		t.Fatalf("names=%v calls=%d err=%v", names, calls, err)
	}
	if names["115341"].name != "Chewacla silt loam" || names["115341"].surveyDate.Format("2006-01-02") != "2026-09-03" {
		t.Fatal(names["115341"])
	}
}

func TestSoilNamesTimeoutAndCallerCancellation(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)
	start := time.Now()
	if _, err := fetchNames(context.Background(), server.Client(), server.URL, []string{"1"}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("stalled lookup: %v", err)
	}
	if elapsed := time.Since(start); elapsed > nameLookupTimeout+3*time.Second {
		t.Fatalf("optional lookup exceeded bound: %s", elapsed)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := fetchNames(ctx, server.Client(), server.URL, []string{"1"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("caller cancellation: %v", err)
	}
}

func TestSoilNamesFailureAndCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "unavailable", 500) }))
	defer server.Close()
	if _, err := fetchNames(context.Background(), server.Client(), server.URL, []string{"1"}); err == nil {
		t.Fatal("expected HTTP error")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := fetchNames(ctx, server.Client(), server.URL, []string{"1"}); err == nil {
		t.Fatal("expected cancellation")
	}
	if names, err := fetchNames(context.Background(), server.Client(), server.URL, nil); err != nil || names != nil {
		t.Fatal(names, err)
	}
}
