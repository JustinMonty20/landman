package union

import (
	"context"
	"crypto/sha256"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/taxartifact"
)

var updateDelinquent = flag.Bool("update", false, "rewrite the curated delinquent golden CSV")

const delinquentGolden = "Property Display Name,Property Key,Year,Primary Source,Balance Due,,,\n" +
	"\"EVANS, WILLIE P HEIRS\",09125043,2022,RE,\"$14,355.02\",,,\n" +
	"JASTCO LLC,07105038A,2025,RE,\"$9,010.51\",,,\n" +
	"\"CUTHBERTSON, ARCHIE HEIRS\",07027002J80,2025,RE,$356.28,,,\n" +
	"CHRIST BIBLE TEACHING CENTER,02176011 60,2018,RE,$10.00,,,\n" +
	"BENTWOOD FARMS LLC,407618,2019,PP,\"$36,800.63\",,,\n" +
	"\"JET IT, LLC\",21,2022,RR,$388.74,,,\n"

func TestDelinquentGolden(t *testing.T) {
	path := filepath.Join("testdata", "tax", "delinquent.csv")
	if *updateDelinquent {
		if err := os.WriteFile(path, []byte(delinquentGolden), 0600); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != delinquentGolden {
		t.Fatal("golden file differs from curated county rows; use -update to regenerate")
	}
	list, store := delinquentTestSource(t, string(data))
	var rows []DelinquentRow
	meta, err := list.Read(context.Background(), func(row DelinquentRow) error { rows = append(rows, row); return nil })
	if err != nil {
		t.Fatal(err)
	}
	want := []DelinquentRow{
		{ParcelKey: "09125043", Year: 2022, Balance: 1435502, PrimarySource: "RE"},
		{ParcelKey: "07105038A", Year: 2025, Balance: 901051, PrimarySource: "RE"},
		{ParcelKey: "07027002J80", Year: 2025, Balance: 35628, PrimarySource: "RE"},
		{ParcelKey: "02176011 60", Year: 2018, Balance: 1000, PrimarySource: "RE"},
	}
	if !reflect.DeepEqual(rows, want) {
		t.Fatalf("rows: %+v, want %+v", rows, want)
	}
	if meta.Name != delinquentSource || meta.URL != DelinquentURL || meta.SHA256 != sha256.Sum256(data) || meta.Artifact.SHA256() != meta.SHA256 || !meta.Artifact.Verified() || meta.Artifact.Mode() != "local" || meta.FetchedAt.IsZero() {
		t.Fatalf("file evidence: %+v", meta)
	}
	if err := taxartifact.Verify(context.Background(), store, meta.Artifact); err != nil {
		t.Fatal(err)
	}
	body, err := store.Get(context.Background(), meta.Artifact)
	if err != nil {
		t.Fatal(err)
	}
	retained, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	if err := body.Close(); err != nil {
		t.Fatal(err)
	}
	if string(retained) != string(data) {
		t.Fatal("stored bytes differ from county bytes")
	}
	pp, rr, err := list.Skipped(context.Background(), meta)
	if err != nil || pp != 1 || rr != 1 {
		t.Fatalf("skipped PP=%d RR=%d error=%v", pp, rr, err)
	}
	var again []DelinquentRow
	second, err := list.Read(context.Background(), func(row DelinquentRow) error { again = append(again, row); return nil })
	if err != nil || !reflect.DeepEqual(again, rows) || second.Artifact.Key() != meta.Artifact.Key() {
		t.Fatalf("repeat rows=%v meta=%+v err=%v", again, second, err)
	}
}

func TestDelinquentRejectsBadFileBeforeYield(t *testing.T) {
	tests := []struct{ name, body, want string }{
		{"missing key header", "Year,Primary Source,Balance Due\n2025,RE,$2.00\n", "Property Key"},
		{"bad money after valid row", "Property Key,Year,Primary Source,Balance Due\n09125043,2025,RE,$1.00\n07105038A,2025,RE,unknown\n", "Balance Due"},
		{"bad year", "Property Key,Year,Primary Source,Balance Due\n09125043,no,RE,$1.00\n", "Year"},
		{"short row", "Property Key,Year,Primary Source,Balance Due\n09125043,2025,RE\n", "Balance Due"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			list, _ := delinquentTestSource(t, test.body)
			yielded := 0
			meta, err := list.Read(context.Background(), func(DelinquentRow) error { yielded++; return nil })
			if err == nil || !strings.Contains(err.Error(), test.want) || yielded != 0 || !meta.Artifact.Verified() {
				t.Fatalf("meta=%+v error=%v yielded=%d", meta, err, yielded)
			}
			if test.name == "missing key header" && !strings.Contains(err.Error(), "Year") {
				t.Fatalf("missing header list: %v", err)
			}
		})
	}
}

