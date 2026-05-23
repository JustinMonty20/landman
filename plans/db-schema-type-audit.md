# Audit initial Postgres schema types

## Context
- User flagged that the initial migration overused `BIGINT`, especially for semantic fields like `parcel_year` and `tax_owner_id`.
- This is correct: the SQL schema should use domain-appropriate database types, not blindly mirror Go `int64` model fields.
- The migration appears newly created and not yet applied in production, so editing `000001_init_schema.*.sql` directly is acceptable. If already applied anywhere persistent, create a corrective migration instead.

## Approach
- Keep internal surrogate keys as large generated integers.
- Treat external identifiers as `TEXT` unless arithmetic is required, because parcel/account/owner identifiers may have leading zeroes or become alphanumeric.
- Use `SMALLINT` plus check constraints for year/month/day/sequence fields.
- Use exact `NUMERIC` for money, acreage, square footage, and source-provided decimal values where precision matters.
- Keep epoch millisecond originals as `BIGINT`.
- Keep SRID as `INTEGER`.
- Keep PostGIS shape column and indexes.

## Files to modify
- `db/migrations/000001_init_schema.up.sql`

## Steps
- [x] Change `parcel_year`, `year_built`, `sale_year`, `sale_month`, `sale_day`, and `sequence` away from broad `BIGINT`/`INTEGER` where appropriate.
- [x] Change `tax_owner_id_1` and `tax_owner_id_2` to `TEXT` external identifiers.
- [x] Change valuation/acreage/sqft/amount fields from floating point to `NUMERIC` with reasonable precision.
- [x] Add check constraints for bounded fields.
- [x] Run `go test ./...`.

## Verification
- `go test ./...`
- Optionally apply/rollback against a local Postgres database once `DATABASE_URL` is available.
