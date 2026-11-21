package vacancyjob

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/JustinMonty20/landman/internal/ingest"
	"github.com/JustinMonty20/landman/internal/taxartifact"
)

func TestLegacyAssessmentRangeReplayOffline(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := taxartifact.NewLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	root := union.AssessmentWhere(0, math.MaxInt64)
	left := union.AssessmentWhere(0, math.MaxInt64/2)
	right := union.AssessmentWhere(math.MaxInt64/2+1, math.MaxInt64)
	page := func(oid int64) string {
		var b strings.Builder
		b.WriteString(`{"features":[`)
		for i := 0; i < 1001; i++ {
			if i != 0 {
				b.WriteByte(',')
			}
			fmt.Fprintf(&b, `{"attributes":{"OBJECTID":%d,"parcel_number":"001","assess_category":"AV","market_building":0}}`, oid)
		}
		b.WriteString(`]}`)
		return b.String()
	}
	countQuery := func(where string) url.Values {
		return url.Values{"f": {"json"}, "where": {where}, "returnCountOnly": {"true"}}
	}
	pageQuery := func(where string) url.Values {
		return url.Values{"f": {"json"}, "where": {where}, "returnGeometry": {"false"}, "outFields": {"*"}, "resultRecordCount": {"2000"}}
	}
	sequence := []struct {
		query url.Values
		body  string
	}{
		{countQuery("1=1"), `{"count":2002}`},
		{countQuery(root), `{"count":2002}`},
		{countQuery(left), `{"count":1001}`},
		{countQuery(left), `{"count":1001}`},
		{pageQuery(left), page(1)},
		{countQuery(left), `{"count":1001}`},
		{countQuery(right), `{"count":1001}`},
		{pageQuery(right), page(math.MaxInt64)},
		{countQuery(right), `{"count":1001}`},
		{countQuery("1=1"), `{"count":2002}`},
	}
	record := func(receipts int) ingest.VerifiedCapture {
		t.Helper()
		r, err := ingest.NewRecorder(store, "union_assessed_value_0", time.Now())
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range sequence[:receipts] {
			request := "https://offline.invalid/Property_Tax_Live/Assessed_Value/MapServer/0/query?" + item.query.Encode()
			if err := r.RecordHTTP(ctx, request, "", 0, 200, true, strings.NewReader(item.body)); err != nil {
				t.Fatal(err)
			}
		}
		if err := r.BindAttempt(AssessedValueID, "00000000-0000-0000-0000-000000000001", "full_scan", 0); err != nil {
			t.Fatal(err)
		}
		v, err := r.Verify(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	complete := record(10)
	if len(complete.Manifest().Receipts) != 10 {
		t.Fatal("legacy request grammar requires ten receipts")
	}
	pub := &publisher{layer: union.AssessedValueLayer}
	if err := pub.prepareAssessmentRanges(ctx, complete); err != nil {
		t.Fatal(err)
	}
	if pub.count != 2002 || len(pub.assessments) != 2002 {
		t.Fatalf("lost legacy rows: count=%d rows=%d", pub.count, len(pub.assessments))
	}
	for i, row := range pub.assessments {
		want := "1"
		if i >= 1001 {
			want = "9223372036854775807"
		}
		if row.ObjectID != want {
			t.Fatalf("row %d identity=%s want=%s", i, row.ObjectID, want)
		}
	}
	if err := (&publisher{layer: union.AssessedValueLayer}).prepareAssessmentRanges(ctx, record(9)); err == nil {
		t.Fatal("incomplete legacy sequence accepted")
	}
}
