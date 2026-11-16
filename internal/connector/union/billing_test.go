package union

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/JustinMonty20/landman/internal/taxartifact"
	"github.com/JustinMonty20/landman/internal/taxsource"
)

const billingTestURL = "https://www.unioncountync.gov/home/showpublisheddocument/6045/639234409945470000"

const billingGolden = "property_key,source,year,TOTAL TAXES AND CHARGES,PROPERTY ADDRESS,Taxpayer Name 1,Taypayer Address 1,Taxpayer City,Taxpayer State,Taxpayer ZIP Code\n" +
	"09336350,RE,2026,\"1,359.48\",100 SAMPLE ROAD,TEST OWNER,PO BOX 1,SAMPLE,NC,28000\n" +
	"07105038A,RE,2026,.65,200 SAMPLE ROAD,TEST OWNER TWO,PO BOX 2,SAMPLE,NC,28001\n" +
	"407618,PP,2026,100.00,NO LAND,TEST BUSINESS,PO BOX 3,SAMPLE,NC,28002\n"

func TestBillingGolden(t *testing.T) {
	path := filepath.Join("testdata", "tax", "billing.csv")
	if *updateDelinquent {
		if err := os.WriteFile(path, []byte(billingGolden), 0600); err != nil {
			t.Fatal(err)
		}
	}
	csvBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(csvBytes) != billingGolden {
		t.Fatal("billing fixture differs from curated rows; use -update to regenerate")
	}
	list, store, raw := billingTestSource(t, "WKBILA-2026-07-22-1222.csv", string(csvBytes))
	var rows []BillRow
	meta, err := list.Read(context.Background(), func(row BillRow) error { rows = append(rows, row); return nil })
	if err != nil {
		t.Fatal(err)
	}
	want := []BillRow{
		{ParcelKey: "09336350", Year: 2026, TotalTaxes: 135948, PropertyAddress: "100 SAMPLE ROAD", TaxpayerName: "TEST OWNER", MailingLine: "PO BOX 1", MailingCity: "SAMPLE", MailingState: "NC", MailingPostalCode: "28000"},
		{ParcelKey: "07105038A", Year: 2026, TotalTaxes: 65, PropertyAddress: "200 SAMPLE ROAD", TaxpayerName: "TEST OWNER TWO", MailingLine: "PO BOX 2", MailingCity: "SAMPLE", MailingState: "NC", MailingPostalCode: "28001"},
	}
	if !reflect.DeepEqual(rows, want) {
		t.Fatalf("rows=%+v want=%+v", rows, want)
	}
	if meta.Name != billingSource || meta.URL != billingTestURL || meta.SHA256 != sha256.Sum256(raw) || meta.Artifact.SHA256() != meta.SHA256 || meta.Artifact.Size() != int64(len(raw)) || !meta.Artifact.Verified() || meta.Artifact.Mode() != "local" || meta.FetchedAt.IsZero() {
		t.Fatalf("billing evidence=%+v", meta)
	}
	if err := taxartifact.Verify(context.Background(), store, meta.Artifact); err != nil {
		t.Fatal(err)
	}
	body, err := store.Get(context.Background(), meta.Artifact)
	if err != nil {
		t.Fatal(err)
	}
	retained, readErr := io.ReadAll(body)
	if err := errors.Join(readErr, body.Close()); err != nil || !bytes.Equal(retained, raw) {
		t.Fatalf("retained ZIP changed: %v", err)
	}
	if pp, err := list.Skipped(context.Background(), meta); err != nil || pp != 1 {
		t.Fatalf("skipped PP=%d err=%v", pp, err)
	}
	var again []BillRow
	second, err := list.Read(context.Background(), func(row BillRow) error { again = append(again, row); return nil })
	if err != nil || !reflect.DeepEqual(again, rows) || second.Artifact.Key() != meta.Artifact.Key() {
		t.Fatalf("repeat rows=%+v meta=%+v err=%v", again, second, err)
	}
}

