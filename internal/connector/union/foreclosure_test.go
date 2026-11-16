package union

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/JustinMonty20/landman/internal/taxartifact"
)

func foreclosureFixture(t *testing.T) []byte {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", "tax", "foreclosure.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func foreclosureTestList(t *testing.T, body []byte) (*ForeclosureList, taxartifact.Store) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "operator.pdf")
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	store, err := taxartifact.NewLocal(filepath.Join(t.TempDir(), "private"))
	if err != nil {
		t.Fatal(err)
	}
	list, err := NewForeclosureList(path, store)
	if err != nil {
		t.Fatal(err)
	}
	return list, store
}

func TestForeclosureGolden(t *testing.T) {
	body := foreclosureFixture(t)
	list, store := foreclosureTestList(t, body)
	var cases []ForeclosureCase
	meta, err := list.Read(context.Background(), func(sale ForeclosureCase) error { cases = append(cases, sale); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if meta.URL != ForeclosureURL || meta.Name != foreclosureSource || meta.SHA256 != sha256.Sum256(body) || meta.Artifact.Size() != int64(len(body)) || !meta.Artifact.Verified() {
		t.Fatalf("invalid PDF evidence: %+v", meta)
	}
	if err := taxartifact.Verify(context.Background(), store, meta.Artifact); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 3 {
		t.Fatalf("cases=%d: %+v", len(cases), cases)
	}
	first := cases[0]
	if first.CaseNumber != "26M000471-890" || first.SaleOn == nil || first.SaleOn.Format("2006-01-02") != "2026-11-04" || first.OpeningBid == nil || *first.OpeningBid != 380000 || first.CurrentBid == nil || *first.CurrentBid != 500000 || first.UpsetBid == nil || *first.UpsetBid != 575000 {
		t.Fatalf("sale: %+v", first)
	}
	var keys []string
	for _, pin := range first.PINs {
		keys = append(keys, pin.ParcelKey)
		if pin.SitusLine != "0 Drexel Drive" || pin.LegalDescription == "" || pin.TaxValue == nil || *pin.TaxValue != 1000000 {
			t.Fatalf("PIN: %+v", pin)
		}
	}
	if !reflect.DeepEqual(keys, []string{"07129160", "07129161", "07129162"}) {
		t.Fatalf("three PINs on one case: %v", keys)
	}
	if cases[1].CaseNumber != "" || cases[1].SaleOn != nil || cases[1].OpeningBid != nil || cases[1].PINs[0].ParcelKey != "09231101B" || cases[2].CaseNumber != "" || cases[2].PINs[0].ParcelKey != "09125043" || cases[2].CurrentBid != nil {
		t.Fatalf("TBD cases: %+v", cases[1:])
	}
	if skipped, err := list.Skipped(context.Background(), meta); err != nil || skipped != 1 {
		t.Fatalf("skipped blocks=%d err=%v", skipped, err)
	}
}

func TestForeclosureRejectsInvalidFileBeforeYield(t *testing.T) {
	fixture := foreclosureFixture(t)
	replace := func(from, to string) []byte {
		t.Helper()
		if len(from) != len(to) {
			t.Fatal("replacement must preserve PDF stream length")
		}
		if !bytes.Contains(fixture, []byte(from)) {
			t.Fatalf("fixture lacks %q", from)
		}
		return bytes.ReplaceAll(fixture, []byte(from), []byte(to))
	}
	for _, test := range []struct {
		name      string
		body      []byte
		errorText string
	}{
		{"missing label", replace("Parcel Identification Number", strings.Repeat("X", len("Parcel Identification Number"))), "Parcel Identification Number"},
		{"bad amount", replace("$5,000.00", "$bad-data"), "dollar amount"},
		{"not a PDF", []byte("not pdf"), "not a PDF"},
		{"corrupt PDF", []byte("%PDF-1.4\nnot an object\n"), "extract foreclosure PDF"},
	} {
		t.Run(test.name, func(t *testing.T) {
			list, _ := foreclosureTestList(t, test.body)
			yielded := 0
			_, err := list.Read(context.Background(), func(ForeclosureCase) error { yielded++; return nil })
			if err == nil || !strings.Contains(err.Error(), test.errorText) || yielded != 0 {
				t.Fatalf("yielded=%d err=%v", yielded, err)
			}
		})
	}
}

func TestForeclosureKeepsSpaceInParcelKey(t *testing.T) {
	body := foreclosureFixture(t)
	from := []byte(`Parcel Identification Number \(PIN\): 09-125-043`)
	to := []byte(`Parcel Identification Number \(X\):02-176-011 60`)
	if len(from) != len(to) || !bytes.Contains(body, from) {
		t.Fatal("PDF stream replacement changed length or fixture lacks source")
	}
	list, _ := foreclosureTestList(t, bytes.Replace(body, from, to, 1))
	var keys []string
	_, err := list.Read(context.Background(), func(sale ForeclosureCase) error {
		for _, pin := range sale.PINs {
			keys = append(keys, pin.ParcelKey)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if keys[len(keys)-1] != "02176011 60" {
		t.Fatalf("space lost in PIN: %v", keys)
	}
}

func TestForeclosureCanceledBeforeOpen(t *testing.T) {
	list, _ := foreclosureTestList(t, foreclosureFixture(t))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := list.Read(ctx, func(ForeclosureCase) error { t.Fatal("yield after cancel"); return nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel=%v", err)
	}
}

func TestForeclosureLocalFile(t *testing.T) {
	path := os.Getenv("FORECLOSURE_LOCAL_FILE")
	if path == "" {
		t.Skip("operator-supplied local file not configured")
	}
	store, err := taxartifact.NewLocal(filepath.Join(t.TempDir(), "private"))
	if err != nil {
		t.Fatal(err)
	}
	list, err := NewForeclosureList(path, store)
	if err != nil {
		t.Fatal(err)
	}
	var cases []ForeclosureCase
	meta, err := list.Read(context.Background(), func(sale ForeclosureCase) error { cases = append(cases, sale); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("no sales from local file")
	}
	count, err := list.Skipped(context.Background(), meta)
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, sale := range cases {
		for _, pin := range sale.PINs {
			keys = append(keys, pin.ParcelKey)
		}
	}
	if hex.EncodeToString(meta.SHA256[:]) == "b8406ff8dcdd927bbb681c21b5a97be8ca1c957ac665969e7d054614cd537a68" {
		if len(cases) != 6 || len(keys) != 9 || cases[5].PINs[0].TaxValue == nil || *cases[5].PINs[0].TaxValue != 5380000 || cases[5].PINs[1].TaxValue == nil || *cases[5].PINs[1].TaxValue != 5440000 {
			t.Fatalf("measured PDF parsed incorrectly: cases=%d keys=%v final PINs=%+v", len(cases), keys, cases[5].PINs)
		}
	}
	t.Logf("operator file: cases=%d skipped blocks=%d bytes=%d hash=%x keys=%v", len(cases), count, meta.Artifact.Size(), meta.SHA256, keys)
}