func TestDelinquentRequiresLocalFile(t *testing.T) {
	store, err := taxartifact.NewLocal(filepath.Join(t.TempDir(), "private"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewDelinquentList("", store); err == nil {
		t.Fatal("accepted missing input path")
	}
	list, err := NewDelinquentList(filepath.Join(t.TempDir(), "missing.csv"), store)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := list.Read(context.Background(), func(DelinquentRow) error { t.Fatal("yielded without file"); return nil })
	if err == nil || meta.Artifact.Verified() {
		t.Fatalf("missing file: meta=%+v err=%v", meta, err)
	}
}

func TestDelinquentCancelBeforeFile(t *testing.T) {
	list, _ := delinquentTestSource(t, delinquentGolden)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	yielded := 0
	meta, err := list.Read(ctx, func(DelinquentRow) error { yielded++; return nil })
	if !errors.Is(err, context.Canceled) || yielded != 0 || meta.Artifact.Verified() {
		t.Fatalf("cancel error=%v yielded=%d meta=%+v", err, yielded, meta)
	}
}

func delinquentTestSource(t *testing.T, body string) (*DelinquentList, taxartifact.Store) {
	t.Helper()
	directory := t.TempDir()
	path := filepath.Join(directory, "operator.csv")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := taxartifact.NewLocal(filepath.Join(directory, "private"))
	if err != nil {
		t.Fatal(err)
	}
	list, err := NewDelinquentList(path, store)
	if err != nil {
		t.Fatal(err)
	}
	return list, store
}

// TestDelinquentMeasuredFile checks the saved county CSV without making a county request.
func TestDelinquentMeasuredFile(t *testing.T) {
	path := os.Getenv("TAX_DELINQUENT_TEST_FILE")
	if path == "" {
		t.Skip("set TAX_DELINQUENT_TEST_FILE to the saved county CSV")
	}
	body, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	var re int
	var cents int64
	start := time.Now()
	counts, err := parseDelinquent(context.Background(), body, func(row DelinquentRow) error {
		re++
		cents += row.Balance
		return nil
	})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("offline parse time=%s RE=%d PP=%d RR=%d RE balance cents=%d", elapsed, re, counts.pp, counts.rr, cents)
	if re != 926 || counts.pp != 5013 || counts.rr != 3 || cents != 23275526 || elapsed >= time.Second {
		t.Fatalf("offline file failed count, sum, or 1-second parse gate")
	}
	store, err := taxartifact.NewLocal(filepath.Join(t.TempDir(), "private"))
	if err != nil {
		t.Fatal(err)
	}
	list, err := NewDelinquentList(path, store)
	if err != nil {
		t.Fatal(err)
	}
	var imported int
	meta, err := list.Read(context.Background(), func(DelinquentRow) error { imported++; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if imported != re || !meta.Artifact.Verified() || meta.Artifact.Size() != 300375 || meta.URL != DelinquentURL {
		t.Fatalf("operator file import: rows=%d artifact=%+v", imported, meta)
	}
}

func FuzzDelinquentMoney(f *testing.F) {
	for _, seed := range []string{"$14,355.02", "$0.00", "-1.00", "$92233720368547759.99", "bad"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		value, err := parseDelinquentBalance(raw)
		if err == nil && value < 0 {
			t.Fatalf("accepted negative balance %q: %d", raw, value)
		}
	})
}

func FuzzParcelKey(f *testing.F) {
	for _, seed := range []string{"09125043", "07105038A", "02176011 60", " 07-10.5038A "} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, key string) {
		got := normalizeDelinquentKey(key)
		if strings.ContainsAny(got, "-.") || got != strings.TrimSpace(got) || normalizeDelinquentKey(got) != got {
			t.Fatalf("bad normalized key %q: %q", key, got)
		}
	})
}