func TestBillingRejectsBadFileBeforeYield(t *testing.T) {
	tests := []struct{ name, body, want string }{
		{"missing key header", "source,year,TOTAL TAXES AND CHARGES,PROPERTY ADDRESS,Taxpayer Name 1,Taypayer Address 1\nRE,2026,1.00,ROAD,NAME,MAIL\n", "property_key"},
		{"bad money after valid row", strings.Replace(billingGolden, "07105038A,RE,2026,.65", "07105038A,RE,2026,invalid", 1), "TOTAL TAXES AND CHARGES"},
		{"bad year", strings.Replace(billingGolden, "09336350,RE,2026", "09336350,RE,bad", 1), "year"},
		{"short row", "property_key,source,year,TOTAL TAXES AND CHARGES,PROPERTY ADDRESS,Taxpayer Name 1,Taypayer Address 1\n09336350,RE,2026,1.00\n", "PROPERTY ADDRESS"},
		{"unknown source", strings.Replace(billingGolden, "09336350,RE,2026", "09336350,XX,2026", 1), "source"},
		{"empty key", strings.Replace(billingGolden, "09336350,RE,2026", ",RE,2026", 1), "property_key"},
		{"missing mailing header", strings.Replace(billingGolden, "Taypayer Address 1", "other header", 1), "Taypayer Address 1"},
		{"nonempty extra column", strings.Replace(billingGolden, "PO BOX 1,SAMPLE,NC,28000", "PO BOX 1,SAMPLE,NC,28000,EXTRA", 1), "fields"},
		{"empty extra column", strings.Replace(billingGolden, "PO BOX 1,SAMPLE,NC,28000", "PO BOX 1,SAMPLE,NC,28000,", 1), "fields"},
		{"shifted empty column", "property_key,source,year,TOTAL TAXES AND CHARGES,PROPERTY ADDRESS,Taxpayer Name 1,Taypayer Address 1,Taxpayer City,Taxpayer State,Taxpayer ZIP Code\n09336350,RE,2026,1.00,\"100 SAMPLE\",ROAD\",OWNER,MAIL,CITY,NC,\n", "quote in unquoted"},
		{"unterminated quote swallows next bill", "property_key,source,year,TOTAL TAXES AND CHARGES,PROPERTY ADDRESS,Taxpayer Name 1,Taypayer Address 1,Taxpayer City,Taxpayer State,Taxpayer ZIP Code\n09336350,RE,2026,1.00,ROAD,OWNER,MAIL,CITY,NC,\"28000\n07105038A,RE,2026,2.00,ROAD,OWNER,MAIL,CITY,NC,28001\n", "unterminated"},
		{"short optional columns", strings.Replace(billingGolden, "PO BOX 1,SAMPLE,NC,28000", "PO BOX 1", 1), "fields"},
		{"amount exceeds numeric column", strings.Replace(billingGolden, "\"1,359.48\"", "\"1,000,000,000,000.00\"", 1), "TOTAL TAXES AND CHARGES"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			list, _, _ := billingTestSource(t, "WKBILA-test.csv", tc.body)
			count := 0
			meta, err := list.Read(context.Background(), func(BillRow) error { count++; return nil })
			if err == nil || !strings.Contains(err.Error(), tc.want) || count != 0 || !meta.Artifact.Verified() {
				t.Fatalf("meta=%+v error=%v yielded=%d", meta, err, count)
			}
			if tc.name == "missing key header" && !strings.Contains(err.Error(), "year") {
				t.Fatalf("missing header list: %v", err)
			}
		})
	}
}

