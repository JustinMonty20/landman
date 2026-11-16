package taxstore

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/JustinMonty20/landman/internal/county"
	"github.com/JustinMonty20/landman/internal/taxsource"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ForeclosureResult counts cases, parcel joins, blocks without PINs, and unmatched PINs.
type ForeclosureResult struct {
	SavedCases    int
	SavedJoins    int
	SkippedBlocks int
	Unmatched     int
}

// SaveForeclosure writes complete cases and parcel joins in the caller's transaction.
// The caller rolls back on failure and commits only after the entire file succeeds.
func SaveForeclosure(ctx context.Context, tx pgx.Tx, cases []union.ForeclosureCase, meta taxsource.Meta, skippedBlocks int) (ForeclosureResult, error) {
	var result ForeclosureResult
	if meta.Name != "union_foreclosure_sales" || meta.URL != union.ForeclosureURL || meta.FetchedAt.IsZero() || !meta.Artifact.Verified() || meta.SHA256 != meta.Artifact.SHA256() || skippedBlocks < 0 {
		return result, errors.New("invalid foreclosure import evidence")
	}
	result.SkippedBlocks = skippedBlocks
	id, err := foreclosureImportID(ctx, tx, meta)
	if err != nil {
		return result, err
	}
	digest := meta.Artifact.SHA256()
	for _, sale := range cases {
		if len(sale.PINs) == 0 || !foreclosureAmountValid(sale.OpeningBid) || !foreclosureAmountValid(sale.CurrentBid) || !foreclosureAmountValid(sale.UpsetBid) {
			return result, errors.New("invalid foreclosure case")
		}
		keys := make([]string, 0, len(sale.PINs))
		legal := make([]string, 0, len(sale.PINs))
		for _, pin := range sale.PINs {
			if pin.ParcelKey == "" || !foreclosureAmountValid(pin.TaxValue) {
				return result, errors.New("invalid foreclosure PIN")
			}
			keys = append(keys, pin.ParcelKey)
			legal = append(legal, pin.LegalDescription)
		}
		sort.Strings(keys)
		sort.Strings(legal)
		pinHash := sha256.Sum256([]byte(strings.Join(keys, "\x00")))
		legalHash := sha256.Sum256([]byte(strings.Join(legal, "\x00")))
		var caseID string
		var number any
		if sale.CaseNumber != "" && !strings.EqualFold(sale.CaseNumber, "TBD") {
			number = sale.CaseNumber
		}
		var on any
		if sale.SaleOn != nil {
			on = sale.SaleOn.Format("2006-01-02")
		}
		query := `INSERT INTO tax_foreclosure_cases (
county_id,source_name,case_number,parcel_set_sha256,legal_sha256,owner_name,sale_on,
opening_bid,current_bid,upset_bid,status,redeemed,bidding_closed,source_url,file_sha256,fetched_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
ON CONFLICT (county_id,source_name,case_number) WHERE case_number IS NOT NULL
DO UPDATE SET parcel_set_sha256=EXCLUDED.parcel_set_sha256,legal_sha256=EXCLUDED.legal_sha256,
owner_name=EXCLUDED.owner_name,sale_on=EXCLUDED.sale_on,opening_bid=EXCLUDED.opening_bid,
current_bid=EXCLUDED.current_bid,upset_bid=EXCLUDED.upset_bid,status=EXCLUDED.status,
redeemed=EXCLUDED.redeemed,bidding_closed=EXCLUDED.bidding_closed,source_url=EXCLUDED.source_url,
file_sha256=EXCLUDED.file_sha256,fetched_at=EXCLUDED.fetched_at,updated_at=now() RETURNING id::text`
		if number == nil {
			query = strings.Replace(query,
				"ON CONFLICT (county_id,source_name,case_number) WHERE case_number IS NOT NULL",
				"ON CONFLICT (county_id,source_name,parcel_set_sha256,legal_sha256) WHERE case_number IS NULL", 1)
		}
		err := tx.QueryRow(ctx, query, county.Union, meta.Name, number, pinHash[:], legalHash[:], foreclosureNullable(sale.OwnerName), on,
			foreclosureDollars(sale.OpeningBid), foreclosureDollars(sale.CurrentBid), foreclosureDollars(sale.UpsetBid),
			foreclosureNullable(sale.Status), sale.Redeemed, sale.BiddingClosed, meta.URL, digest[:], meta.FetchedAt).Scan(&caseID)
		if err != nil {
			return result, fmt.Errorf("save foreclosure case: %w", err)
		}
		result.SavedCases++
		for _, pin := range sale.PINs {
			var parcelID string
			err := tx.QueryRow(ctx, `SELECT id FROM parcels WHERE county_id=$1 AND source_name=$2 AND source_parcel_id=$3`, county.Union, "union_county_gis", pin.ParcelKey).Scan(&parcelID)
			if errors.Is(err, pgx.ErrNoRows) {
				result.Unmatched++
				continue
			}
			if err != nil {
				return result, fmt.Errorf("lookup foreclosure parcel: %w", err)
			}
			_, err = tx.Exec(ctx, `INSERT INTO tax_foreclosure_parcels (case_id,parcel_id,tax_value,situs_line,legal_description)
VALUES ($1,$2,$3,$4,$5) ON CONFLICT (case_id,parcel_id) DO UPDATE SET
 tax_value=EXCLUDED.tax_value,situs_line=EXCLUDED.situs_line,
 legal_description=EXCLUDED.legal_description,updated_at=now()`, caseID, parcelID,
				foreclosureDollars(pin.TaxValue), foreclosureNullable(pin.SitusLine), foreclosureNullable(pin.LegalDescription))
			if err != nil {
				return result, fmt.Errorf("save foreclosure parcel: %w", err)
			}
			result.SavedJoins++
		}
	}
	if err := ActivateImport(ctx, tx, id); err != nil {
		return result, err
	}
	return result, nil
}

