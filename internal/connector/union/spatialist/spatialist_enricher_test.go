package spatialist

import (
	"context"
	"errors"
	"testing"

	"github.com/JustinMonty20/landman/internal/connector"
)

type fakeBatchEnricher struct {
	records map[string]map[string]connector.RawRecord
	err     error
	calls   int
}

func (f *fakeBatchEnricher) EnrichBatch(ctx context.Context, parcelIDs []string) (map[string]map[string]connector.RawRecord, error) {
	f.calls++
	return f.records, f.err
}

func TestSpatialistEnricher_EnrichBatch_Success(t *testing.T) {
	fake := &fakeBatchEnricher{
		records: map[string]map[string]connector.RawRecord{
			"P1": {"spatialist": {RawData: map[string]interface{}{"id": "P1"}}},
			"P2": {"spatialist": {RawData: map[string]interface{}{"id": "P2"}}},
		},
	}

	transform := func(raw map[string]interface{}) (*SpatialistFlatEnvelope, error) {
		id, _ := raw["id"].(string)
		return &SpatialistFlatEnvelope{
			UCRealPropertySearch: &SpatialistFlatRecord{ParcelID: id},
		}, nil
	}

	svc, err := NewSpatialistEnricher(fake, "spatialist", transform)
	if err != nil {
		t.Fatalf("NewSpatialistEnricher error: %v", err)
	}

	results, err := svc.EnrichBatch(context.Background(), []string{"P1", "P2"})
	if err != nil {
		t.Fatalf("EnrichBatch error: %v", err)
	}

	if got := results["P1"].UCRealPropertySearch.ParcelID; got != "P1" {
		t.Fatalf("P1 ParcelID = %q, want %q", got, "P1")
	}
	if got := results["P2"].UCRealPropertySearch.ParcelID; got != "P2" {
		t.Fatalf("P2 ParcelID = %q, want %q", got, "P2")
	}
}

func TestSpatialistEnricher_EnrichBatch_EmptyInput(t *testing.T) {
	fake := &fakeBatchEnricher{}
	transform := func(raw map[string]interface{}) (*SpatialistFlatEnvelope, error) {
		return &SpatialistFlatEnvelope{}, nil
	}

	svc, err := NewSpatialistEnricher(fake, "spatialist", transform)
	if err != nil {
		t.Fatalf("NewSpatialistEnricher error: %v", err)
	}

	results, err := svc.EnrichBatch(context.Background(), nil)
	if err != nil {
		t.Fatalf("EnrichBatch error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("results len = %d, want 0", len(results))
	}
	if fake.calls != 0 {
		t.Fatalf("expected batch enricher to not be called")
	}
}

func TestSpatialistEnricher_EnrichBatch_MissingSource(t *testing.T) {
	fake := &fakeBatchEnricher{
		records: map[string]map[string]connector.RawRecord{
			"P1": {"other": {RawData: map[string]interface{}{"id": "P1"}}},
		},
	}

	transform := func(raw map[string]interface{}) (*SpatialistFlatEnvelope, error) {
		return &SpatialistFlatEnvelope{}, nil
	}

	svc, err := NewSpatialistEnricher(fake, "spatialist", transform)
	if err != nil {
		t.Fatalf("NewSpatialistEnricher error: %v", err)
	}

	if _, err := svc.EnrichBatch(context.Background(), []string{"P1"}); err == nil {
		t.Fatalf("expected error for missing source")
	}
}

func TestSpatialistEnricher_EnrichBatch_TransformError(t *testing.T) {
	fake := &fakeBatchEnricher{
		records: map[string]map[string]connector.RawRecord{
			"P1": {"spatialist": {RawData: map[string]interface{}{"id": "P1"}}},
		},
	}

	transform := func(raw map[string]interface{}) (*SpatialistFlatEnvelope, error) {
		return nil, errors.New("boom")
	}

	svc, err := NewSpatialistEnricher(fake, "spatialist", transform)
	if err != nil {
		t.Fatalf("NewSpatialistEnricher error: %v", err)
	}

	if _, err := svc.EnrichBatch(context.Background(), []string{"P1"}); err == nil {
		t.Fatalf("expected error for transform failure")
	}
}

func TestSpatialistEnricher_EnrichBatch_BatchError(t *testing.T) {
	fake := &fakeBatchEnricher{
		records: map[string]map[string]connector.RawRecord{
			"P1": {"spatialist": {RawData: map[string]interface{}{"id": "P1"}}},
		},
		err: errors.New("fetch failed"),
	}

	transform := func(raw map[string]interface{}) (*SpatialistFlatEnvelope, error) {
		return &SpatialistFlatEnvelope{
			UCRealPropertySearch: &SpatialistFlatRecord{ParcelID: "P1"},
		}, nil
	}

	svc, err := NewSpatialistEnricher(fake, "spatialist", transform)
	if err != nil {
		t.Fatalf("NewSpatialistEnricher error: %v", err)
	}

	results, err := svc.EnrichBatch(context.Background(), []string{"P1"})
	if err == nil {
		t.Fatalf("expected error for batch failure")
	}
	if results["P1"] == nil {
		t.Fatalf("expected results to include P1 despite batch error")
	}
}