func TestBillingAcceptsLiteralQuote(t *testing.T) {
	body := strings.Replace(billingGolden, "100 SAMPLE ROAD", "\"100 SAMPLE \"ROAD\"", 1)
	list, _, _ := billingTestSource(t, "WKBILA-test.csv", body)
	var rows []BillRow
	if _, err := list.Read(context.Background(), func(row BillRow) error { rows = append(rows, row); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].PropertyAddress != "100 SAMPLE \"ROAD" {
		t.Fatalf("literal quote shifted columns: %+v", rows)
	}
}

func TestBillingOptionalAddressFields(t *testing.T) {
	body := "property_key,source,year,TOTAL TAXES AND CHARGES,PROPERTY ADDRESS,Taxpayer Name 1,Taypayer Address 1\n" +
		"09336350,RE,2026,.00,ROAD,NAME,MAIL\n"
	list, _, _ := billingTestSource(t, "WKBILA-test.csv", body)
	var rows []BillRow
	if _, err := list.Read(context.Background(), func(row BillRow) error { rows = append(rows, row); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].TotalTaxes != 0 || rows[0].MailingCity != "" || rows[0].MailingState != "" || rows[0].MailingPostalCode != "" {
		t.Fatalf("optional fields: %+v", rows)
	}
}

func TestBillingYieldError(t *testing.T) {
	list, _, _ := billingTestSource(t, "WKBILA-test.csv", billingGolden)
	failure := errors.New("stop billing rows")
	count := 0
	meta, err := list.Read(context.Background(), func(BillRow) error { count++; return failure })
	if !errors.Is(err, failure) || count != 1 || !meta.Artifact.Verified() {
		t.Fatalf("yield error=%v count=%d meta=%+v", err, count, meta)
	}
	if _, err := list.Skipped(context.Background(), taxsource.Meta{}); err == nil {
		t.Fatal("accepted unverified billing artifact")
	}
}

func TestBillingRejectsDuplicateEntries(t *testing.T) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, name := range []string{"WKBILA-first.csv", "WKBILA-second.csv"} {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(entry, billingGolden); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	path := filepath.Join(root, "multiple.zip")
	if err := os.WriteFile(path, buffer.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := taxartifact.NewLocal(filepath.Join(root, "private"))
	if err != nil {
		t.Fatal(err)
	}
	list, err := NewBillingFile(path, billingTestURL, store)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := list.Read(context.Background(), func(BillRow) error { t.Fatal("yielded ambiguous entry"); return nil }); err == nil || !strings.Contains(err.Error(), "multiple WKBILA") {
		t.Fatalf("duplicate entries: %v", err)
	}
}

func TestBillingRejectsInvalidZIP(t *testing.T) {
	root := t.TempDir()
	store, err := taxartifact.NewLocal(filepath.Join(root, "private"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "operator.zip")
	if err := os.WriteFile(path, []byte("not a ZIP"), 0600); err != nil {
		t.Fatal(err)
	}
	list, err := NewBillingFile(path, billingTestURL, store)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := list.Read(context.Background(), func(BillRow) error { t.Fatal("yielded invalid ZIP"); return nil })
	if err == nil || !strings.Contains(err.Error(), "invalid billing ZIP") || !meta.Artifact.Verified() {
		t.Fatalf("invalid ZIP meta=%+v err=%v", meta, err)
	}
	other, _, _ := billingTestSource(t, "other.csv", billingGolden)
	if _, err := other.Read(context.Background(), func(BillRow) error { t.Fatal("yielded unrelated entry"); return nil }); err == nil || !strings.Contains(err.Error(), "no WKBILA") {
		t.Fatalf("unrelated entry: %v", err)
	}
}

func TestBillingRejectsDamagedZIPBeforeYield(t *testing.T) {
	_, _, raw := billingTestSource(t, "WKBILA-test.csv", billingGolden)
	archive, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	offset, err := archive.File[0].DataOffset()
	if err != nil {
		t.Fatal(err)
	}
	damaged := append([]byte(nil), raw...)
	damaged[offset+int64(archive.File[0].CompressedSize64/2)] ^= 0x40
	root := t.TempDir()
	path := filepath.Join(root, "damaged.zip")
	if err := os.WriteFile(path, damaged, 0600); err != nil {
		t.Fatal(err)
	}
	store, err := taxartifact.NewLocal(filepath.Join(root, "private"))
	if err != nil {
		t.Fatal(err)
	}
	list, err := NewBillingFile(path, billingTestURL, store)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	meta, err := list.Read(context.Background(), func(BillRow) error { count++; return nil })
	if err == nil || count != 0 || !meta.Artifact.Verified() {
		t.Fatalf("damaged entry meta=%+v yielded=%d err=%v", meta, count, err)
	}
}

func TestBillingCancelDuringRead(t *testing.T) {
	list, _, _ := billingTestSource(t, "WKBILA-test.csv", billingGolden)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	count := 0
	meta, err := list.Read(ctx, func(BillRow) error { count++; cancel(); return nil })
	if !errors.Is(err, context.Canceled) || count != 1 || !meta.Artifact.Verified() {
		t.Fatalf("canceled billing meta=%+v yielded=%d err=%v", meta, count, err)
	}
}

func TestBillingRejectsZIPGrowthBeforeRetention(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "growing.zip")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := errors.Join(file.Truncate(billingLimit), file.Close()); err != nil {
		t.Fatal(err)
	}
	store, err := taxartifact.NewLocal(filepath.Join(root, "private"))
	if err != nil {
		t.Fatal(err)
	}
	list, err := NewBillingFile(path, billingTestURL, growingBillingStore{Store: store, path: path})
	if err != nil {
		t.Fatal(err)
	}
	meta, err := list.Read(context.Background(), func(BillRow) error { t.Fatal("yielded growing ZIP"); return nil })
	if err == nil || !strings.Contains(err.Error(), "compressed cap") || meta.Artifact.Verified() {
		t.Fatalf("growing ZIP meta=%+v err=%v", meta, err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "private", "tax"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("retained growing artifact: %v", entries)
	}
}

type growingBillingStore struct {
	taxartifact.Store
	path string
}

func (s growingBillingStore) Put(ctx context.Context, source string, body io.Reader) (taxartifact.Artifact, error) {
	file, err := os.OpenFile(s.path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return taxartifact.Artifact{}, err
	}
	_, writeErr := file.Write([]byte{0})
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return taxartifact.Artifact{}, err
	}
	return s.Store.Put(ctx, source, body)
}

func TestBillingRejectsOversizeZIPBeforeRetention(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "operator.zip")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := errors.Join(file.Truncate(billingLimit+1), file.Close()); err != nil {
		t.Fatal(err)
	}
	store, err := taxartifact.NewLocal(filepath.Join(root, "private"))
	if err != nil {
		t.Fatal(err)
	}
	list, err := NewBillingFile(path, billingTestURL, store)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := list.Read(context.Background(), func(BillRow) error { t.Fatal("yielded oversized ZIP"); return nil })
	if err == nil || !strings.Contains(err.Error(), "compressed cap") || meta.Artifact.Verified() {
		t.Fatalf("oversized ZIP meta=%+v err=%v", meta, err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "private", "tax"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("retained oversized artifact: %v", entries)
	}
}

func TestBillingRejectsOversizeEntry(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "large.zip")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	entry, err := writer.Create("WKBILA-large.csv")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.CopyN(entry, zeroBillingReader{}, billingLimit+1); err != nil {
		t.Fatal(err)
	}
	if err := errors.Join(writer.Close(), file.Close()); err != nil {
		t.Fatal(err)
	}
	store, err := taxartifact.NewLocal(filepath.Join(root, "private"))
	if err != nil {
		t.Fatal(err)
	}
	list, err := NewBillingFile(path, billingTestURL, store)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := list.Read(context.Background(), func(BillRow) error { t.Fatal("yielded oversize entry"); return nil })
	if err == nil || !strings.Contains(err.Error(), "uncompressed cap") || !meta.Artifact.Verified() {
		t.Fatalf("oversize entry meta=%+v err=%v", meta, err)
	}
}

type zeroBillingReader struct{}

func (zeroBillingReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

func TestBillingRequiresLocalInput(t *testing.T) {
	store, err := taxartifact.NewLocal(filepath.Join(t.TempDir(), "private"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path, sourceURL string
		store           taxartifact.Store
	}{
		{"", billingTestURL, store},
		{"operator.zip", "", store},
		{"operator.zip", "http://example.invalid/zip", store},
		{"operator.zip", billingTestURL, nil},
	} {
		if _, err := NewBillingFile(tc.path, tc.sourceURL, tc.store); err == nil {
			t.Fatalf("accepted input path=%q URL=%q", tc.path, tc.sourceURL)
		}
	}
	list, err := NewBillingFile(filepath.Join(t.TempDir(), "missing.zip"), billingTestURL, store)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := list.Read(context.Background(), func(BillRow) error { t.Fatal("yielded without file"); return nil })
	if err == nil || meta.Artifact.Verified() {
		t.Fatalf("missing ZIP meta=%+v err=%v", meta, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	meta, err = list.Read(ctx, func(BillRow) error { t.Fatal("yielded after cancel"); return nil })
	if !errors.Is(err, context.Canceled) || meta.Artifact.Verified() {
		t.Fatalf("cancel meta=%+v err=%v", meta, err)
	}
}

func billingTestSource(t *testing.T, entryName, contents string) (*BillingFile, taxartifact.Store, []byte) {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	entry, err := writer.Create(entryName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(entry, contents); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	path := filepath.Join(root, "operator.zip")
	if err := os.WriteFile(path, buffer.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := taxartifact.NewLocal(filepath.Join(root, "private"))
	if err != nil {
		t.Fatal(err)
	}
	list, err := NewBillingFile(path, billingTestURL, store)
	if err != nil {
		t.Fatal(err)
	}
	return list, store, buffer.Bytes()
}

// TestBillingMeasuredFile checks the saved ZIP without a county request.
func TestBillingMeasuredFile(t *testing.T) {
	path := os.Getenv("TAX_BILLING_TEST_FILE")
	if path == "" {
		t.Skip("set TAX_BILLING_TEST_FILE to the operator-supplied ZIP")
	}
	store, err := taxartifact.NewLocal(filepath.Join(t.TempDir(), "private"))
	if err != nil {
		t.Fatal(err)
	}
	list, err := NewBillingFile(path, billingTestURL, store)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	var cents int64
	meta, err := list.Read(context.Background(), func(row BillRow) error {
		if row.Year != 2026 {
			t.Fatalf("unexpected billing year %d", row.Year)
		}
		count++
		cents += row.TotalTaxes
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	pp, err := list.Skipped(context.Background(), meta)
	if err != nil || count != 116421 || pp != 21984 || meta.Artifact.Size() != 12642916 || meta.SHA256 != [32]byte{0xdd, 0xc8, 0x35, 0xbb, 0xa1, 0x3e, 0xf5, 0xd0, 0x6a, 0x63, 0xb4, 0xb7, 0x09, 0x7a, 0xa3, 0x15, 0xf2, 0x41, 0x94, 0x75, 0xbf, 0xd5, 0x98, 0xd5, 0x1e, 0x4f, 0xaa, 0xe1, 0x24, 0x50, 0x26, 0x6a} {
		t.Fatalf("billing file RE=%d PP=%d size=%d cents=%d meta=%+v err=%v", count, pp, meta.Artifact.Size(), cents, meta, err)
	}
	if !meta.Artifact.Verified() {
		t.Fatal("operator ZIP was not retained before parsing")
	}
	t.Logf("operator ZIP: RE=%d PP=%d billed cents=%d", count, pp, cents)
}

func FuzzBillingMoney(f *testing.F) {
	for _, seed := range []string{".00", ".65", "$1,359.48", "-1.00", "1000000000000.00", "bad"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		cents, err := parseBillingMoney(raw)
		if err == nil && (cents < 0 || cents > 99999999999999) {
			t.Fatalf("invalid billing amount %q: %d", raw, cents)
		}
	})
}
