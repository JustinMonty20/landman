//go:build postgres

package taxstore_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"

	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/JustinMonty20/landman/internal/taxartifact"
	"github.com/JustinMonty20/landman/internal/taxsource"
	"github.com/JustinMonty20/landman/internal/taxstore"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSaveForeclosure(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL must name a disposable migrated database")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	latest := taxstore.NewForeclosures(pool)
	if _, ok, err := latest.Latest(ctx); err != nil || ok {
		t.Fatalf("empty latest: ok=%v err=%v", ok, err)
	}
	store, err := taxartifact.NewLocal(filepath.Join(t.TempDir(), "private"))
	if err != nil {
		t.Fatal(err)
	}
	list, err := union.NewForeclosureList(filepath.Join("..", "connector", "union", "testdata", "tax", "foreclosure.pdf"), store)
	if err != nil {
		t.Fatal(err)
	}
	var sales []union.ForeclosureCase
	meta, err := list.Read(ctx, func(sale union.ForeclosureCase) error { sales = append(sales, sale); return nil })
	if err != nil {
		t.Fatal(err)
	}
	skipped, err := list.Skipped(ctx, meta)
	if err != nil || skipped != 1 {
		t.Fatalf("skipped=%d err=%v", skipped, err)
	}
	var ids []string
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	for i, pin := range sales[0].PINs {
		var id string
		if err := tx.QueryRow(ctx, `INSERT INTO parcels (county_id,county_code,source_name,source_parcel_id,likely_vacant)
VALUES ('union','179','union_county_gis',$1,$2) RETURNING id::text`, pin.ParcelKey, i != 1).Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	var secondID string
	if err := tx.QueryRow(ctx, `INSERT INTO parcels (county_id,county_code,source_name,source_parcel_id,likely_vacant)
VALUES ('union','179','union_county_gis',$1,false) RETURNING id::text`, sales[2].PINs[0].ParcelKey).Scan(&secondID); err != nil {
		t.Fatal(err)
	}
	ids = append(ids, secondID)
	var before int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM parcels`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	first, err := taxstore.SaveForeclosure(ctx, tx, sales, meta, skipped)
	if err != nil || first.SavedCases != 3 || first.SavedJoins != 4 || first.SkippedBlocks != 1 || first.Unmatched != 1 {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	var numberedID, nullID, firstNumber, bid string
	if err := tx.QueryRow(ctx, `SELECT id::text,opening_bid::text FROM tax_foreclosure_cases WHERE case_number=$1`, sales[0].CaseNumber).Scan(&numberedID, &bid); err != nil || bid != "3800.00" {
		t.Fatalf("case id=%s bid=%s err=%v", numberedID, bid, err)
	}
	if err := tx.QueryRow(ctx, `SELECT id::text FROM tax_foreclosure_cases WHERE case_number IS NULL AND owner_name=$1`, sales[2].OwnerName).Scan(&nullID); err != nil {
		t.Fatal(err)
	}
	if repeat, err := taxstore.SaveForeclosure(ctx, tx, sales, meta, skipped); err != nil || repeat.SavedCases != 3 || repeat.SavedJoins != 4 {
		t.Fatalf("repeat=%+v err=%v", repeat, err)
	}
	if err := tx.QueryRow(ctx, `SELECT id::text FROM tax_foreclosure_cases WHERE case_number=$1`, sales[0].CaseNumber).Scan(&firstNumber); err != nil || firstNumber != numberedID {
		t.Fatalf("repeat id=%s/%s err=%v", firstNumber, numberedID, err)
	}
	var nullCases, joins, after, owed int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM tax_foreclosure_cases WHERE case_number IS NULL`).Scan(&nullCases); err != nil {
		t.Fatal(err)
	}
	if nullCases != 2 {
		t.Fatalf("TBD cases=%d", nullCases)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM tax_foreclosure_parcels WHERE case_id=$1`, numberedID).Scan(&joins); err != nil || joins != 3 {
		t.Fatalf("joins=%d err=%v", joins, err)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM tax_owed WHERE parcel_id=$1`, ids[0]).Scan(&owed); err != nil || owed != 0 {
		t.Fatalf("foreclosure changed tax_owed=%d err=%v", owed, err)
	}
	// Legal text follows sorted PIN order, not its own lexical order.
	unnumbered := sales[0]
	unnumbered.CaseNumber = ""
	unnumbered.PINs = append([]union.ForeclosurePIN(nil), sales[0].PINs[:2]...)
	unnumbered.PINs[0].LegalDescription = "Z legal"
	unnumbered.PINs[1].LegalDescription = "A legal"
	if _, err := taxstore.SaveForeclosure(ctx, tx, []union.ForeclosureCase{unnumbered}, meta, 0); err != nil {
		t.Fatal(err)
	}
	pinHash := sha256.Sum256([]byte("07129160\n07129161"))
	legalHash := sha256.Sum256([]byte("Z legal\nA legal"))
	var legalID string
	var storedPINHash, storedLegalHash []byte
	if err := tx.QueryRow(ctx, `SELECT id::text,parcel_set_sha256,legal_sha256 FROM tax_foreclosure_cases
WHERE case_number IS NULL AND parcel_set_sha256=$1 AND legal_sha256=$2`, pinHash[:], legalHash[:]).Scan(&legalID, &storedPINHash, &storedLegalHash); err != nil || !bytes.Equal(storedPINHash, pinHash[:]) || !bytes.Equal(storedLegalHash, legalHash[:]) {
		t.Fatalf("PIN order hash: id=%s err=%v", legalID, err)
	}
	unnumbered.PINs[0], unnumbered.PINs[1] = unnumbered.PINs[1], unnumbered.PINs[0]
	if _, err := taxstore.SaveForeclosure(ctx, tx, []union.ForeclosureCase{unnumbered}, meta, 0); err != nil {
		t.Fatal(err)
	}
	var sameOrderID string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM tax_foreclosure_cases WHERE case_number IS NULL AND parcel_set_sha256=$1 AND legal_sha256=$2`, pinHash[:], legalHash[:]).Scan(&sameOrderID); err != nil || sameOrderID != legalID {
		t.Fatalf("PIN order changed identity: %s/%s err=%v", sameOrderID, legalID, err)
	}
	unnumbered.PINs[0].LegalDescription, unnumbered.PINs[1].LegalDescription = unnumbered.PINs[1].LegalDescription, unnumbered.PINs[0].LegalDescription
	if _, err := taxstore.SaveForeclosure(ctx, tx, []union.ForeclosureCase{unnumbered}, meta, 0); err != nil {
		t.Fatal(err)
	}
	var swappedID string
	swappedHash := sha256.Sum256([]byte("A legal\nZ legal"))
	if err := tx.QueryRow(ctx, `SELECT id::text FROM tax_foreclosure_cases WHERE case_number IS NULL AND parcel_set_sha256=$1 AND legal_sha256=$2`, pinHash[:], swappedHash[:]).Scan(&swappedID); err != nil || swappedID == legalID {
		t.Fatalf("swapped legal identity: %s/%s err=%v", swappedID, legalID, err)
	}
	// A later numbered case does not absorb the earlier null-number identity.
	numbered := sales[2]
	numbered.CaseNumber = "26M000998-890"
	if result, err := taxstore.SaveForeclosure(ctx, tx, []union.ForeclosureCase{numbered}, meta, skipped); err != nil || result.SavedCases != 1 || result.SavedJoins != 1 || result.Unmatched != 0 {
		t.Fatalf("new number=%+v err=%v", result, err)
	}
	var sameNull int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM tax_foreclosure_cases WHERE id=$1 AND case_number IS NULL`, nullID).Scan(&sameNull); err != nil || sameNull != 1 {
		t.Fatalf("old TBD id=%d err=%v", sameNull, err)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM parcels`).Scan(&after); err != nil || before != after {
		t.Fatalf("parcels before=%d after=%d err=%v", before, after, err)
	}
	if _, ok, err := latest.Latest(ctx); err != nil || ok {
		t.Fatalf("uncommitted latest: ok=%v err=%v", ok, err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := latest.Latest(ctx); err != nil || !ok {
		t.Fatalf("latest: ok=%v err=%v", ok, err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM parcels WHERE id::text = ANY($1)`, ids)
		_, _ = pool.Exec(ctx, `DELETE FROM tax_foreclosure_cases WHERE county_id='union' AND source_name='union_foreclosure' AND file_sha256=$1`, meta.SHA256[:])
		_, _ = pool.Exec(ctx, `DELETE FROM tax_file_imports WHERE county_id='union' AND source_name='union_foreclosure' AND source_url=$1`, meta.URL)
	})
}

