package vacancy

import "testing"

func TestSelectBuildingValue(t *testing.T) {
	zero, positive := 0.0, 123.0
	for _, test := range []struct {
		name      string
		rows      []ValueRow
		year      int
		want      *bool
		ambiguous bool
	}{
		{"duplicate zero", []ValueRow{{2026, "AV", &zero}, {2026, "AV", &zero}}, 2026, ptr(false), false},
		{"positive with prior year", []ValueRow{{2026, "AV", &positive}, {2026, "PY", &zero}}, 2026, ptr(true), false},
		{"competing appraisal", []ValueRow{{2026, "AV", &zero}, {2026, "AV", &positive}}, 2026, nil, true},
		{"missing value", []ValueRow{{2026, "AV", nil}}, 2026, nil, true},
		{"unapproved year", []ValueRow{{2027, "AV", &zero}}, 2026, nil, true},
		{"no approved year", []ValueRow{{2027, "AV", &zero}}, 0, nil, true},
		{"prior year only", []ValueRow{{2026, "PY", &zero}}, 2026, nil, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, ambiguous := SelectBuildingValue(test.rows, test.year)
			if ambiguous != test.ambiguous || (got == nil) != (test.want == nil) || got != nil && *got != *test.want {
				t.Fatalf("got %v ambiguous %v", got, ambiguous)
			}
		})
	}
}
