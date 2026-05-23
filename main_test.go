package main

import (
	"reflect"
	"testing"

	"github.com/JustinMonty20/landman/internal/connector/union/spatialist"
)

func TestSortedParcelIDs(t *testing.T) {
	t.Run("returns sorted non-empty parcel IDs", func(t *testing.T) {
		input := map[string]*spatialist.SpatialistFlatEnvelope{
			"P2": {},
			"P1": {},
			"":   {},
		}

		got := sortedParcelIDs(input)
		want := []string{"P1", "P2"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("sortedParcelIDs() = %v, want %v", got, want)
		}
	})

	t.Run("returns nil on empty input", func(t *testing.T) {
		got := sortedParcelIDs(nil)
		if got != nil {
			t.Fatalf("sortedParcelIDs() = %v, want nil", got)
		}
	})
}
