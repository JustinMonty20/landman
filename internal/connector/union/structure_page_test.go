package union

import (
	"fmt"
	"strings"
	"testing"
)

func TestValidateStructurePageTransferLimit(t *testing.T) {
	ids := make([]int64, 2001)
	for i := range ids {
		ids[i] = int64(i + 1)
	}
	page := func(count int, flag bool, first int) []byte {
		var b strings.Builder
		b.WriteString(`{"features":[`)
		for i := 0; i < count; i++ {
			if i > 0 {
				b.WriteByte(',')
			}
			fmt.Fprintf(&b, `{"attributes":{"OBJECTID":%d}}`, first+i)
		}
		fmt.Fprintf(&b, `],"exceededTransferLimit":%t}`, flag)
		return []byte(b.String())
	}
	for _, tc := range []struct {
		name      string
		body      []byte
		offset    int
		wantError string
	}{
		{"full intermediate flagged", page(2000, true, 1), 0, ""},
		{"full intermediate unflagged", page(2000, false, 1), 0, ""},
		{"final unflagged", page(1, false, 2001), 2000, ""},
		{"short intermediate flagged", page(1999, true, 1), 0, "incomplete"},
		{"final flagged", page(1, true, 2001), 2000, "final page reports transfer limit"},
		{"shifted ID flagged", page(2000, true, 2), 0, "membership drift"},
		{"malformed", []byte(`{"features":[`), 0, "incomplete"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateStructurePage(tc.body, tc.offset, len(ids), ids)
			if tc.wantError == "" && err != nil || tc.wantError != "" && (err == nil || !strings.Contains(err.Error(), tc.wantError)) {
				t.Fatalf("error = %v, want %q", err, tc.wantError)
			}
		})
	}
}
