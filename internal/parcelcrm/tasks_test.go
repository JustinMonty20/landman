package parcelcrm

import (
	"strings"
	"testing"
	"time"
)

func TestMessageSegments(t *testing.T) {
	for _, tc := range []struct {
		message string
		valid   bool
	}{
		{strings.Repeat("a", 160), true}, {strings.Repeat("a", 161), false},
		{strings.Repeat("^", 80), true}, {strings.Repeat("^", 81), false},
		{strings.Repeat("é", 160), true}, {strings.Repeat("界", 70), true}, {strings.Repeat("界", 71), false},
		{strings.Repeat("😀", 35), true}, {strings.Repeat("😀", 36), false},
		{"  ", false}, {"\xff", false},
	} {
		if validMessage(tc.message) != tc.valid {
			t.Errorf("message %q: expected %t", tc.message, tc.valid)
		}
	}
}
func TestDueOffsetAndDST(t *testing.T) {
	for _, tc := range []struct {
		when  string
		valid bool
	}{
		{"2027-03-14T02:30:00-05:00", false}, // spring skipped hour
		{"2027-11-07T01:30:00-04:00", true},
		{"2027-11-07T01:30:00-05:00", true},
		{"2027-11-07T01:30:00-06:00", false},
		{"2027-11-07T01:30:00", false},
	} {
		_, e := parseDue(tc.when, "America/New_York")
		if (e == nil) != tc.valid {
			t.Errorf("due %s: %v", tc.when, e)
		}
	}
	a, _ := parseDue("2027-11-07T01:30:00-04:00", "America/New_York")
	b, _ := parseDue("2027-11-07T01:30:00-05:00", "America/New_York")
	if b.Sub(a) != time.Hour {
		t.Fatal("ambiguous offsets must resolve to different instants")
	}
}
