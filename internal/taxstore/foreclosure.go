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

// SaveForeclosure writes cases and replaces numbered-case parcel links in the caller's transaction.
// The caller rolls back on failure and commits only after the entire file succeeds.
func SaveForeclosure(ctx context.Context, tx pgx.Tx, cases []union.ForeclosureCase, meta taxsource.Meta, skippedBlocks int) (ForeclosureResult, error) {
	var result ForeclosureResult
	if meta.Name != "union_foreclosure" || meta.URL != union.ForeclosureURL || meta.FetchedAt.IsZero() || !meta.Artifact.Verified() || meta.SHA256 != meta.Artifact.SHA256() || skippedBlocks < 0 {
		return result, errors.New("invalid foreclosure import evidence")
	}
	result.SkippedBlocks = skippedBlocks
	id, err := foreclosureImportID(ctx, tx, meta)
	if err != nil {
		return result, err
	}
	digest := meta.Artifact.SHA256()
	numbered := make(map[string][]string)
	var caseOrder []string
	for _, sale := range cases {
		if len(sale.PINs) == 0 || !foreclosureAmountValid(sale.OpeningBid) || !foreclosureAmountValid(sale.CurrentBid) || !foreclosureAmountValid(sale.UpsetBid) {
			return result, errors.New("invalid foreclosure case")
		}
		for _, pin := range sale.PINs {
			if pin.ParcelKey == "" || !foreclosureAmountValid(pin.TaxValue) {
				return result, errors.New("invalid foreclosure PIN")
			}
		}
		identity := foreclosureIdentityFor(sale)
		var caseID string
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
		if identity.number == nil {
			query = strings.Replace(query,
				"ON CONFLICT (county_id,source_name,case_number) WHERE case_number IS NOT NULL",
				"ON CONFLICT (county_id,source_name,parcel_set_sha256,legal_sha256) WHERE case_number IS NULL", 1)
		}
		err := tx.QueryRow(ctx, query, county.Union, meta.Name, identity.number, identity.pinHash[:], identity.legalHash[:], foreclosureNullable(sale.OwnerName), on,
			foreclosureDollars(sale.OpeningBid), foreclosureDollars(sale.CurrentBid), foreclosureDollars(sale.UpsetBid),
			foreclosureNullable(sale.Status), sale.Redeemed, sale.BiddingClosed, meta.URL, digest[:], meta.FetchedAt).Scan(&caseID)
		if err != nil {
			return result, fmt.Errorf("save foreclosure case: %w", err)
		}
		result.SavedCases++
		matched := make([]string, 0, len(sale.PINs))
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
			matched = append(matched, parcelID)
		}
		if identity.number != nil {
			if _, ok := numbered[caseID]; !ok {
				caseOrder = append(caseOrder, caseID)
				numbered[caseID] = make([]string, 0, len(sale.PINs))
			}
			numbered[caseID] = append(numbered[caseID], matched...)
		}
	}
	for _, caseID := range caseOrder {
		_, err = tx.Exec(ctx, `DELETE FROM tax_foreclosure_parcels WHERE case_id=$1 AND NOT (parcel_id = ANY($2::uuid[]))`, caseID, numbered[caseID])
		if err != nil {
			return result, fmt.Errorf("remove obsolete foreclosure parcel links: %w", err)
		}
	}
	if err := ActivateImport(ctx, tx, id); err != nil {
		return result, err
	}
	return result, nil
}

type foreclosureIdentity struct {
	number    *string
	pinHash   [32]byte
	legalHash [32]byte
	keys      []string
}

