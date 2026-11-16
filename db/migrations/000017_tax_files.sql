-- +goose Up
CREATE TABLE tax_delinquent_balances (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    county_id TEXT NOT NULL CHECK (county_id IN ('union')),
    source_name TEXT NOT NULL,
    parcel_id UUID NOT NULL REFERENCES parcels(id) ON DELETE CASCADE,
    tax_year SMALLINT NOT NULL CHECK (tax_year BETWEEN 1900 AND 2200),
    balance_due NUMERIC(14, 2) NOT NULL CHECK (balance_due >= 0),
    primary_source TEXT NOT NULL CHECK (primary_source = 'RE'),
    source_url TEXT NOT NULL,
    file_sha256 BYTEA NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (county_id, source_name, parcel_id, tax_year)
);
CREATE INDEX tax_delinquent_balances_parcel_year_idx ON tax_delinquent_balances (parcel_id, tax_year);

CREATE TABLE tax_bills (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    county_id TEXT NOT NULL CHECK (county_id IN ('union')),
    source_name TEXT NOT NULL,
    parcel_id UUID NOT NULL REFERENCES parcels(id) ON DELETE CASCADE,
    tax_year SMALLINT NOT NULL CHECK (tax_year BETWEEN 1900 AND 2200),
    total_taxes NUMERIC(14, 2) NOT NULL CHECK (total_taxes >= 0),
    property_address TEXT,
    taxpayer_name TEXT,
    mailing_line TEXT,
    mailing_city TEXT,
    mailing_state TEXT,
    mailing_postal_code TEXT,
    source_url TEXT NOT NULL,
    file_sha256 BYTEA NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (county_id, source_name, parcel_id, tax_year)
);
CREATE INDEX tax_bills_parcel_year_idx ON tax_bills (parcel_id, tax_year);

CREATE TABLE tax_foreclosure_cases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    county_id TEXT NOT NULL CHECK (county_id IN ('union')),
    source_name TEXT NOT NULL,
    case_number TEXT,
    parcel_set_sha256 BYTEA NOT NULL,
    legal_sha256 BYTEA NOT NULL,
    owner_name TEXT,
    sale_on DATE,
    tax_year SMALLINT CHECK (tax_year BETWEEN 1900 AND 2200),
    opening_bid NUMERIC(14, 2) CHECK (opening_bid >= 0),
    current_bid NUMERIC(14, 2) CHECK (current_bid >= 0),
    upset_bid NUMERIC(14, 2) CHECK (upset_bid >= 0),
    status TEXT,
    redeemed BOOLEAN NOT NULL DEFAULT false,
    bidding_closed BOOLEAN NOT NULL DEFAULT false,
    source_url TEXT NOT NULL,
    file_sha256 BYTEA NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX tax_foreclosure_cases_number_idx
    ON tax_foreclosure_cases (county_id, source_name, case_number)
    WHERE case_number IS NOT NULL;
CREATE UNIQUE INDEX tax_foreclosure_cases_unnumbered_idx
    ON tax_foreclosure_cases (county_id, source_name, parcel_set_sha256, legal_sha256)
    WHERE case_number IS NULL;

CREATE TABLE tax_foreclosure_parcels (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    case_id UUID NOT NULL REFERENCES tax_foreclosure_cases(id) ON DELETE CASCADE,
    parcel_id UUID NOT NULL REFERENCES parcels(id) ON DELETE CASCADE,
    tax_value NUMERIC(14, 2) CHECK (tax_value >= 0),
    situs_line TEXT,
    legal_description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (case_id, parcel_id)
);

CREATE TABLE tax_foreclosure_pipeline (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    county_id TEXT NOT NULL CHECK (county_id IN ('union')),
    source_name TEXT NOT NULL,
    parcel_id UUID NOT NULL REFERENCES parcels(id) ON DELETE CASCADE,
    owner_name TEXT,
    situs_line TEXT,
    township TEXT,
    acreage TEXT,
    tax_value NUMERIC(14, 2) CHECK (tax_value >= 0),
    case_number TEXT,
    sale_on DATE,
    schedule_text TEXT,
    status TEXT,
    redeemed BOOLEAN NOT NULL DEFAULT false,
    source_url TEXT NOT NULL,
    file_sha256 BYTEA NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (county_id, source_name, parcel_id)
);
CREATE INDEX tax_foreclosure_pipeline_parcel_idx ON tax_foreclosure_pipeline (parcel_id);

CREATE VIEW tax_owed AS
SELECT tax_delinquent_balances.parcel_id,
       tax_delinquent_balances.tax_year,
       SUM(tax_delinquent_balances.balance_due) AS balance_due
FROM tax_delinquent_balances
JOIN parcels ON parcels.id = tax_delinquent_balances.parcel_id
WHERE parcels.likely_vacant AND tax_delinquent_balances.balance_due > 0
GROUP BY tax_delinquent_balances.parcel_id, tax_delinquent_balances.tax_year;

-- +goose Down
DROP VIEW tax_owed;
DROP TABLE tax_foreclosure_pipeline;
DROP TABLE tax_foreclosure_parcels;
DROP TABLE tax_foreclosure_cases;
DROP TABLE tax_bills;
DROP TABLE tax_delinquent_balances;