func TestSaveForeclosureReplacesNumberedCasePINs(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL must name a disposable migrated database")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	store, err := taxartifact.NewLocal(filepath.Join(t.TempDir(), "private"))
	if err != nil {
		t.Fatal(err)
	}
	readCase := func(path string) (union.ForeclosureCase, taxsource.Meta) {
		t.Helper()
		list, err := union.NewForeclosureList(path, store)
		if err != nil {
			t.Fatal(err)
		}
		var cases []union.ForeclosureCase
		meta, err := list.Read(ctx, func(sale union.ForeclosureCase) error {
			cases = append(cases, sale)
			return nil
		})
		if err != nil || len(cases) == 0 {
			t.Fatalf("read foreclosure case: count=%d err=%v", len(cases), err)
		}
		return cases[0], meta
	}
	path := filepath.Join("..", "connector", "union", "testdata", "tax", "foreclosure.pdf")
	original, originalMeta := readCase(path)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	changed := bytes.Replace(body, []byte("07-129-160"), []byte("07-129-163"), 1)
	if bytes.Equal(body, changed) {
		t.Fatal("fixture PIN did not change")
	}
	changedPath := filepath.Join(t.TempDir(), "sales.pdf")
	if err := os.WriteFile(changedPath, changed, 0600); err != nil {
		t.Fatal(err)
	}
	revised, revisedMeta := readCase(changedPath)
	if original.CaseNumber == "" || revised.CaseNumber != original.CaseNumber || originalMeta.SHA256 == revisedMeta.SHA256 {
		t.Fatal("fixture must change one PIN of the same numbered case")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	for _, key := range []string{"07129160", "07129161", "07129162", "07129163"} {
		if _, err := tx.Exec(ctx, `INSERT INTO parcels (county_id,county_code,source_name,source_parcel_id,likely_vacant)
VALUES ('union','179','union_county_gis',$1,true)`, key); err != nil {
			t.Fatal(err)
		}
	}
	links := func() (string, map[string]string) {
		t.Helper()
		rows, err := tx.Query(ctx, `SELECT c.id::text,p.source_parcel_id,j.id::text FROM tax_foreclosure_cases c
JOIN tax_foreclosure_parcels j ON j.case_id=c.id JOIN parcels p ON p.id=j.parcel_id
WHERE c.county_id='union' AND c.source_name='union_foreclosure' AND c.case_number=$1`, original.CaseNumber)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var caseID string
		found := make(map[string]string)
		for rows.Next() {
			var key, linkID string
			if err := rows.Scan(&caseID, &key, &linkID); err != nil {
				t.Fatal(err)
			}
			found[key] = linkID
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		return caseID, found
	}
	if _, err := taxstore.SaveForeclosure(ctx, tx, []union.ForeclosureCase{original}, originalMeta, 0); err != nil {
		t.Fatal(err)
	}
	firstCase, first := links()
	if len(first) != 3 {
		t.Fatalf("first numbered-case links=%v", first)
	}
	if result, err := taxstore.SaveForeclosure(ctx, tx, []union.ForeclosureCase{revised}, revisedMeta, 0); err != nil || result.SavedJoins != 3 {
		t.Fatalf("revised case=%+v err=%v", result, err)
	}
	secondCase, second := links()
	if firstCase != secondCase || len(second) != 3 || second["07129160"] != "" || second["07129161"] != first["07129161"] || second["07129162"] != first["07129162"] || second["07129163"] == "" {
		t.Fatalf("numbered-case links after PIN change: case=%s/%s first=%v second=%v", firstCase, secondCase, first, second)
	}
	if _, err := taxstore.SaveForeclosure(ctx, tx, []union.ForeclosureCase{revised}, revisedMeta, 0); err != nil {
		t.Fatal(err)
	}
	_, repeat := links()
	if repeat["07129163"] != second["07129163"] || len(repeat) != 3 {
		t.Fatalf("retry changed numbered-case links: %v", repeat)
	}
	firstBlock, secondBlock := revised, revised
	firstBlock.PINs = revised.PINs[:1]
	secondBlock.PINs = revised.PINs[1:]
	if _, err := taxstore.SaveForeclosure(ctx, tx, []union.ForeclosureCase{firstBlock, secondBlock}, revisedMeta, 0); err != nil {
		t.Fatal(err)
	}
	_, split := links()
	if len(split) != 3 || split["07129161"] != second["07129161"] || split["07129162"] != second["07129162"] || split["07129163"] != second["07129163"] {
		t.Fatalf("repeated numbered-case blocks lost PINs: %v", split)
	}
	unmatchedPDF := changed
	for old, replacement := range map[string]string{"07-129-163": "07-129-180", "07-129-161": "07-129-181", "129-162": "129-182"} {
		if !bytes.Contains(unmatchedPDF, []byte(old)) {
			t.Fatalf("fixture PIN %s not found", old)
		}
		unmatchedPDF = bytes.Replace(unmatchedPDF, []byte(old), []byte(replacement), 1)
	}
	unmatchedPath := filepath.Join(t.TempDir(), "unmatched.pdf")
	if err := os.WriteFile(unmatchedPath, unmatchedPDF, 0600); err != nil {
		t.Fatal(err)
	}
	unmatched, unmatchedMeta := readCase(unmatchedPath)
	if unmatched.CaseNumber != original.CaseNumber || len(unmatched.PINs) != 3 {
		t.Fatalf("unmatched file must contain three new PINs for the same case: %+v", unmatched)
	}
	result, err := taxstore.SaveForeclosure(ctx, tx, []union.ForeclosureCase{unmatched}, unmatchedMeta, 0)
	if err != nil || result.SavedJoins != 0 || result.Unmatched != 3 {
		t.Fatalf("unmatched case=%+v err=%v", result, err)
	}
	_, empty := links()
	if len(empty) != 0 {
		t.Fatalf("all-unmatched revision retained obsolete links: %v", empty)
	}
}

func TestSaveForeclosureRejectsInvalidEvidence(t *testing.T) {
	ctx := context.Background()
	if _, ok, err := taxstore.NewForeclosures(nil).Latest(ctx); err == nil || ok {
		t.Fatalf("nil database ok=%v err=%v", ok, err)
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL must name a disposable migrated database")
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	store, err := taxartifact.NewLocal(filepath.Join(t.TempDir(), "private"))
	if err != nil {
		t.Fatal(err)
	}
	list, err := union.NewForeclosureList(filepath.Join("..", "connector", "union", "testdata", "tax", "foreclosure.pdf"), store)
	if err != nil {
		t.Fatal(err)
	}
	var sale union.ForeclosureCase
	meta, err := list.Read(ctx, func(item union.ForeclosureCase) error { sale = item; return nil })
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name    string
		meta    taxsource.Meta
		sale    union.ForeclosureCase
		skipped int
	}{
		{"wrong source", func() taxsource.Meta { bad := meta; bad.Name = "union_billing"; return bad }(), sale, 0},
		{"negative skip", meta, sale, -1},
		{"no PIN", meta, union.ForeclosureCase{}, 0},
		{"empty parcel key", meta, union.ForeclosureCase{PINs: []union.ForeclosurePIN{{}}}, 0},
		{"negative bid", meta, union.ForeclosureCase{PINs: sale.PINs, CurrentBid: func() *int64 { v := int64(-1); return &v }()}, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := taxstore.SaveForeclosure(ctx, tx, []union.ForeclosureCase{test.sale}, test.meta, test.skipped); err == nil {
				t.Fatal("invalid foreclosure input accepted")
			}
		})
	}
	cancelled, stop := context.WithCancel(ctx)
	stop()
	if _, err := taxstore.SaveForeclosure(cancelled, tx, nil, meta, 0); err == nil {
		t.Fatal("canceled import accepted")
	}
	if _, ok, err := taxstore.NewForeclosures(pool).Latest(cancelled); err == nil || ok {
		t.Fatalf("canceled latest: ok=%v err=%v", ok, err)
	}
}