func foreclosureIdentityFor(sale union.ForeclosureCase) foreclosureIdentity {
	ordered := append([]union.ForeclosurePIN(nil), sale.PINs...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ParcelKey < ordered[j].ParcelKey })
	identity := foreclosureIdentity{keys: make([]string, 0, len(ordered))}
	legal := make([]string, 0, len(ordered))
	for _, pin := range ordered {
		identity.keys = append(identity.keys, pin.ParcelKey)
		legal = append(legal, pin.LegalDescription)
	}
	identity.pinHash = sha256.Sum256([]byte(strings.Join(identity.keys, "\n")))
	identity.legalHash = sha256.Sum256([]byte(strings.Join(legal, "\n")))
	if sale.CaseNumber != "" && !strings.EqualFold(sale.CaseNumber, "TBD") {
		identity.number = &sale.CaseNumber
	}
	return identity
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

// Foreclosures checks active file evidence and parcel links without changing stored rows.
// Latest reports case time only; it does not prove a complete import.
type Foreclosures struct{ pool *pgxpool.Pool }

func NewForeclosures(pool *pgxpool.Pool) Foreclosures { return Foreclosures{pool: pool} }

// Current reports whether the active foreclosure file has every current parcel link.
// It does not change stored rows, so an unchanged retry does not write.
func (s Foreclosures) Current(ctx context.Context, meta taxsource.Meta, cases []union.ForeclosureCase) (bool, error) {
	if s.pool == nil {
		return false, errors.New("foreclosure database required")
	}
	digest := meta.Artifact.SHA256()
	var active bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM tax_file_imports WHERE county_id=$1 AND source_name=$2 AND source_url=$3 AND active AND file_sha256=$4)`, county.Union, meta.Name, meta.URL, digest[:]).Scan(&active); err != nil || !active {
		return false, err
	}
	numbered := make(map[string][]string)
	for _, sale := range cases {
		identity := foreclosureIdentityFor(sale)
		var id string
		err := s.pool.QueryRow(ctx, `SELECT id::text FROM tax_foreclosure_cases WHERE county_id=$1 AND source_name=$2 AND file_sha256=$3 AND ((case_number=$4 AND case_number IS NOT NULL) OR (case_number IS NULL AND $4::text IS NULL AND parcel_set_sha256=$5 AND legal_sha256=$6))`, county.Union, meta.Name, digest[:], identity.number, identity.pinHash[:], identity.legalHash[:]).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		for _, pin := range sale.PINs {
			var missing bool
			err = s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM parcels p LEFT JOIN tax_foreclosure_parcels j ON j.case_id=$1::uuid AND j.parcel_id=p.id WHERE p.county_id=$2 AND p.source_name='union_county_gis' AND p.source_parcel_id=$3 AND j.id IS NULL)`, id, county.Union, pin.ParcelKey).Scan(&missing)
			if err != nil {
				return false, err
			}
			if missing {
				return false, nil
			}
		}
		if identity.number != nil {
			numbered[id] = append(numbered[id], identity.keys...)
		}
	}
	for id, keys := range numbered {
		var obsolete bool
		err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM tax_foreclosure_parcels j JOIN parcels p ON p.id=j.parcel_id
WHERE j.case_id=$1::uuid AND (p.county_id <> $2 OR p.source_name <> 'union_county_gis' OR NOT (p.source_parcel_id = ANY($3::text[]))))`, id, county.Union, keys).Scan(&obsolete)
		if err != nil {
			return false, err
		}
		if obsolete {
			return false, nil
		}
	}
	return true, nil
}

func (s Foreclosures) Latest(ctx context.Context) (time.Time, bool, error) {
	if s.pool == nil {
		return time.Time{}, false, errors.New("foreclosure database required")
	}
	var latest *time.Time
	if err := s.pool.QueryRow(ctx, `SELECT max(fetched_at) FROM tax_foreclosure_cases WHERE county_id=$1 AND source_name=$2`, county.Union, "union_foreclosure").Scan(&latest); err != nil {
		return time.Time{}, false, fmt.Errorf("latest foreclosure case: %w", err)
	}
	if latest == nil {
		return time.Time{}, false, nil
	}
	return *latest, true, nil
}
