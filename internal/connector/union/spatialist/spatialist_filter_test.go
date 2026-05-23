package spatialist

import "testing"

func TestKeepWhenYearBuiltIsNil(t *testing.T) {
	tests := []struct {
		name string
		raw  map[string]interface{}
		want bool
	}{
		{
			name: "keep when YEARBLT is nil",
			raw:  rawWithSection2Entry(map[string]interface{}{"YEARBLT": nil}),
			want: true,
		},
		{
			name: "filter out when YEARBLT is numeric",
			raw:  rawWithSection2Entry(map[string]interface{}{"YEARBLT": float64(1998)}),
			want: false,
		},
		{
			name: "filter out when YEARBLT is string",
			raw:  rawWithSection2Entry(map[string]interface{}{"YEARBLT": "1998"}),
			want: false,
		},
		{
			name: "keep when YEARBLT key is missing",
			raw:  rawWithSection2Entry(map[string]interface{}{"OTHER": "value"}),
			want: true,
		},
		{
			name: "keep when path is malformed",
			raw: map[string]interface{}{
				"parcel": map[string]interface{}{
					"sections": []interface{}{"not-an-array"},
				},
			},
			want: true,
		},
		{
			name: "keep when raw is nil",
			raw:  nil,
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := KeepWhenYearBuiltIsNil(tt.raw, nil)
			if got != tt.want {
				t.Fatalf("KeepWhenYearBuiltIsNil() = %v, want %v", got, tt.want)
			}
		})
	}
}

func rawWithSection2Entry(entry map[string]interface{}) map[string]interface{} {
	sections := make([]interface{}, 3)
	sections[0] = []interface{}{}
	sections[1] = []interface{}{}
	sections[2] = []interface{}{
		[]interface{}{entry},
	}
	return map[string]interface{}{
		"parcel": map[string]interface{}{
			"sections": sections,
		},
	}
}