func foreclosureAmountValid(amount *int64) bool {
	return amount == nil || (*amount >= 0 && *amount <= 99999999999999)
}
func foreclosureNullable(text string) any {
	if text == "" {
		return nil
	}
	return text
}
func foreclosureDollars(amount *int64) any {
	if amount == nil {
		return nil
	}
	return fmt.Sprintf("%d.%02d", *amount/100, *amount%100)
}

func foreclosureImportID(ctx context.Context, tx pgx.Tx, meta taxsource.Meta) (string, error) {
	digest := meta.Artifact.SHA256()
	var id, validation string
	err := tx.QueryRow(ctx, `SELECT id::text,validation_result FROM tax_file_imports
WHERE county_id=$1 AND source_name=$2 AND source_url=$3 AND file_sha256=$4 AND fetched_at=$5`,
		county.Union, meta.Name, meta.URL, digest[:], meta.FetchedAt).Scan(&id, &validation)
	if err == nil {
		if validation != string(ImportValid) {
			return "", errors.New("foreclosure file import is invalid")
		}
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("find foreclosure file import: %w", err)
	}
	return SaveImportEvidence(ctx, tx, ImportEvidence{CountyID: county.Union, SourceName: meta.Name, SourceURL: meta.URL,
		FetchedAt: meta.FetchedAt, Artifact: meta.Artifact, Validation: ImportValid})
}

// Foreclosures reads the latest stored case time, not proof of a fully imported file.
type Foreclosures struct{ pool *pgxpool.Pool }

func NewForeclosures(pool *pgxpool.Pool) Foreclosures { return Foreclosures{pool: pool} }
func (s Foreclosures) Latest(ctx context.Context) (time.Time, bool, error) {
	if s.pool == nil {
		return time.Time{}, false, errors.New("foreclosure database required")
	}
	var latest *time.Time
	if err := s.pool.QueryRow(ctx, `SELECT max(fetched_at) FROM tax_foreclosure_cases WHERE county_id=$1 AND source_name=$2`, county.Union, "union_foreclosure_sales").Scan(&latest); err != nil {
		return time.Time{}, false, fmt.Errorf("latest foreclosure case: %w", err)
	}
	if latest == nil {
		return time.Time{}, false, nil
	}
	return *latest, true, nil
}
