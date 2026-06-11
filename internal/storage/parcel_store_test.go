package storage

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/models"
	"github.com/jackc/pgx/v5/pgconn"
)

type fakeBeginner struct {
	tx *fakeTx
}

func (f *fakeBeginner) Begin(ctx context.Context) (transaction, error) {
	return f.tx, nil
}

type fakeTx struct {
	beginQueue []*fakeTx
	begun      []*fakeTx
	queryErrs  []error
	execErrs   []error
	queries    []queryCall
	execs      []execCall
	commits    int
	rollbacks  int
}

type queryCall struct {
	sql  string
	args []any
}

type execCall struct {
	sql  string
	args []any
}

func (f *fakeTx) Begin(ctx context.Context) (transaction, error) {
	child := &fakeTx{}
	if len(f.beginQueue) > 0 {
		child = f.beginQueue[0]
		f.beginQueue = f.beginQueue[1:]
	}
	f.begun = append(f.begun, child)
	return child, nil
}

func (f *fakeTx) Commit(ctx context.Context) error {
	f.commits++
	return nil
}

func (f *fakeTx) Rollback(ctx context.Context) error {
	f.rollbacks++
	return nil
}

func (f *fakeTx) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	f.execs = append(f.execs, execCall{sql: sql, args: arguments})
	if len(f.execErrs) == 0 {
		return pgconn.CommandTag{}, nil
	}
	err := f.execErrs[0]
	f.execErrs = f.execErrs[1:]
	return pgconn.CommandTag{}, err
}

func (f *fakeTx) QueryRow(ctx context.Context, sql string, args ...any) row {
	f.queries = append(f.queries, queryCall{sql: sql, args: args})
	if len(f.queryErrs) == 0 {
		return fakeRow{id: "00000000-0000-0000-0000-000000000001"}
	}
	err := f.queryErrs[0]
	f.queryErrs = f.queryErrs[1:]
	return fakeRow{err: err}
}

type fakeRow struct {
	id  string
	err error
}

func (f fakeRow) Scan(dest ...any) error {
	if f.err != nil {
		return f.err
	}
	ptr := dest[0].(*string)
	*ptr = f.id
	return nil
}

func TestPostgresParcelStore_InsertBatch_CommitsValidParcelsAndRollsBackFailedSavepoints(t *testing.T) {
	duplicateErr := errors.New("duplicate key value violates unique constraint")
	validChild := &fakeTx{}
	duplicateChild := &fakeTx{queryErrs: []error{duplicateErr}}
	parent := &fakeTx{beginQueue: []*fakeTx{validChild, duplicateChild}}

	store := newPostgresParcelStore(&fakeBeginner{tx: parent})

	result, err := store.InsertBatch(context.Background(), []*models.GISParcel{
		testParcel("P1"),
		testParcel("P2"),
	})
	if err != nil {
		t.Fatalf("InsertBatch returned fatal error: %v", err)
	}

	if result.Inserted != 1 {
		t.Fatalf("Inserted = %d, want 1", result.Inserted)
	}
	if len(result.Errors) != 1 {
		t.Fatalf("Errors = %d, want 1", len(result.Errors))
	}
	if result.Errors[0].ParcelID != "P2" || !errors.Is(result.Errors[0].Err, duplicateErr) {
		t.Fatalf("unexpected parcel error: %+v", result.Errors[0])
	}
	if parent.commits != 1 {
		t.Fatalf("parent commits = %d, want 1", parent.commits)
	}
	if validChild.commits != 1 {
		t.Fatalf("valid savepoint commits = %d, want 1", validChild.commits)
	}
	if duplicateChild.rollbacks != 1 {
		t.Fatalf("failed savepoint rollbacks = %d, want 1", duplicateChild.rollbacks)
	}
}

func TestPostgresParcelStore_InsertBatch_InsertsParcelAndSales(t *testing.T) {
	child := &fakeTx{}
	parent := &fakeTx{beginQueue: []*fakeTx{child}}
	store := newPostgresParcelStore(&fakeBeginner{tx: parent})

	parcel := testParcel("P1")
	saleDate := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	amount := 125000.50
	parcel.Sales = []models.ParcelSale{{Sequence: 1, SaleDate: &saleDate, Amount: &amount}}

	result, err := store.InsertBatch(context.Background(), []*models.GISParcel{parcel})
	if err != nil {
		t.Fatalf("InsertBatch returned error: %v", err)
	}
	if result.Inserted != 1 || len(result.Errors) != 0 {
		t.Fatalf("result = %+v, want one insert and no errors", result)
	}
	if len(child.queries) != 1 {
		t.Fatalf("parcel insert count = %d, want 1", len(child.queries))
	}
	if len(child.queries[0].args) != 52 {
		t.Fatalf("parcel arg count = %d, want 52", len(child.queries[0].args))
	}
	if got := child.queries[0].args[0]; got != "union" {
		t.Fatalf("county_id arg = %v, want union", got)
	}
	if got := child.queries[0].args[3]; got != "P1" {
		t.Fatalf("source_parcel_id arg = %v, want P1", got)
	}
	if !strings.Contains(string(child.queries[0].args[51].([]byte)), "union") {
		t.Fatalf("source_extras JSON = %s, want union payload", child.queries[0].args[51])
	}
	if len(child.execs) != 1 {
		t.Fatalf("sale insert count = %d, want 1", len(child.execs))
	}
	if got := child.execs[0].args[1]; got != 1 {
		t.Fatalf("sale sequence arg = %v, want 1", got)
	}
}

func TestPostgresParcelStore_InsertBatch_ValidationErrorsAreParcelErrors(t *testing.T) {
	child := &fakeTx{}
	parent := &fakeTx{beginQueue: []*fakeTx{child}}
	store := newPostgresParcelStore(&fakeBeginner{tx: parent})

	badYear := int64(2201)
	parcel := testParcel("P1")
	parcel.Identity.ParcelYear = &badYear

	result, err := store.InsertBatch(context.Background(), []*models.GISParcel{parcel})
	if err != nil {
		t.Fatalf("InsertBatch returned fatal error: %v", err)
	}
	if result.Inserted != 0 || len(result.Errors) != 1 {
		t.Fatalf("result = %+v, want one parcel error", result)
	}
	if len(child.queries) != 0 {
		t.Fatalf("queries = %d, want 0 for invalid parcel", len(child.queries))
	}
}

func testParcel(sourceParcelID string) *models.GISParcel {
	parcelNumber := sourceParcelID
	ownerID := int64(42)
	return &models.GISParcel{
		Identity: models.ParcelIdentity{
			CountyID:       "union",
			CountyCode:     "union",
			SourceName:     "union_county_gis",
			SourceParcelID: sourceParcelID,
			ParcelNumber:   &parcelNumber,
		},
		Owner:        models.ParcelOwner{TaxOwnerID1: &ownerID},
		Geometry:     models.GISGeometry{SRID: 3857},
		SourceExtras: models.ParcelSourceExtras{Union: &models.UnionParcelExtras{}},
	}
}
